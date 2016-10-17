/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pbft

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric/consensus"
	pb "github.com/hyperledger/fabric/protos"
)

type communicator interface {
	consensus.Communicator
	consensus.Inquirer
}

type broadcaster struct {
	comm communicator

	f                int
	broadcastTimeout time.Duration
	msgChans         map[uint64]chan *sendRequest
	closed           sync.WaitGroup
	closedCh         chan struct{}
}

type sendRequest struct {
	msg  *pb.Message
	done chan bool
}

func newBroadcaster(self uint64, N int, f int, broadcastTimeout time.Duration, c communicator) *broadcaster {
	queueSize := 10 // XXX increase after testing

	chans := make(map[uint64]chan *sendRequest)
	b := &broadcaster{
		comm:             c,
		f:                f,
		broadcastTimeout: broadcastTimeout,
		msgChans:         chans,
		closedCh:         make(chan struct{}),
	}
	for i := 0; i < N; i++ {
		if uint64(i) == self {
			continue
		}
		chans[uint64(i)] = make(chan *sendRequest, queueSize)
	}

	// We do not start the go routines in the above loop to avoid concurrent map read/writes
	for i := 0; i < N; i++ {
		if uint64(i) == self {
			continue
		}
		go b.drainer(uint64(i))
	}

	return b
}

func (b *broadcaster) Close() {
	close(b.closedCh)
	b.closed.Wait()
}

func (b *broadcaster) Wait() {
	b.closed.Wait()
}

func (b *broadcaster) drainerSend(dest uint64, send *sendRequest, successLastTime bool) bool {
	// Note, successLastTime is purely used to avoid flooding the log with unnecessary warning messages when a network problem is encountered
	defer func() {
		b.closed.Done()
	}()
	h, err := getValidatorHandle(dest)
	if err != nil {
		if successLastTime {
			logger.Warningf("could not get handle for replica %d", dest)
		}
		send.done <- false
		return false
	}

	err = b.comm.Unicast(send.msg, h)
	if err != nil {
		if successLastTime {
			logger.Warningf("could not send to replica %d: %v", dest, err)
		}
		send.done <- false
		return false
	}

	send.done <- true
	return true

}

func (b *broadcaster) drainer(dest uint64) {
	successLastTime := false
	destChan, exsit := b.msgChans[dest] // Avoid doing the map lookup every send
	if !exsit {
		logger.Warningf("could not get message channel for replica %d", dest)
		return
	}

	for {
		select {
		case send := <-destChan:
			successLastTime = b.drainerSend(dest, send, successLastTime)
		case <-b.closedCh:
			for {
				// Drain the message channel to free calling waiters before we shut down
				select {
				case send := <-destChan:
					send.done <- false
					b.closed.Done()
				default:
					return
				}
			}
		}
	}
}

func (b *broadcaster) unicastOne(msg *pb.Message, dest uint64, wait chan bool) {
	select {
	case b.msgChans[dest] <- &sendRequest{
		msg:  msg,
		done: wait,
	}:
	default:
		// If this channel is full, we must discard the message and flag it as done
		wait <- false
		b.closed.Done()
	}
}

func (b *broadcaster) send(msg *pb.Message, dest *uint64) error {
	select {
	case <-b.closedCh:
		return fmt.Errorf("broadcaster closed")
	default:
	}

	var destCount int
	var required int
	if dest != nil {
		destCount = 1
		required = 1
	} else {
		destCount = len(b.msgChans)
		required = destCount - b.f
	}

	wait := make(chan bool, destCount)

	if dest != nil {
		b.closed.Add(1)
		b.unicastOne(msg, *dest, wait)
	} else {
		b.closed.Add(len(b.msgChans))
		for i := range b.msgChans {
			b.unicastOne(msg, i, wait)
		}
	}

	succeeded := 0
	timer := time.NewTimer(b.broadcastTimeout)

	// This loop will try to send, until one of:
	// a) the required number of sends succeed
	// b) all sends complete regardless of success
	// c) the timeout expires and the required number of sends have returned
outer:
	for i := 0; i < destCount; i++ {
		select {
		case success := <-wait:
			if success {
				succeeded++
				if succeeded >= required {
					break outer
				}
			}
		case <-timer.C:
			for i := i; i < required; i++ {
				<-wait
			}
			break outer
		}
	}

	return nil
}

func (b *broadcaster) Unicast(msg *pb.Message, dest uint64) error {
	return b.send(msg, &dest)
}

func (b *broadcaster) Broadcast(msg *pb.Message) error {
	return b.send(msg, nil)
}
