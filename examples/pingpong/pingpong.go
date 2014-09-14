// this is a simple ping/pong example
// run one instance as a server with:  go run pingpong.go -s [-addr 10101]
// then run one (or more) clients with: go run pingpong [-addr 10101]
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/OneOfOne/netchan"
)

var (
	servPort = flag.String("addr", "10101", "server port")
	serv     = flag.Bool("s", false, "if true it will act as a server")
	ch       = netchan.New(1)
	N        = runtime.NumCPU()
)

func dieIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
func servLoop(i int) {
	for {
		id, msg, st := ch.Recv(true)
		if st != netchan.StatusOK {
			dieIf(fmt.Errorf("invalid recv, dying (%s): [id=%x] %q %s", st, id, msg))
		}
		log.Printf("[i=%d] received %q from %x\n", i, msg, id)
		ch.Send(true, fmt.Sprintf("pong.%d", i))
	}
}

func clientLoop(i int) {
	for {
		time.Sleep(time.Duration(rand.Intn(1000)+1000) * time.Millisecond)
		ch.Send(true, fmt.Sprintf("ping.%d", i))
		_, msg, st := ch.Recv(true)
		log.Printf("[i=%d] received %q from remote\n", i, msg)
		if st != netchan.StatusOK {
			os.Exit(0)
		}
	}
}

func main() {
	runtime.GOMAXPROCS(N)
	flag.Parse()
	addr := "127.0.0.1:" + *servPort
	log.SetFlags(0)
	if *serv {
		dieIf(ch.ListenAndServe("tcp4", addr))
		for i := 0; i < N; i++ {
			go servLoop(i)
		}
	} else {
		dieIf(ch.Connect("tcp4", addr))
		for i := 0; i < N; i++ {
			go clientLoop(i)
		}
	}
	select {} //block forever
}
