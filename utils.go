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
