package netchan

import (
	"flag"
	"fmt"
	"log"
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
	l, err := net.Listen("tcp", "127.0.0.1:"+*listenPort)
	if err != nil {
		panic(err)
	}
	if *listenPort == "0" {
		*listenPort = strings.Split(l.Addr().String(), ":")[1]
	}
	log.Println("listening on", l.Addr())
	go sch.Serve(l)
}

// this is super ugly, t.Parallel didn't work for some reason
func Test(t *testing.T) {
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
