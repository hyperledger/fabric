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
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
)

// MagicLargestWindow is used as the default max window size for initializing the deliver service
const MagicLargestWindow int = 1000

type mockD struct {
	grpc.ServerStream
	recvChan chan *ab.DeliverUpdate
	sendChan chan *ab.DeliverResponse
}

func newMockD() *mockD {
	return &mockD{
		recvChan: make(chan *ab.DeliverUpdate),
		sendChan: make(chan *ab.DeliverResponse),
	}
}

func (m *mockD) Send(br *ab.DeliverResponse) error {
	m.sendChan <- br
	return nil
}

func (m *mockD) Recv() (*ab.DeliverUpdate, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return msg, fmt.Errorf("Channel closed")
	}
	return msg, nil
}

func TestOldestSeek(t *testing.T) {
	ledgerSize := 5
	rl := ramledger.New(ledgerSize, genesisBlock)
	for i := 1; i < ledgerSize; i++ {
		rl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_OLDEST}}}

	count := 0
	for {
		select {
		case deliverReply := <-m.sendChan:
			if deliverReply.GetError() != ab.Status_SUCCESS {
				t.Fatalf("Received an error on the reply channel")
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
		if count == ledgerSize {
			break
		}
	}
}

func TestNewestSeek(t *testing.T) {
	ledgerSize := 5
	rl := ramledger.New(ledgerSize, genesisBlock)
	for i := 1; i < ledgerSize; i++ {
		rl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_NEWEST}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != ab.Status_SUCCESS {
			t.Fatalf("Received an error on the reply channel")
		}

		if blockReply.GetBlock().Number != uint64(ledgerSize-1) {
			t.Fatalf("Expected only the most recent block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestSpecificSeek(t *testing.T) {
	ledgerSize := 5
	rl := ramledger.New(ledgerSize, genesisBlock)
	for i := 1; i < ledgerSize; i++ {
		rl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_SPECIFIED, SpecifiedNumber: uint64(ledgerSize - 1)}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != ab.Status_SUCCESS {
			t.Fatalf("Received an error on the reply channel")
		}

		if blockReply.GetBlock().Number != uint64(ledgerSize-1) {
			t.Fatalf("Expected only to get block 4")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadSeek(t *testing.T) {
	ledgerSize := 5
	rl := ramledger.New(ledgerSize, genesisBlock)
	for i := 1; i < 2*ledgerSize; i++ {
		rl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_SPECIFIED, SpecifiedNumber: uint64(ledgerSize - 1)}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != ab.Status_NOT_FOUND {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_SPECIFIED, SpecifiedNumber: uint64(3 * ledgerSize)}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != ab.Status_NOT_FOUND {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadWindow(t *testing.T) {
	ledgerSize := 5
	rl := ramledger.New(ledgerSize, genesisBlock)

	m := newMockD()
	defer close(m.recvChan)
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow) * 2, Start: ab.SeekInfo_OLDEST}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != ab.Status_BAD_REQUEST {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestAck(t *testing.T) {
	ledgerSize := 10
	windowSize := uint64(2)
	rl := ramledger.New(ledgerSize, genesisBlock)
	for i := 1; i < ledgerSize; i++ {
		rl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := newDeliverServer(rl, MagicLargestWindow)

	go ds.handleDeliver(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: windowSize, Start: ab.SeekInfo_OLDEST}}}

	count := uint64(0)
	for {
		select {
		case blockReply := <-m.sendChan:
			if blockReply.GetError() != ab.Status_SUCCESS {
				t.Fatalf("Received an error on the reply channel")
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
		if count == windowSize {
			select {
			case <-m.sendChan:
				t.Fatalf("Window size exceeded")
			default:
			}
		}

		if count%windowSize == 0 {
			m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Acknowledgement{Acknowledgement: &ab.Acknowledgement{Number: count}}}
		}

		if count == uint64(ledgerSize) {
			break
		}
	}
}
