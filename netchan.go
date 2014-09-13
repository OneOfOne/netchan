package netchan

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	oerr "github.com/OneOfOne/go-utils/errors"
	osync "github.com/OneOfOne/go-utils/sync"
)

var (
	// ErrInvalidResponse is returned when the client or server sent something
	// unexpected during the handshake.
	ErrInvalidResponse = errors.New("invalid response")
)

type pktType byte

const (
	pktInvalid pktType = iota
	pktHandshake
	pktHandshakeConfirm
	pktData
	pktStatus
)

// ChannelStatus defines the return values from Send/Recv
type ChannelStatus byte

const (
	// StatusNotFound returned when a valid receiver (or sender) coudln't be found
	StatusNotFound ChannelStatus = iota

	// StatusNotReady means all channel buffers were full, try again later
	StatusNotReady

	// StatusClosed means the channel is closed and should no longer be used
	StatusClosed

	// StatusOK means the message was received (or sent) successfully
	StatusOK
)

func (i ChannelStatus) String() string {
	switch i {
	case StatusNotFound:
		return "NotFound"
	case StatusNotReady:
		return "NotReady"
	case StatusOK:
		return "OK"
	case StatusClosed:
		return "Closed"
	}
	return "Invalid"
}

type pkt struct {
	Type  pktType
	Value interface{}
	srcID []byte
}

func init() {
	gob.Register(pkt{})
}

func genID() (id []byte) {
	id = make([]byte, 16)
	rand.Read(id)
	return
}

type connChan struct {
	id []byte
	net.Conn
	r, s chan *pkt

	*gob.Encoder
	*gob.Decoder

	ctrl chan *connChan
}

func newConnChan(conn net.Conn, r, s chan *pkt, ctrl chan *connChan) (ch *connChan) {
	ch = &connChan{
		Conn: conn,

		r: r,
		s: s,

		Encoder: gob.NewEncoder(conn),
		Decoder: gob.NewDecoder(bufio.NewReaderSize(conn, 16)), //using larger buffers make the connection block sometimes.

		ctrl: ctrl,
	}
	return
}

func (c *connChan) clientHandshake(id []byte) (err error) {
	c.id = id
	if err = c.Encode(&pkt{Type: pktHandshake, Value: id}); err != nil {
		return
	}
	p := &pkt{}
	if err = c.Decode(p); err != nil {
		return
	}
	if p.Type != pktHandshakeConfirm {
		return ErrInvalidResponse
	}
	go c.receiver()
	go c.sender()
	return
}

func (c *connChan) serverHandshake() (err error) {
	p := &pkt{}
	if err = c.Decode(p); err != nil {
		return
	}
	if p.Type != pktHandshake {
		return ErrInvalidResponse
	}
	c.id = p.Value.([]byte)
	p.Type = pktHandshakeConfirm
	if err = c.Encode(p); err != nil {
		return
	}
	go c.receiver()
	go c.sender()
	return
}

func (c *connChan) receiver() {
L:
	for {
		p := &pkt{srcID: c.id}
		if err := c.Decode(&p); err != nil {
			break
		}

		switch p.Type {
		case pktData:
			c.r <- p
		case pktStatus:
			if st, ok := p.Value.(ChannelStatus); ok && st == StatusClosed {
				break L
			}
		default: //might remove this
			panic(fmt.Sprintf("%v", p))
		}
	}
	c.ctrl <- c
	//fmt.Println("receiver closed")
}

func (c *connChan) sender() {
	for {
		select {
		case p := <-c.s:
			if p.Type == pktInvalid { // screw it all
				break
			}
			if err := c.Encode(p); err != nil {
				c.s <- p
				break
			}
		}
	}
}

func (c *connChan) Send(val interface{}) (st ChannelStatus) {
	p := &pkt{Type: pktData, Value: val}
	return c.send(p)
}

func (c *connChan) send(p *pkt) (st ChannelStatus) {
	st = StatusOK
	if err := c.Encode(p); err != nil {
		st = StatusClosed
	}
	return
}

func (c *connChan) Close() error {
	c.send(&pkt{Type: pktStatus, Value: StatusClosed})
	return c.Conn.Close()
}

type ctOS struct {
	id []byte
	lk osync.SpinLock

	listeners []net.Listener

	// using a map as a slice because we're that cool,
	// also halfway guarenteed random selection
	nchans map[*connChan]struct{}

	r, s chan *pkt

	ctrl chan *connChan
	log  *log.Logger
}

func (ct *ctOS) addChan(ch *connChan) {
	ct.lk.Lock()
	ct.nchans[ch] = struct{}{}
	ct.lk.Unlock()
}

func (ct *ctOS) sucideLine() {
	for ch := range ct.ctrl {
		ct.log.Printf("channel [id=%x] disconnected from %v", ch.id, ch.RemoteAddr())
		ct.lk.Lock()
		delete(ct.nchans, ch)
		ct.lk.Unlock()
	}
}

// IsClosed returns if the channel is closed or not
func (ct *ctOS) IsClosed() (st bool) {
	ct.lk.Lock()
	st = len(ct.nchans) == 0 && len(ct.listeners) == 0
	ct.lk.Unlock()
	return
}

// Recv returns who sent the message, the message and the status of the channel
// optionally blocks until there's a message to receive.
func (ct *ctOS) Recv(block bool) (id []byte, val interface{}, st ChannelStatus) {
	if ct.IsClosed() {
		st = StatusClosed
		return
	}
	var (
		p  *pkt
		ok bool
	)
	if block {
		p, ok = <-ct.r
	} else {
		select {
		case p, ok = <-ct.r:
		default:
			st = StatusNotReady
		}
	}
	if ok {
		st = StatusOK
		id = p.srcID
		val = p.Value
	} else if st == StatusNotFound {
		st = StatusClosed
	}
	return
}

// Send sends a message over the network, optionally blocks until it finds a receiver.
func (ct *ctOS) Send(block bool, val interface{}) (st ChannelStatus) {
	if ct.IsClosed() {
		return StatusClosed
	}

	p := &pkt{Type: pktData, Value: val}
	if block {
		select {
		case ct.s <- p:
			st = StatusOK
		}
	} else {
		select {
		case ct.s <- p:
			st = StatusOK
		default:
			st = StatusNotReady
		}

	}
	return
}

// SendTo like send but tries to send to a specific client,
// returns StatusNotFound if the client doesn't exist anymore.
// note that SendTo always blocks until the message is sent or it errors out
func (ct *ctOS) SendTo(id []byte, val interface{}) (st ChannelStatus) {
	if ct.IsClosed() {
		return StatusClosed
	}
	st = StatusNotFound
	var nch *connChan

	ct.lk.Lock()
	for ch := range ct.nchans {
		if bytes.Equal(ch.id, id) {
			nch = ch
		}
	}

	ct.lk.Unlock()
	if nch != nil {
		st = nch.Send(val)
	}
	return
}

// Connect connects to a remote channel
// it can be called multiple times with different addresses
// (or really the same one if you're that kind of a person)
func (ct *ctOS) Connect(network, addr string) error {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return err
	}
	return ct.ConnectTo(conn)
}

// ConnectTo conencts to a specific net.Conn
func (ct *ctOS) ConnectTo(conn net.Conn) error {
	ch := newConnChan(conn, ct.r, ct.s, ct.ctrl)
	if err := ch.clientHandshake(ct.id); err != nil {
		ct.log.Printf("error during handshake to %s: %v", conn.RemoteAddr(), err)
		return err
	}
	ct.addChan(ch)

	ct.log.Printf("channel [id=%x] connected to %s", ch.id, conn.RemoteAddr())
	return nil
}

// ListenAndServe listens on the specific network and address waiting for remote channels
// it can be called multiple times with different addresses
func (ct *ctOS) ListenAndServe(network, addr string) error {
	l, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	go ct.Serve(l)
	return nil
}

// Serve well, it serves a listener, ok golint?
func (ct *ctOS) Serve(l net.Listener) error {
	ct.lk.Lock()
	ct.listeners = append(ct.listeners, l)
	ct.lk.Unlock()
	ct.log.Printf("serving on %s", l.Addr())
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		ch := newConnChan(conn, ct.r, ct.s, ct.ctrl)
		if err := ch.serverHandshake(); err != nil {
			ct.log.Printf("error during handshake from %s: %v", conn.RemoteAddr(), err)
			continue
		}
		ct.addChan(ch)
		ct.log.Printf("channel [id=%x] connected from %s", ch.id, conn.RemoteAddr())
	}
}

// Close closes all the listeners and connected channels
func (ct *ctOS) Close() error {
	var errs oerr.MultiError

	// kill listeners first if any
	ct.lk.Lock()
	for _, l := range ct.listeners {
		errs.Append(l.Close())
	}
	ct.listeners = nil
	nchans := make([]*connChan, 0, len(ct.nchans))

	die := &pkt{}
	for ch := range ct.nchans {
		ct.s <- die
		nchans = append(nchans, ch)
	}
	ct.lk.Unlock()
	for _, ch := range nchans {
		if err := ch.Close(); err != nil {
			errs.Append(Error{Id: ch.id, Addr: ch.RemoteAddr(), Err: err})
		}
	}
	select { //because why the hell not
	case ct.s <- die:
	default:
	}

	close(ct.r)
	close(ct.s)
	close(ct.ctrl)
	return &errs
}

// Stats returns information about the current connections
func (ct *ctOS) Stats() (activeConnections, activeListeners int) {
	ct.lk.Lock()
	activeConnections, activeListeners = len(ct.nchans), len(ct.listeners)
	ct.lk.Unlock()
	return
}

// Logger returns the *log.Logger instance the channel is using.
func (ct *ctOS) Logger() *log.Logger {
	return ct.log
}

// Sender defines a write-only channel
type Sender interface {
	// Send sends a message over the network, optionally blocks until it finds a receiver.
	Send(block bool, val interface{}) (st ChannelStatus)

	// SendTo like send but tries to send to a specific client,
	// returns StatusNotFound if the client doesn't exist anymore.
	// note that SendTo always blocks until the message is sent or it errors out
	SendTo(id []byte, val interface{}) (st ChannelStatus)
}

// Receiver defines a read-only channel
type Receiver interface {
	// Recv returns who sent the message, the message and the status of the channel
	// optionally blocks until there's a message to receive.
	Recv(block bool) (id []byte, val interface{}, st ChannelStatus)
}

// Interface is the common interface for all network channels
type Interface interface {
	Sender
	Receiver
	io.Closer

	// ConnectTo conencts to a remote channel using the specific net.Conn
	ConnectTo(conn net.Conn) error

	// Serve well, serves a listener.
	// it blocks until the server is terminated.
	Serve(l net.Listener) error

	// IsClosed returns true if the channel is not usable anymore
	IsClosed() bool
}

type Channel struct {
	*ctOS
}

func (c Channel) String() string {
	ac, al := c.Stats()
	return fmt.Sprintf("Channel [id=%x, active=%d, listeners=%d]", c.id, ac, al)
}

// ListenAndServe is an alias for New(initialCap).ListenAndServe(network, addr)
func ListenAndServe(initialCap int, network, addr string) (ct Channel, err error) {
	ct = New(initialCap)
	return ct, ct.ListenAndServe(network, addr)
}

// Connect is an alias for New(1).Connect(network, addr)
func Connect(network, addr string) (ct Channel, err error) {
	ct = New(1)
	return ct, ct.Connect(network, addr)
}

// New returns a new Channel with the initial cap size for the receiver and connection pool
func New(initialCap int) Channel {
	ct := &ctOS{
		id: genID(),

		nchans: make(map[*connChan]struct{}, initialCap),

		r: make(chan *pkt, initialCap),
		s: make(chan *pkt, initialCap),

		ctrl: make(chan *connChan, 0),

		log: log.New(os.Stdout, "netchan: ", log.LstdFlags),
	}
	go ct.sucideLine()
	return Channel{ct}
}

// don't silently break the api
var _ Interface = (*Channel)(nil)
