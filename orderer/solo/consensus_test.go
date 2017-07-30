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
	"testing"
	"time"

	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mockmultichain "github.com/hyperledger/fabric/orderer/mocks/multichain"
	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var testMessage = &cb.Envelope{Payload: []byte("TEST_MESSAGE")}

func syncQueueMessage(msg *cb.Envelope, chain *chain, bc *mockblockcutter.Receiver) {
	chain.Enqueue(msg)
	bc.Block <- struct{}{}
}

type waitableGo struct {
	done chan struct{}
}

func goWithWait(target func()) *waitableGo {
	wg := &waitableGo{
		done: make(chan struct{}),
	}
	go func() {
		target()
		close(wg.done)
	}()
	return wg
}

// This test checks that if consenter is halted before a timer fires, nothing is actually written.
func TestHaltBeforeTimeout(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	bs.Halt()
	select {
	case <-support.Blocks:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestStart(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	close(support.BlockCutterVal.Block)
	bs, _ := New().HandleChain(support, nil)
	bs.Start()
	defer bs.Halt()

	support.BlockCutterVal.CutNext = true
	bs.Enqueue(testMessage)
	select {
	case <-support.Blocks:
	case <-bs.Errored():
		t.Fatalf("Expected not to exit")
	}
}

func TestEnqueueAfterHalt(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	bs.Halt()
	assert.False(t, bs.Enqueue(testMessage), "Enqueue should not be accepted after halt")
	select {
	case <-bs.Errored():
	default:
		t.Fatalf("Expected Errored to be closed by halt")
	}
}

func TestBatchTimer(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because of batch timer expiration but did not")
	}

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the timer was not appopriately reset")
	}

	support.SharedConfigVal.BatchTimeoutVal, _ = time.ParseDuration("10s")
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	select {
	case <-support.Blocks:
		t.Fatalf("Created another batch, indicating that the timer was not appopriately re-read")
	case <-time.After(100 * time.Millisecond):
	}

	bs.Halt()
	select {
	case <-support.Blocks:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestBatchTimerHaltOnFilledBatch(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1h")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)

	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	support.BlockCutterVal.CutNext = true
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because the batch was filled, but did not")
	}

	// Change the batch timeout to be near instant, if the timer was not reset, it will still be waiting an hour
	support.SharedConfigVal.BatchTimeoutVal = time.Millisecond

	support.BlockCutterVal.CutNext = false
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the old timer was still running")
	}

	bs.Halt()
	select {
	case <-time.After(time.Second):
		t.Fatalf("Should have exited")
	case <-wg.done:
	}
}

func TestConfigStyleMultiBatch(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1h")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	support.BlockCutterVal.IsolatedTx = true
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Expected two blocks to be cut but never got the first")
	}

	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Expected the config type tx to create two blocks, but only go the first")
	}

	bs.Halt()
	select {
	case <-time.After(time.Second):
		t.Fatalf("Should have exited")
	case <-wg.done:
	}
}

// This test checks that solo consenter could recover from an erroneous situation
// where empty batch is cut
func TestRecoverFromError(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Blocks:          make(chan *cb.Block),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfig.Orderer{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	_ = goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	support.BlockCutterVal.CurBatch = nil

	select {
	case <-support.Blocks:
		t.Fatalf("Expected no invocations of Append")
	case <-time.After(2 * time.Millisecond):
	}

	support.BlockCutterVal.CutNext = true
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	select {
	case <-support.Blocks:
	case <-time.After(time.Second):
		t.Fatalf("Expected block to be cut")
	}
}
