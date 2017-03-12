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

	mockconfigvaluesorderer "github.com/hyperledger/fabric/common/mocks/configvalues/channel/orderer"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mockmultichain "github.com/hyperledger/fabric/orderer/mocks/multichain"
	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
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

func TestEmptyBatch(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfigvaluesorderer.SharedConfig{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	bs.Halt()
	select {
	case <-support.Batches:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestBatchTimer(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1ms")
	support := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfigvaluesorderer.SharedConfig{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because of batch timer expiration but did not")
	}

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	select {
	case <-support.Batches:
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the timer was not appopriately reset")
	}

	bs.Halt()
	select {
	case <-support.Batches:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestBatchTimerHaltOnFilledBatch(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1h")
	support := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfigvaluesorderer.SharedConfig{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)

	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	support.BlockCutterVal.CutNext = true
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because the batch was filled, but did not")
	}

	// Change the batch timeout to be near instant, if the timer was not reset, it will still be waiting an hour
	bs.batchTimeout = time.Millisecond

	support.BlockCutterVal.CutNext = false
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Batches:
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
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		SharedConfigVal: &mockconfigvaluesorderer.SharedConfig{BatchTimeoutVal: batchTimeout},
	}
	defer close(support.BlockCutterVal.Block)
	bs := newChain(support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.BlockCutterVal)
	support.BlockCutterVal.IsolatedTx = true
	syncQueueMessage(testMessage, bs, support.BlockCutterVal)

	select {
	case <-support.Batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected two blocks to be cut but never got the first")
	}

	select {
	case <-support.Batches:
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
