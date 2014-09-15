package netchan

import "reflect"

// SelectRecv loops on multiple receivers until it gets a value,
// it blocks until a value is returned or all channels are closed.
// this function doesn't panic.
func SelectRecv(recvs []Receiver) (r Receiver, val interface{}) {
	defer func() {
		if recover() != nil {
			r = nil
			val = nil
		}
	}()
	cases := make([]reflect.SelectCase, len(recvs))
	for i, rch := range recvs {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv}
		switch rch := rch.(type) {
		case *Channel:
			cases[i].Chan = reflect.ValueOf(rch.r)
		case LocalReceiver:
			cases[i].Chan = reflect.ValueOf((<-chan interface{})(rch))
		default:
			panic("SelectRecv only supports Channel and/or LocalReceiver")
		}
	}
	i, v, ok := reflect.Select(cases)
	if ok {
		r = recvs[i]
		val = v.Interface()
		if val, ok := val.(*pkt); ok {
			return r, val.Value
		}
		return
	}
	return
}

// SelectSend loops on multiple senders and returns the first channel that
// was able to send.
// it returns the target that sent the value or returns nil if all channels were closed
// this function doesn't panic.
func SelectSend(targets []Sender, val interface{}) (sch Sender) {
	defer func() {
		if recover() != nil {
			sch = nil
		}
	}()
	cases := make([]reflect.SelectCase, len(targets))
	p := reflect.ValueOf(&pkt{Type: pktData, Value: val})
	for i, sch := range targets {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectSend}
		switch sch := sch.(type) {
		case *Channel:
			cases[i].Chan = reflect.ValueOf(sch.s)
			cases[i].Send = p
		case LocalSender:
			cases[i].Chan = reflect.ValueOf((chan<- interface{})(sch))
			cases[i].Send = reflect.ValueOf(val)
		default:
			panic("SelectSend only supports Channel and/or LocalSender")
		}
	}
	i, _, _ := reflect.Select(cases)
	return targets[i]
}
