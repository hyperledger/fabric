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

package state

import (
	"crypto/rand"
	"fmt"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
	"sync"
	"sync/atomic"
)

func uuid() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}
	uuid[8] = uuid[8] &^ 0xc0 | 0x80

	uuid[6] = uuid[6] &^ 0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func randomPayloadWithSeqNum(seqNum uint64) (*proto.Payload, error) {
	data := make([]byte, 64)
	_, err := rand.Read(data)
	if err != nil {
		return nil, err
	}
	uuid, err := uuid()
	if err != nil {
		return nil, err
	}

	return &proto.Payload{seqNum, uuid, data}, nil
}

func TestNewPayloadsBuffer(t *testing.T) {
	payloadsBuffer := NewPayloadsBuffer(10)
	assert.Equal(t, payloadsBuffer.Next(), uint64(10))
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
	assert.Equal(t, buffer.Next(), uint64(5))
	t.Log("Check block buffer size")
	assert.Equal(t, buffer.Size(), 0)

	// Adding new payload with seq. number equal to top
	// payload should not be added
	payload, err = randomPayloadWithSeqNum(5)
	if err != nil {
		t.Fatal("Wasn't able to generate random payload for test")
	}

	t.Log("Pushing new payload into buffer")
	buffer.Push(payload)
	t.Log("Getting next block sequence number")
	assert.Equal(t, buffer.Next(), uint64(5))
	t.Log("Check block buffer size")
	assert.Equal(t, buffer.Size(), 1)
}

func TestPayloadsBufferImpl_Ready(t *testing.T) {
	fin := make(chan struct{})
	buffer := NewPayloadsBuffer(1)
	assert.Equal(t, buffer.Next(), uint64(1))

	go func() {
		<-buffer.Ready()
		fin <- struct{}{}
	}()

	time.AfterFunc(100 * time.Millisecond, func() {
		payload, err := randomPayloadWithSeqNum(1)

		if err != nil {
			t.Fatal("Wasn't able to generate random payload for test")
		}
		buffer.Push(payload)
	})

	select {
	case <-fin:
		payload := buffer.Pop()
		assert.Equal(t, payload.SeqNum, uint64(1))
	case <-time.After(500 * time.Millisecond):
		t.Fail()
	}
}

func TestPayloadsBufferImpl_MinAvail(t *testing.T) {
	buffer := NewPayloadsBuffer(1)

	assert.Equal(t, buffer.Next(), uint64(1))

	// Buffer is empty no messages expected,
	// hence no min shoyld be value available
	_, err := buffer.MinAvail()
	assert.Error(t, err)

	pushNewRandomPayload(t, buffer, 10)

	min, err := buffer.MinAvail()
	assert.NoError(t, err)
	assert.Equal(t, min, uint64(10))

	pushNewRandomPayload(t, buffer, 17)

	// Presence of payload w/ sequence number 17 should not affect the minimum available block
	min, err = buffer.MinAvail()
	assert.NoError(t, err)
	assert.Equal(t, min, uint64(10))

	// Add new block w/ lower sequence number
	pushNewRandomPayload(t, buffer, 6)

	min, err = buffer.MinAvail()
	assert.NoError(t, err)
	// New sequence number now should be the minimum
	assert.Equal(t, min, uint64(6))
}

func TestPayloadsBufferImpl_MinAvail2(t *testing.T) {
	buffer := NewPayloadsBuffer(1)

	assert.Equal(t, buffer.Next(), uint64(1))

	_, err := buffer.MinAvail()
	assert.Error(t, err)

	pushNewRandomPayload(t, buffer, 3)
	min, err := buffer.MinAvail()
	assert.NoError(t, err)
	assert.Equal(t, min, uint64(3))

	pushNewRandomPayload(t, buffer, 1)
	min, err = buffer.MinAvail()
	assert.NoError(t, err)
	assert.Equal(t, min, uint64(1))

	done := sync.WaitGroup{}
	done.Add(1)

	go func() {
		select {
		case <-buffer.Ready():
			{
				// Once payload is ready extract it
				assert.Equal(t, buffer.Next(), uint64(1))
				payload := buffer.Pop()
				assert.Equal(t, payload.SeqNum, uint64(1))

				// Next min sequence number has to be 3
				min, err = buffer.MinAvail()
				assert.NoError(t, err)
				assert.Equal(t, min, uint64(3))
			}
		case <-time.After(500 * time.Millisecond):
			{
				t.Fatalf("Expected to receive notification with next payload")
			}
		}
		done.Done()
	}()

	// Wait to make sure that payload was extracted
	done.Wait()
}

// Test to push several concurrent blocks into the buffer
// with same sequence number, only one expected to succeed
func TestPayloadsBufferImpl_ConcurrentPush(t *testing.T) {

	// Test setup, next block num to expect and
	// how many concurrent pushes to simulate
	nextSeqNum := uint64(7)
	concurrency := 10

	buffer := NewPayloadsBuffer(nextSeqNum)
	assert.Equal(t, buffer.Next(), uint64(nextSeqNum))

	startWG := sync.WaitGroup{}
	startWG.Add(1)

	finishWG := sync.WaitGroup{}
	finishWG.Add(concurrency)

	payload, err := randomPayloadWithSeqNum(nextSeqNum)
	assert.NoError(t, err)

	errors := make([]error, 0)

	ready := int32(0)
	go func() {

		// Wait for next expected block to arrive
		<-buffer.Ready()
		atomic.AddInt32(&ready, 1)
	}()

	for i := 0; i < concurrency; i++ {
		go func() {
			startWG.Wait()
			errors = append(errors, buffer.Push(payload))
			finishWG.Done()
		}();
	}
	startWG.Done()
	finishWG.Wait()

	success := 0

	// Only one push attempt expected to succeed
	for _, err := range errors {
		if err == nil {
			success ++
		}
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&ready))
	assert.Equal(t, 1, success)
	// Buffer size has to be only one
	assert.Equal(t, 1, buffer.Size())
}

func pushNewRandomPayload(t *testing.T, b PayloadsBuffer, seqNum uint64) {
	// Add new block w/ lower sequence number
	payload, err := randomPayloadWithSeqNum(seqNum);
	assert.NoError(t, err)
	err = b.Push(payload)
	assert.NoError(t, err)
}