/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
)

func init() {
	util.SetupTestLogging()
}

func randomPayloadWithSeqNum(seqNum uint64) (*proto.Payload, error) {
	data := make([]byte, 64)
	_, err := rand.Read(data)
	if err != nil {
		return nil, err
	}
	return &proto.Payload{
		SeqNum: seqNum,
		Data:   data,
	}, nil
}

func TestNewPayloadsBuffer(t *testing.T) {
	payloadsBuffer := NewPayloadsBuffer(10)
	require.Equal(t, payloadsBuffer.Next(), uint64(10))
}

func TestPayloadsBufferImpl_Push(t *testing.T) {
	buffer := NewPayloadsBuffer(5)

	payload, err := randomPayloadWithSeqNum(4)
	if err != nil {
		t.Fatal("Wasn't able to generate random payload for test")
	}

	t.Log("Pushing new payload into buffer")
	buffer.Push(payload)

	// Payloads with sequence number less than buffer top
	// index should not be accepted
	t.Log("Getting next block sequence number")
	require.Equal(t, buffer.Next(), uint64(5))
	t.Log("Check block buffer size")
	require.Equal(t, buffer.Size(), 0)

	// Adding new payload with seq. number equal to top
	// payload should not be added
	payload, err = randomPayloadWithSeqNum(5)
	if err != nil {
		t.Fatal("Wasn't able to generate random payload for test")
	}

	t.Log("Pushing new payload into buffer")
	buffer.Push(payload)
	t.Log("Getting next block sequence number")
	require.Equal(t, buffer.Next(), uint64(5))
	t.Log("Check block buffer size")
	require.Equal(t, buffer.Size(), 1)
}

func TestPayloadsBufferImpl_Ready(t *testing.T) {
	fin := make(chan struct{})
	buffer := NewPayloadsBuffer(1)
	require.Equal(t, buffer.Next(), uint64(1))

	go func() {
		<-buffer.Ready()
		fin <- struct{}{}
	}()

	time.AfterFunc(100*time.Millisecond, func() {
		payload, err := randomPayloadWithSeqNum(1)
		if err != nil {
			t.Fatal("Wasn't able to generate random payload for test")
		}
		buffer.Push(payload)
	})

	select {
	case <-fin:
		payload := buffer.Pop()
		require.Equal(t, payload.SeqNum, uint64(1))
	case <-time.After(500 * time.Millisecond):
		t.Fail()
	}
}

// Test to push several concurrent blocks into the buffer
// with same sequence number, only one expected to succeed
func TestPayloadsBufferImpl_ConcurrentPush(t *testing.T) {
	// Test setup, next block num to expect and
	// how many concurrent pushes to simulate
	nextSeqNum := uint64(7)
	concurrency := 10

	buffer := NewPayloadsBuffer(nextSeqNum)
	require.Equal(t, buffer.Next(), uint64(nextSeqNum))

	startWG := sync.WaitGroup{}
	startWG.Add(1)

	finishWG := sync.WaitGroup{}
	finishWG.Add(concurrency)

	payload, err := randomPayloadWithSeqNum(nextSeqNum)
	require.NoError(t, err)

	ready := int32(0)
	readyWG := sync.WaitGroup{}
	readyWG.Add(1)
	go func() {
		// Wait for next expected block to arrive
		<-buffer.Ready()
		atomic.AddInt32(&ready, 1)
		readyWG.Done()
	}()

	for i := 0; i < concurrency; i++ {
		go func() {
			buffer.Push(payload)
			startWG.Wait()
			finishWG.Done()
		}()
	}
	startWG.Done()
	finishWG.Wait()

	readyWG.Wait()
	require.Equal(t, int32(1), atomic.LoadInt32(&ready))
	// Buffer size has to be only one
	require.Equal(t, 1, buffer.Size())
}

// Tests the scenario where payload pushes and pops are interleaved after a Ready() signal.
func TestPayloadsBufferImpl_Interleave(t *testing.T) {
	buffer := NewPayloadsBuffer(1)
	require.Equal(t, buffer.Next(), uint64(1))

	//
	// First two sequences arrives and the buffer is emptied without interleave.
	//
	// This is also an example of the produce/consumer pattern in Fabric.
	// Producer:
	//
	// Payloads are pushed into the buffer. These payloads can be out of order.
	// When the buffer has a sequence of payloads ready (in order), it fires a signal
	// on it's Ready() channel.
	//
	// The consumer waits for the signal and then drains all ready payloads.

	payload, err := randomPayloadWithSeqNum(1)
	require.NoError(t, err, "generating random payload failed")
	buffer.Push(payload)

	payload, err = randomPayloadWithSeqNum(2)
	require.NoError(t, err, "generating random payload failed")
	buffer.Push(payload)

	select {
	case <-buffer.Ready():
	case <-time.After(500 * time.Millisecond):
		t.Error("buffer wasn't ready after 500 ms for first sequence")
	}

	// The consumer empties the buffer.
	for payload := buffer.Pop(); payload != nil; payload = buffer.Pop() {
	}

	// The buffer isn't ready since no new sequences have come since emptying the buffer.
	select {
	case <-buffer.Ready():
		t.Error("buffer should not be ready as no new sequences have come")
	case <-time.After(500 * time.Millisecond):
	}

	//
	// Next sequences are incoming at the same time the buffer is being emptied by the consumer.
	//
	payload, err = randomPayloadWithSeqNum(3)
	require.NoError(t, err, "generating random payload failed")
	buffer.Push(payload)

	select {
	case <-buffer.Ready():
	case <-time.After(500 * time.Millisecond):
		t.Error("buffer wasn't ready after 500 ms for second sequence")
	}
	payload = buffer.Pop()
	require.NotNil(t, payload, "payload should not be nil")

	// ... Block processing now happens on sequence 3.

	// In the mean time, sequence 4 is pushed into the queue.
	payload, err = randomPayloadWithSeqNum(4)
	require.NoError(t, err, "generating random payload failed")
	buffer.Push(payload)

	// ... Block processing completes on sequence 3, the consumer loop grabs the next one (4).
	payload = buffer.Pop()
	require.NotNil(t, payload, "payload should not be nil")

	// In the mean time, sequence 5 is pushed into the queue.
	payload, err = randomPayloadWithSeqNum(5)
	require.NoError(t, err, "generating random payload failed")
	buffer.Push(payload)

	// ... Block processing completes on sequence 4, the consumer loop grabs the next one (5).
	payload = buffer.Pop()
	require.NotNil(t, payload, "payload should not be nil")

	//
	// Now we see that goroutines are building up due to the interleaved push and pops above.
	//
	select {
	case <-buffer.Ready():
		//
		// Should be error - no payloads are ready
		//
		t.Log("buffer ready (1) -- should be error")
		t.Fail()
	case <-time.After(500 * time.Millisecond):
		t.Log("buffer not ready (1)")
	}
	payload = buffer.Pop()
	t.Logf("payload: %v", payload)
	require.Nil(t, payload, "payload should be nil")

	select {
	case <-buffer.Ready():
		//
		// Should be error - no payloads are ready
		//
		t.Log("buffer ready (2) -- should be error")
		t.Fail()
	case <-time.After(500 * time.Millisecond):
		t.Log("buffer not ready (2)")
	}
	payload = buffer.Pop()
	require.Nil(t, payload, "payload should be nil")
	t.Logf("payload: %v", payload)

	select {
	case <-buffer.Ready():
		t.Error("buffer ready (3)")
	case <-time.After(500 * time.Millisecond):
		t.Log("buffer not ready (3) -- good")
	}
}
