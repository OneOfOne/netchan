package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OneOfOne/netchan"
)

var (
	servPort = flag.String("addr", "10101", "server port")
	serv     = flag.Bool("s", false, "if true it will act as a server")
	ch       = netchan.New(1)
)

func dieIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
func main() {
	flag.Parse()
	addr := "127.0.0.1:" + *servPort
	if *serv {
		dieIf(ch.ListenAndServe("tcp4", addr))
		time.Sleep(10 * time.Millisecond)
		for {
			id, msg, st := ch.Recv(true)
			if st != netchan.StatusOK {
				dieIf(fmt.Errorf("invalid recv, dying (%s): [id=%x] %q %s", st, id, msg))
			}
			fmt.Printf("received %q from %x\n", msg, id)
			ch.Send(true, "pong")
		}
	} else {
		dieIf(ch.Connect("tcp4", addr))
		for {
			ch.Send(true, "ping")
			_, msg, st := ch.Recv(true)
			fmt.Printf("received %q from remote\n", msg)
			if st != netchan.StatusOK {
				dieIf(fmt.Errorf("invalid recv, dying (%s): %q", st, msg))
			}
			time.Sleep(1 * time.Second)
		}
	}
}
