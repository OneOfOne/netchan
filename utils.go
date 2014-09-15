package netchan

import (
	"fmt"
	"net"
)

type Error struct {
	Id   []byte
	Addr net.Addr
	Err  error
}

func (err Error) Error() string {
	return fmt.Sprintln("Channel [id=%x, remote=%s] error = %v", err.Id, err.Addr, err.Err)
}

// LocalSender wraps a local channel as a Sender interface,
// keep in mind that it will use defer to prevent panics.
// SendTo will always return StatusNotFound
type LocalSender chan<- interface{}

func (ls LocalSender) Send(block bool, val interface{}) (st ChannelStatus) {
	defer func() {
		if recover() != nil {
			st = StatusClosed
		}
	}()
	if block {
		ls <- val
		return StatusOK
	}
	select {
	case ls <- val:
		return StatusOK
	default:
		return StatusNotReady
	}
}

func (ls LocalSender) SendTo(id []byte, val interface{}) (st ChannelStatus) {
	return StatusNotFound
}

// LocalReceiver wraps a local channel as a receiver interface,
// the id will always be nil
type LocalReceiver <-chan interface{}

func (lr LocalReceiver) Recv(block bool) (id []byte, val interface{}, st ChannelStatus) {
	defer func() {
		if recover() != nil {
			st = StatusClosed
		}
	}()
	if block {
		val = <-lr
		st = StatusOK
		return
	}
	select {
	case val = <-lr:
		st = StatusOK
	default:
		st = StatusNotReady
	}
	return
}
