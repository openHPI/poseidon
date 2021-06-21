package tests

import "time"

// ChannelReceivesSomething waits timeout seconds for something to be received from channel ch.
// If something is received, it returns true. If the timeout expires without receiving anything, it return false.
func ChannelReceivesSomething(ch chan bool, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}
