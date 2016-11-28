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

	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
)

type mockBlockCutter struct {
	queueNext  bool // Ordered returns nil false when not set to true
	isolatedTx bool // Ordered returns [][]{curBatch, []{newTx}}, true when set to true
	cutNext    bool // Ordered returns [][]{append(curBatch, newTx)}, true when set to true
	curBatch   []*cb.Envelope
	block      chan struct{}
}

func newMockBlockCutter() *mockBlockCutter {
	return &mockBlockCutter{
		queueNext:  true,
		isolatedTx: false,
		cutNext:    false,
		block:      make(chan struct{}),
	}
}

func noopCommitters(size int) []filter.Committer {
	res := make([]filter.Committer, size)
	for i := range res {
		res[i] = filter.NoopCommitter
	}
	return res
}

func (mbc *mockBlockCutter) Ordered(env *cb.Envelope) ([][]*cb.Envelope, [][]filter.Committer, bool) {
	defer func() {
		<-mbc.block
	}()

	if !mbc.queueNext {
		logger.Debugf("mockBlockCutter: Not queueing message")
		return nil, nil, false
	}

	if mbc.isolatedTx {
		logger.Debugf("mockBlockCutter: Returning dual batch")
		res := [][]*cb.Envelope{mbc.curBatch, []*cb.Envelope{env}}
		mbc.curBatch = nil
		return res, [][]filter.Committer{noopCommitters(len(res[0])), noopCommitters(len(res[1]))}, true
	}

	mbc.curBatch = append(mbc.curBatch, env)

	if mbc.cutNext {
		logger.Debugf("mockBlockCutter: Returning regular batch")
		res := [][]*cb.Envelope{mbc.curBatch}
		mbc.curBatch = nil
		return res, [][]filter.Committer{noopCommitters(len(res))}, true
	}

	logger.Debugf("mockBlockCutter: Appending to batch")
	return nil, nil, true
}

func (mbc *mockBlockCutter) Cut() ([]*cb.Envelope, []filter.Committer) {
	logger.Debugf("mockBlockCutter: Cutting batch")
	res := mbc.curBatch
	mbc.curBatch = nil
	return res, noopCommitters(len(res))
}

type mockConsenterSupport struct {
	cutter  *mockBlockCutter
	batches chan []*cb.Envelope
}

func (mcs *mockConsenterSupport) BlockCutter() blockcutter.Receiver {
	return mcs.cutter
}
func (mcs *mockConsenterSupport) SharedConfig() sharedconfig.Manager {
	panic("Unimplemented")
}
func (mcs *mockConsenterSupport) WriteBlock(data []*cb.Envelope, metadata [][]byte, committers []filter.Committer) {
	logger.Debugf("mockWriter: attempting to write batch")
	mcs.batches <- data
}

var testMessage = &cb.Envelope{Payload: []byte("TEST_MESSAGE")}

func syncQueueMessage(msg *cb.Envelope, chain multichain.Chain, bc *mockBlockCutter) {
	chain.Enqueue(msg)
	bc.block <- struct{}{}
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
	support := &mockConsenterSupport{
		batches: make(chan []*cb.Envelope),
		cutter:  newMockBlockCutter(),
	}
	defer close(support.cutter.block)
	bs := newChain(time.Millisecond, support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.cutter)
	bs.Halt()
	select {
	case <-support.batches:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestBatchTimer(t *testing.T) {
	support := &mockConsenterSupport{
		batches: make(chan []*cb.Envelope),
		cutter:  newMockBlockCutter(),
	}
	defer close(support.cutter.block)
	bs := newChain(time.Millisecond, support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.cutter)

	select {
	case <-support.batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because of batch timer expiration but did not")
	}

	syncQueueMessage(testMessage, bs, support.cutter)
	select {
	case <-support.batches:
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the timer was not appopriately reset")
	}

	bs.Halt()
	select {
	case <-support.batches:
		t.Fatalf("Expected no invocations of Append")
	case <-wg.done:
	}
}

func TestBatchTimerHaltOnFilledBatch(t *testing.T) {
	support := &mockConsenterSupport{
		batches: make(chan []*cb.Envelope),
		cutter:  newMockBlockCutter(),
	}
	defer close(support.cutter.block)

	bs := newChain(time.Hour, support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.cutter)
	support.cutter.cutNext = true
	syncQueueMessage(testMessage, bs, support.cutter)

	select {
	case <-support.batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because the batch was filled, but did not")
	}

	// Change the batch timeout to be near instant, if the timer was not reset, it will still be waiting an hour
	bs.batchTimeout = time.Millisecond

	support.cutter.cutNext = false
	syncQueueMessage(testMessage, bs, support.cutter)

	select {
	case <-support.batches:
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
	support := &mockConsenterSupport{
		batches: make(chan []*cb.Envelope),
		cutter:  newMockBlockCutter(),
	}
	defer close(support.cutter.block)
	bs := newChain(time.Hour, support)
	wg := goWithWait(bs.main)
	defer bs.Halt()

	syncQueueMessage(testMessage, bs, support.cutter)
	support.cutter.isolatedTx = true
	syncQueueMessage(testMessage, bs, support.cutter)

	select {
	case <-support.batches:
	case <-time.After(time.Second):
		t.Fatalf("Expected two blocks to be cut but never got the first")
	}

	select {
	case <-support.batches:
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
