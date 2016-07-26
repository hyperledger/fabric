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

package executor

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/consensus/util/events"

	pb "github.com/hyperledger/fabric/protos"

	"github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// -------------------------
//
// Mock consumer
//
// -------------------------

type mockConsumer struct {
	ExecutedImpl     func(tag interface{})                            // Called whenever Execute completes
	CommittedImpl    func(tag interface{}, target *pb.BlockchainInfo) // Called whenever Commit completes
	RolledBackImpl   func(tag interface{})                            // Called whenever a Rollback completes
	StateUpdatedImpl func(tag interface{}, target *pb.BlockchainInfo) // Called when state transfer completes, if target is nil, this indicates a failure and a new target should be supplied
}

func (mock *mockConsumer) Executed(tag interface{}) {
	if mock.ExecutedImpl != nil {
		mock.ExecutedImpl(tag)
	}
}

func (mock *mockConsumer) Committed(tag interface{}, target *pb.BlockchainInfo) {
	if mock.CommittedImpl != nil {
		mock.CommittedImpl(tag, target)
	}
}

func (mock *mockConsumer) RolledBack(tag interface{}) {
	if mock.RolledBackImpl != nil {
		mock.RolledBackImpl(tag)
	}
}

func (mock *mockConsumer) StateUpdated(tag interface{}, target *pb.BlockchainInfo) {
	if mock.StateUpdatedImpl != nil {
		mock.StateUpdatedImpl(tag, target)
	}
}

// -------------------------
//
// Mock rawExecutor
//
// -------------------------

type mockRawExecutor struct {
	t           *testing.T
	curBatch    interface{}
	curTxs      []*pb.Transaction
	commitCount uint64
}

func (mock *mockRawExecutor) BeginTxBatch(id interface{}) error {
	if mock.curBatch != nil {
		e := fmt.Errorf("Attempted to start a new batch without stopping the other")
		mock.t.Fatal(e)
		return e
	}
	mock.curBatch = id
	return nil
}

func (mock *mockRawExecutor) ExecTxs(id interface{}, txs []*pb.Transaction) ([]byte, error) {
	if mock.curBatch != id {
		e := fmt.Errorf("Attempted to exec on a different batch")
		mock.t.Fatal(e)
		return nil, e
	}
	mock.curTxs = append(mock.curTxs, txs...)
	return nil, nil
}

func (mock *mockRawExecutor) CommitTxBatch(id interface{}, meta []byte) (*pb.Block, error) {
	if mock.curBatch != id {
		e := fmt.Errorf("Attempted to commit a batch which doesn't exist")
		mock.t.Fatal(e)
		return nil, e
	}
	mock.commitCount++
	return nil, nil
}

func (mock *mockRawExecutor) RollbackTxBatch(id interface{}) error {
	if mock.curBatch == nil {
		e := fmt.Errorf("Attempted to rollback a batch which doesn't exist")
		mock.t.Fatal(e)
		return e
	}

	mock.curTxs = nil
	mock.curBatch = nil

	return nil
}

func (mock *mockRawExecutor) PreviewCommitTxBatch(id interface{}, meta []byte) ([]byte, error) {
	if mock.curBatch != nil {
		e := fmt.Errorf("Attempted to preview a batch which doesn't exist")
		mock.t.Fatal(e)
		return nil, e
	}

	return nil, nil
}

func (mock *mockRawExecutor) GetBlockchainInfo() *pb.BlockchainInfo {
	return &pb.BlockchainInfo{
		Height:            mock.commitCount,
		CurrentBlockHash:  []byte(fmt.Sprintf("%d", mock.commitCount)),
		PreviousBlockHash: []byte(fmt.Sprintf("%d", mock.commitCount-1)),
	}
}

// -------------------------
//
// Mock stateTransfer
//
// -------------------------

type mockStateTransfer struct {
	StartImpl        func()
	StopImpl         func()
	SyncToTargetImpl func(blockNumber uint64, blockHash []byte, peerIDs []*pb.PeerID) (error, bool)
}

func (mock *mockStateTransfer) Start() {}
func (mock *mockStateTransfer) Stop()  {}

func (mock *mockStateTransfer) SyncToTarget(blockNumber uint64, blockHash []byte, peerIDs []*pb.PeerID) (error, bool) {
	if mock.SyncToTargetImpl != nil {
		return mock.SyncToTargetImpl(blockNumber, blockHash, peerIDs)
	}
	return nil, false
}

// -------------------------
//
// Mock event manager
//
// -------------------------

type mockEventManager struct {
	target          events.Receiver
	bufferedChannel chan events.Event // This is buffered so that queueing never blocks
}

func (mock *mockEventManager) Start() {}

func (mock *mockEventManager) Halt() {}

func (mock *mockEventManager) Inject(event events.Event) {}

func (mock *mockEventManager) SetReceiver(receiver events.Receiver) {
	mock.target = receiver
}

func (mock *mockEventManager) Queue() chan<- events.Event {
	return mock.bufferedChannel
}

func (mock *mockEventManager) process() {
	for {
		select {
		case ev := <-mock.bufferedChannel:
			events.SendEvent(mock.target, ev)
		default:
			return
		}
	}
}

// -------------------------
//
// Util functions
//
// -------------------------

func newMocks(t *testing.T) (*coordinatorImpl, *mockConsumer, *mockRawExecutor, *mockStateTransfer, *mockEventManager) {
	mc := &mockConsumer{}
	mre := &mockRawExecutor{t: t}
	mst := &mockStateTransfer{}
	mev := &mockEventManager{bufferedChannel: make(chan events.Event, 100)}
	co := &coordinatorImpl{
		consumer:    mc,
		rawExecutor: mre,
		stc:         mst,
		manager:     mev,
	}
	mev.target = co
	return co, mc, mre, mst, mev
}

// -------------------------
//
// Actual Tests
//
// -------------------------

// TestNormalExecutes executes 50 transactions, then commits, ensuring that the callbacks are called appropriately
func TestNormalExecutes(t *testing.T) {
	co, mc, _, _, mev := newMocks(t)

	times := uint64(50)

	id := struct{}{}
	testTxs := []*pb.Transaction{&pb.Transaction{}, &pb.Transaction{}, &pb.Transaction{}}

	executed := uint64(0)
	mc.ExecutedImpl = func(tag interface{}) {
		if tag != id {
			t.Fatalf("Executed got wrong ID")
		}
		executed++
	}

	committed := false
	mc.CommittedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if tag != id {
			t.Fatalf("Committed got wrong ID")
		}
		committed = true
		if info.Height != 1 {
			t.Fatalf("Blockchain info should have returned height of %d, returned %d", 1, info.Height)
		}
	}

	for i := uint64(0); i < times; i++ {
		co.Execute(id, testTxs)
	}

	co.Commit(id, nil)
	mev.process()

	if executed != times {
		t.Fatalf("Should have executed %d times but executed %d times", times, executed)
	}

	if !committed {
		t.Fatalf("Should have committed")
	}
}

// TestRollbackExecutes executes 5 transactions, then rolls back, executes 5 more and commits, ensuring that the callbacks are called appropriately
func TestRollbackExecutes(t *testing.T) {
	co, mc, _, _, mev := newMocks(t)

	times := uint64(5)

	id := struct{}{}
	testTxs := []*pb.Transaction{&pb.Transaction{}, &pb.Transaction{}, &pb.Transaction{}}

	executed := uint64(0)
	mc.ExecutedImpl = func(tag interface{}) {
		if tag != id {
			t.Fatalf("Executed got wrong ID")
		}
		executed++
	}

	committed := false
	mc.CommittedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if tag != id {
			t.Fatalf("Committed got wrong ID")
		}
		committed = true
		if info.Height != 1 {
			t.Fatalf("Blockchain info should have returned height of %d, returned %d", 1, info.Height)
		}
	}

	rolledBack := false
	mc.RolledBackImpl = func(tag interface{}) {
		if tag != id {
			t.Fatalf("RolledBack got wrong ID")
		}
		rolledBack = true
	}

	for i := uint64(0); i < times; i++ {
		co.Execute(id, testTxs)
	}

	co.Rollback(id)
	mev.process()

	if !rolledBack {
		t.Fatalf("Should have rolled back")
	}

	for i := uint64(0); i < times; i++ {
		co.Execute(id, testTxs)
	}
	co.Commit(id, nil)
	mev.process()

	if executed != 2*times {
		t.Fatalf("Should have executed %d times but executed %d times", 2*times, executed)
	}

	if !committed {
		t.Fatalf("Should have committed")
	}
}

// TestEmptyCommit attempts to commit without executing any transactions, this is considered a fatal error and no callback should occur
func TestEmptyCommit(t *testing.T) {
	co, mc, _, _, mev := newMocks(t)

	mc.CommittedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		t.Fatalf("Should not have committed")
	}

	co.Commit(nil, nil)
	mev.process()
}

// TestEmptyRollback attempts a rollback without executing any transactions, this is considered an error and no callback should occur
func TestEmptyRollback(t *testing.T) {
	co, mc, _, _, mev := newMocks(t)

	mc.RolledBackImpl = func(tag interface{}) {
		t.Fatalf("Should not have committed")
	}

	co.Rollback(nil)
	mev.process()
}

// TestNormalStateTransfer attempts a simple state transfer request with 10 recoverable failures
func TestNormalStateTransfer(t *testing.T) {
	co, mc, _, mst, mev := newMocks(t)
	//co, mc, mre, mst, mev := newMocks(t)

	id := struct{}{}
	blockNumber := uint64(2389)
	blockHash := []byte("BlockHash")

	stateUpdated := false
	mc.StateUpdatedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if id != tag {
			t.Fatalf("Incorrect tag received")
		}
		if stateUpdated {
			t.Fatalf("State should only be updated once")
		}
		if info.Height != blockNumber+1 {
			t.Fatalf("Final height should have been %d", blockNumber+1)
		}
		if !bytes.Equal(info.CurrentBlockHash, blockHash) {
			t.Fatalf("Final height should have been %d", blockNumber+1)
		}
		stateUpdated = true
	}

	count := 0
	mst.SyncToTargetImpl = func(bn uint64, bh []byte, ps []*pb.PeerID) (error, bool) {
		count++
		if count <= 10 {
			return fmt.Errorf("Transient state transfer error"), true
		}

		return nil, true
	}

	co.UpdateState(id, &pb.BlockchainInfo{Height: blockNumber + 1, CurrentBlockHash: blockHash}, nil)
	mev.process()

	if !stateUpdated {
		t.Fatalf("State should have been updated")
	}
}

// TestFailingStateTransfer attempts a failing simple state transfer request with 10 recoverable failures, then a fatal error, then a success
func TestFailingStateTransfer(t *testing.T) {
	co, mc, _, mst, mev := newMocks(t)
	//co, mc, mre, mst, mev := newMocks(t)

	id := struct{}{}
	blockNumber1 := uint64(1)
	blockHash1 := []byte("BlockHash1")
	blockNumber2 := uint64(2)
	blockHash2 := []byte("BlockHash2")

	stateUpdated := false
	mc.StateUpdatedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if id != tag {
			t.Fatalf("Incorrect tag received")
		}
		if stateUpdated {
			t.Fatalf("State should only be updated once")
		}
		if info == nil {
			return
		}
		if info.Height != blockNumber2+1 {
			t.Fatalf("Final height should have been %d", blockNumber2+1)
		}
		if !bytes.Equal(info.CurrentBlockHash, blockHash2) {
			t.Fatalf("Final height should have been %d", blockNumber2+1)
		}
		stateUpdated = true
	}

	count := 0
	mst.SyncToTargetImpl = func(bn uint64, bh []byte, ps []*pb.PeerID) (error, bool) {
		count++
		if count <= 10 {
			return fmt.Errorf("Transient state transfer error"), true
		}

		if bn == blockNumber1 {
			return fmt.Errorf("Irrecoverable state transfer error"), false
		}

		return nil, true
	}

	co.UpdateState(id, &pb.BlockchainInfo{Height: blockNumber1 + 1, CurrentBlockHash: blockHash1}, nil)
	mev.process()

	if stateUpdated {
		t.Fatalf("State should not have been updated")
	}

	co.UpdateState(id, &pb.BlockchainInfo{Height: blockNumber2 + 1, CurrentBlockHash: blockHash2}, nil)
	mev.process()

	if !stateUpdated {
		t.Fatalf("State should have been updated")
	}
}

// TestExecuteAfterStateTransfer attempts an execute and commit after a simple state transfer request
func TestExecuteAfterStateTransfer(t *testing.T) {
	co, mc, _, _, mev := newMocks(t)
	testTxs := []*pb.Transaction{&pb.Transaction{}, &pb.Transaction{}, &pb.Transaction{}}

	id := struct{}{}
	blockNumber := uint64(2389)
	blockHash := []byte("BlockHash")

	stateTransferred := false
	mc.StateUpdatedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if nil == info {
			t.Fatalf("State transfer should have succeeded")
		}
		stateTransferred = true
	}

	executed := false
	mc.ExecutedImpl = func(tag interface{}) {
		executed = true
	}

	co.UpdateState(id, &pb.BlockchainInfo{Height: blockNumber + 1, CurrentBlockHash: blockHash}, nil)
	co.Execute(id, testTxs)
	mev.process()

	if !executed {
		t.Fatalf("Execution should have occurred")
	}
}

// TestExecuteDuringStateTransfer attempts a state transfer which fails, then an execute which should not be performed
func TestExecuteDuringStateTransfer(t *testing.T) {
	co, mc, mre, mst, mev := newMocks(t)
	testTxs := []*pb.Transaction{&pb.Transaction{}, &pb.Transaction{}, &pb.Transaction{}}

	id := struct{}{}
	blockNumber := uint64(2389)
	blockHash := []byte("BlockHash")

	mc.StateUpdatedImpl = func(tag interface{}, info *pb.BlockchainInfo) {
		if info != nil {
			t.Fatalf("State transfer should not succeed")
		}
	}

	mst.SyncToTargetImpl = func(bn uint64, bh []byte, ps []*pb.PeerID) (error, bool) {
		return fmt.Errorf("Irrecoverable error"), false
	}

	co.UpdateState(id, &pb.BlockchainInfo{Height: blockNumber + 1, CurrentBlockHash: blockHash}, nil)
	co.Execute(id, testTxs)
	mev.process()

	if mre.curBatch != nil {
		t.Fatalf("Execution should not have executed beginning a new batch")
	}
}
