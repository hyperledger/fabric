/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/comm"
)

var matchAnything = func(_ interface{}) bool { return true }

// fill two channels.
func TestChannelDeMultiplexer_EvenOddChannels(t *testing.T) {
	demux := comm.NewChannelDemultiplexer()
	evens := demux.AddChannel(func(number interface{}) bool {
		if i, ok := number.(int); ok {
			return i%2 == 0
		}
		return false
	})

	odds := demux.AddChannel(func(number interface{}) bool {
		if i, ok := number.(int); ok {
			return i%2 == 1
		}
		return false
	})
	demux.DeMultiplex("msg") // neither even, nor odd
	for i := 0; i < 20; i++ {
		demux.DeMultiplex(i)
	}
	if len(evens) != 10 || len(odds) != 10 {
		t.Fatalf("filter didn't work, or something got pulled out of the channel buffers, evens:%d, odds:%d", len(evens), len(odds))
	}
	demux.Close()
	demux.Close() // Close is idempotent
	// currently existing channels still work
	for number := range odds {
		if i, ok := number.(int); ok {
			if i%2 != 1 {
				t.Fatal("got an even in my odds")
			}
		}
	}
	for number := range evens {
		if i, ok := number.(int); ok {
			if i%2 != 0 {
				t.Fatal("got an odd in my evens")
			}
		}
	}

	ensureClosedEmptyChannelWithNilReturn(t, evens)
	ensureClosedEmptyChannelWithNilReturn(t, odds)
}

func TestChannelDeMultiplexer_OperationsAfterClose(t *testing.T) {
	demux := comm.NewChannelDemultiplexer()
	demux.Close()
	ch := make(chan struct{})
	matcher := func(_ interface{}) bool { ch <- struct{}{}; return true }
	things := demux.AddChannel(matcher)
	// We should get a closed channel
	ensureClosedEmptyChannelWithNilReturn(t, things)
	// demux is closed, so this should exit without attempting a match.
	demux.DeMultiplex("msg")
	ensureClosedEmptyChannelWithNilReturn(t, things)
	select {
	case <-ch:
		t.Fatal("matcher should not have been called")
	default:
	}
}

func TestChannelDeMultiplexer_ShouldCloseWithFullChannel(t *testing.T) {
	demux := comm.NewChannelDemultiplexer()
	demux.AddChannel(matchAnything)
	for i := 0; i < 10; i++ {
		demux.DeMultiplex(i)
	}

	// there is no guarantee that the goroutine runs before
	// Close() is called, so if this were to fail, it would be
	// arbitrarily based on the scheduler
	finished := make(chan struct{})
	go func() {
		demux.DeMultiplex(0) // filled channel, but in the background
		close(finished)
	}()
	demux.Close() // does not hang
	select {
	case <-finished: // this forces the goroutine to run to completion
	case <-time.After(time.Second):
		t.Fatal("DeMultiplex should return when demux.Close() is called")
	}
}

// run the operations in various orders to make sure there are no
// missing unlocks that would block anything
func TestChannelDeMultiplexer_InterleaveOperations(t *testing.T) {
	finished := make(chan struct{})
	go func() {
		demux := comm.NewChannelDemultiplexer()
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.Close()
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.Close()
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.AddChannel(matchAnything)
		demux.DeMultiplex("msg")
		demux.Close()
		close(finished)
	}()

	select {
	case <-time.After(time.Second):
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		t.Fatal("timed out waiting for operations to complete, may be deadlock")
	case <-finished:
	}
}

func ensureClosedEmptyChannelWithNilReturn(t *testing.T, things <-chan interface{}) {
	if thing, openChannel := <-things; openChannel || thing != nil {
		t.Fatalf("channel should be closed and should not get non-nil from closed empty channel")
	}
}
