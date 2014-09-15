package netchan

import (
	"flag"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	listenPort = flag.String("addr", "0", "listen port for tests")
	sch        = New(1)
	wg         sync.WaitGroup
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// this is super ugly, t.Parallel didn't work for some reason
func TestServerClient(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:"+*listenPort)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if *listenPort == "0" {
		*listenPort = strings.Split(l.Addr().String(), ":")[1]
	}
	t.Log("listening on", l.Addr())
	go sch.Serve(l)
	wg.Add(20)
	go testServerChannel(t)
	go testClientChannel(t)
	wg.Wait()
	sch.Close()
}
func testServerChannel(t *testing.T) {
	for i := 0; i < 10; i++ {
		go func(i int) {
			id, msg, st := sch.Recv(true)
			if st != StatusOK {
				t.Fatalf("unexpected status : %x %q %s", id, msg, st)
			}
			t.Logf("sch recieved %q from %x", msg, id)
			sch.Send(true, fmt.Sprintf("sch:%d", i))
			wg.Done()
		}(i)
	}
	//wg.Wait()
}

func testClientChannel(t *testing.T) {
	ch, err := Connect("tcp4", "127.0.0.1:"+*listenPort)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		go func(i int) {
			ch.Send(true, fmt.Sprintf("cch:%d", i))
			id, msg, st := ch.Recv(true)
			if st != StatusOK {
				t.Fatalf("unexpected status : %x %q %s", id, msg, st)
			}
			t.Logf("cch recieved %q", msg)
			wg.Done()
		}(i)
	}
	//wg.Wait()
}

func TestSelectRecv(t *testing.T) {
	nch, lch := New(1), make(chan interface{}, 0)
	go func() {
		lch <- 10
	}()
	if _, val := SelectRecv([]Receiver{nch, LocalReceiver(lch)}); val != 10 {
		t.Fatalf("expected %v, received %v", 10, val)
	}
}

func TestSelectSend(t *testing.T) {
	nch, lch := New(1), make(chan interface{}, 0)
	go func() {
		if SelectSend([]Sender{nch, LocalSender(lch)}, 10) == nil {
			t.Fatal("Select returned nil")
		}

	}()
	val := <-lch
	if val != 10 {
		t.Fatalf("expected %v, received %v", 10, val)
	}
}
