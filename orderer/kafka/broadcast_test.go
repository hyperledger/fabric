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

package kafka

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
)

func TestBroadcastResponse(t *testing.T) {
	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	// Send a message to the orderer
	go func() {
		mbs.incoming <- &cb.Envelope{Payload: []byte("single message")}
	}()

	for {
		select {
		case reply := <-mbs.outgoing:
			if reply.Status != cb.Status_SUCCESS {
				t.Fatal("Client should have received a SUCCESS reply")
			}
			return
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Should have received a broadcast reply by the orderer by now")
		}
	}
}

func TestBroadcastBatch(t *testing.T) {
	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	// Pump a batch's worth of messages into the system
	go func() {
		for i := 0; i < int(testConf.General.BatchSize); i++ {
			mbs.incoming <- &cb.Envelope{Payload: []byte("message " + strconv.Itoa(i))}
		}
	}()

	// Ignore the broadcast replies as they have been tested elsewhere
	for i := 0; i < int(testConf.General.BatchSize); i++ {
		<-mbs.outgoing
	}

	for {
		select {
		case in := <-disk:
			block := new(cb.Block)
			err := proto.Unmarshal(in, block)
			if err != nil {
				t.Fatal("Expected a block on the broker's disk")
			}
			if len(block.Data.Data) != int(testConf.General.BatchSize) {
				t.Fatalf("Expected block to have %d messages instead of %d", testConf.General.BatchSize, len(block.Data.Data))
			}
			return
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Should have received a block by now")
		}
	}
}

// If the capacity of the response queue is less than the batch size,
// then if the response queue overflows, the order should not be able
// to send back a block to the client. (Sending replies and adding
// messages to the about-to-be-sent block happens on the same routine.)
/* func TestBroadcastResponseQueueOverflow(t *testing.T) {

	// Make sure that the response queue is less than the batch size
	originalQueueSize := testConf.General.QueueSize
	defer func() { testConf.General.QueueSize = originalQueueSize }()
	testConf.General.QueueSize = testConf.General.BatchSize - 1

	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	// Force the response queue to overflow by blocking the broadcast stream's Send() method
	mbs.closed = true
	defer func() { mbs.closed = false }()

	// Pump a batch's worth of messages into the system
	go func() {
		for i := 0; i < int(testConf.General.BatchSize); i++ {
			mbs.incoming <- &cb.Envelope{Payload: []byte("message " + strconv.Itoa(i))}
		}
	}()

loop:
	for {
		select {
		case <-mbs.outgoing:
			t.Fatal("Client shouldn't have received anything from the orderer")
		case <-time.After(testConf.General.BatchTimeout + timePadding):
			break loop // This is the success path
		}
	}
} */

func TestBroadcastIncompleteBatch(t *testing.T) {
	if testConf.General.BatchSize <= 1 {
		t.Skip("Skipping test as it requires a batchsize > 1")
	}

	messageCount := int(testConf.General.BatchSize) - 1

	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	// Pump less than batchSize messages into the system
	go func() {
		for i := 0; i < messageCount; i++ {
			payload, _ := proto.Marshal(&cb.Payload{Data: []byte("message " + strconv.Itoa(i))})
			mbs.incoming <- &cb.Envelope{Payload: payload}
		}
	}()

	// Ignore the broadcast replies as they have been tested elsewhere
	for i := 0; i < messageCount; i++ {
		<-mbs.outgoing
	}

	for {
		select {
		case in := <-disk:
			block := new(cb.Block)
			err := proto.Unmarshal(in, block)
			if err != nil {
				t.Fatal("Expected a block on the broker's disk")
			}
			if len(block.Data.Data) != messageCount {
				t.Fatalf("Expected block to have %d messages instead of %d", messageCount, len(block.Data.Data))
			}
			return
		case <-time.After(testConf.General.BatchTimeout + timePadding):
			t.Fatal("Should have received a block by now")
		}
	}
}

func TestBroadcastConsecutiveIncompleteBatches(t *testing.T) {
	if testConf.General.BatchSize <= 1 {
		t.Skip("Skipping test as it requires a batchsize > 1")
	}

	var once sync.Once

	messageCount := int(testConf.General.BatchSize) - 1

	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	for i := 0; i < 2; i++ {
		// Pump less than batchSize messages into the system
		go func() {
			for i := 0; i < messageCount; i++ {
				payload, _ := proto.Marshal(&cb.Payload{Data: []byte("message " + strconv.Itoa(i))})
				mbs.incoming <- &cb.Envelope{Payload: payload}
			}
		}()

		// Ignore the broadcast replies as they have been tested elsewhere
		for i := 0; i < messageCount; i++ {
			<-mbs.outgoing
		}

		once.Do(func() {
			<-disk // First incomplete block, tested elsewhere
		})
	}

	for {
		select {
		case in := <-disk:
			block := new(cb.Block)
			err := proto.Unmarshal(in, block)
			if err != nil {
				t.Fatal("Expected a block on the broker's disk")
			}
			if len(block.Data.Data) != messageCount {
				t.Fatalf("Expected block to have %d messages instead of %d", messageCount, len(block.Data.Data))
			}
			return
		case <-time.After(testConf.General.BatchTimeout + timePadding):
			t.Fatal("Should have received a block by now")
		}
	}
}

func TestBroadcastBatchAndQuitEarly(t *testing.T) {
	disk := make(chan []byte)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, disk)
	defer testClose(t, mb)

	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	// Pump a batch's worth of messages into the system
	go func() {
		for i := 0; i < int(testConf.General.BatchSize); i++ {
			mbs.incoming <- &cb.Envelope{Payload: []byte("message " + strconv.Itoa(i))}
		}
	}()

	// In contrast to TestBroadcastBatch, do not receive any replies.
	// This simulates the case where you quit early (though you would
	// most likely still get replies in a real world scenario, as long
	// as you don't receive all of them we're on the same page).
	for !mbs.CloseOut() {
	}

	for {
		select {
		case in := <-disk:
			block := new(cb.Block)
			err := proto.Unmarshal(in, block)
			if err != nil {
				t.Fatal("Expected a block on the broker's disk")
			}
			if len(block.Data.Data) != int(testConf.General.BatchSize) {
				t.Fatalf("Expected block to have %d messages instead of %d", testConf.General.BatchSize, len(block.Data.Data))
			}
			return
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Should have received a block by now")
		}
	}
}

func TestBroadcastClose(t *testing.T) {
	errChan := make(chan error)

	mb := mockNewBroadcaster(t, testConf, oldestOffset, make(chan []byte))
	mbs := newMockBroadcastStream(t)
	go func() {
		if err := mb.Broadcast(mbs); err != nil {
			t.Fatal("Broadcast error:", err)
		}
	}()

	go func() {
		errChan <- mb.Close()
	}()

	for {
		select {
		case err := <-errChan:
			if err != nil {
				t.Fatal("Error when closing the broadcaster:", err)
			}
			return
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Broadcaster should have closed its producer by now")
		}
	}

}
