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

package solo

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
)

var genesisBlock *ab.Block

func init() {
	bootstrapper := static.New()
	var err error
	genesisBlock, err = bootstrapper.GenesisBlock()
	if err != nil {
		panic("Error intializing static bootstrap genesis block")
	}
}

type mockB struct {
	grpc.ServerStream
	recvChan chan *ab.BroadcastMessage
	sendChan chan *ab.BroadcastResponse
}

func newMockB() *mockB {
	return &mockB{
		recvChan: make(chan *ab.BroadcastMessage),
		sendChan: make(chan *ab.BroadcastResponse),
	}
}

func (m *mockB) Send(br *ab.BroadcastResponse) error {
	m.sendChan <- br
	return nil
}

func (m *mockB) Recv() (*ab.BroadcastMessage, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return msg, fmt.Errorf("Channel closed")
	}
	return msg, nil
}

func TestQueueOverflow(t *testing.T) {
	bs := newPlainBroadcastServer(2, 1, time.Second, nil) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused)
	m := newMockB()
	b := newBroadcaster(bs)
	go b.queueBroadcastMessages(m)
	defer close(m.recvChan)

	bs.halt()

	for i := 0; i < 2; i++ {
		m.recvChan <- &ab.BroadcastMessage{Data: []byte("Some bytes")}
		reply := <-m.sendChan
		if reply.Status != ab.Status_SUCCESS {
			t.Fatalf("Should have successfully queued the message")
		}
	}

	m.recvChan <- &ab.BroadcastMessage{Data: []byte("Some bytes")}
	reply := <-m.sendChan
	if reply.Status != ab.Status_SERVICE_UNAVAILABLE {
		t.Fatalf("Should not have successfully queued the message")
	}

}

func TestMultiQueueOverflow(t *testing.T) {
	bs := newPlainBroadcastServer(2, 1, time.Second, nil) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused)
	// m := newMockB()
	ms := []*mockB{newMockB(), newMockB(), newMockB()}

	for _, m := range ms {
		b := newBroadcaster(bs)
		go b.queueBroadcastMessages(m)
		defer close(m.recvChan)
	}

	for _, m := range ms {
		for i := 0; i < 2; i++ {
			m.recvChan <- &ab.BroadcastMessage{Data: []byte("Some bytes")}
			reply := <-m.sendChan
			if reply.Status != ab.Status_SUCCESS {
				t.Fatalf("Should have successfully queued the message")
			}
		}
	}

	for _, m := range ms {
		m.recvChan <- &ab.BroadcastMessage{Data: []byte("Some bytes")}
		reply := <-m.sendChan
		if reply.Status != ab.Status_SERVICE_UNAVAILABLE {
			t.Fatalf("Should not have successfully queued the message")
		}
	}
}

func TestEmptyBroadcastMessage(t *testing.T) {
	bs := newPlainBroadcastServer(2, 1, time.Second, nil) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused)
	m := newMockB()
	defer close(m.recvChan)
	go bs.handleBroadcast(m)

	m.recvChan <- &ab.BroadcastMessage{}
	reply := <-m.sendChan
	if reply.Status != ab.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected the null message")
	}

}

func TestEmptyBatch(t *testing.T) {
	bs := newPlainBroadcastServer(2, 1, time.Millisecond, ramledger.New(10, genesisBlock))
	time.Sleep(100 * time.Millisecond) // Note, this is not a race, as worst case, the timer does not expire, and the test still passes
	if bs.rl.(rawledger.Reader).Height() != 1 {
		t.Fatalf("Expected no new blocks created")
	}
}

func TestFilledBatch(t *testing.T) {
	batchSize := 2
	bs := newBroadcastServer(0, batchSize, time.Hour, ramledger.New(10, genesisBlock))
	defer bs.halt()
	messages := 11 // Sending 11 messages, with a batch size of 2, ensures the 10th message is processed before we proceed for 5 blocks
	for i := 0; i < messages; i++ {
		bs.sendChan <- &ab.BroadcastMessage{Data: []byte("Some bytes")}
	}
	expected := uint64(1 + messages/batchSize)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}
}
