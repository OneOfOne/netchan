package netchan

import "runtime"

// SelectRecv loops on multiple receivers until it gets a value, it will
// return nil, nil if block==false and there was nothing to receive
func SelectRecv(block bool, recvs []Receiver) (Receiver, interface{}) {
	for {
		for _, ch := range recvs {
			_, val, st := ch.Recv(false)
			if st == StatusOK {
				return ch, val
			}
		}
		if !block {
			return nil, nil
		}
		runtime.Gosched()
	}
}

// SelectSend loops on multiple senders and returns the first channel that
// was able to send or nil if block == false and nothing was sent.
func SelectSend(block bool, targets []Sender, val interface{}) Sender {
	for {
		for _, ch := range targets {
			st := ch.Send(false, val)
			if st == StatusOK {
				return ch
			}
		}
		if !block {
			return nil
		}
		runtime.Gosched()
	}
}
