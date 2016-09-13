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

package statetransfer

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	configSetup "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos"

	"github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var AllFailures = [...]mockResponse{Timeout, Corrupt, OutOfOrder}

type testPartialStack struct {
	*MockRemoteHashLedgerDirectory
	*MockLedger
}

func TestMain(m *testing.M) {
	configSetup.SetupTestConfig("./../../../peer")
	os.Exit(m.Run())
}

func newPartialStack(ml *MockLedger, rld *MockRemoteHashLedgerDirectory) PartialStack {
	return &testPartialStack{
		MockLedger:                    ml,
		MockRemoteHashLedgerDirectory: rld,
	}
}

func newTestStateTransfer(ml *MockLedger, rld *MockRemoteHashLedgerDirectory) *coordinatorImpl {
	ci := NewCoordinatorImpl(newPartialStack(ml, rld)).(*coordinatorImpl)
	ci.Start()
	return ci
}

func newTestThreadlessStateTransfer(ml *MockLedger, rld *MockRemoteHashLedgerDirectory) *coordinatorImpl {
	return NewCoordinatorImpl(newPartialStack(ml, rld)).(*coordinatorImpl)
}

type MockRemoteHashLedgerDirectory struct {
	*HashLedgerDirectory
}

func (mrls *MockRemoteHashLedgerDirectory) GetMockRemoteLedgerByPeerID(peerID *protos.PeerID) *MockRemoteLedger {
	ml, _ := mrls.GetLedgerByPeerID(peerID)
	return ml.(*MockRemoteLedger)
}

func createRemoteLedgers(low, high uint64) *MockRemoteHashLedgerDirectory {
	rols := make(map[protos.PeerID]peer.BlockChainAccessor)

	for i := low; i <= high; i++ {
		peerID := &protos.PeerID{
			Name: fmt.Sprintf("Peer %d", i),
		}
		l := &MockRemoteLedger{}
		rols[*peerID] = l
	}
	return &MockRemoteHashLedgerDirectory{&HashLedgerDirectory{rols}}
}

func executeStateTransfer(sts *coordinatorImpl, ml *MockLedger, blockNumber, sequenceNumber uint64, mrls *MockRemoteHashLedgerDirectory) error {

	for peerID := range mrls.remoteLedgers {
		mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = blockNumber + 1
	}

	var err error

	blockHash := SimpleGetBlockHash(blockNumber)
	for i := 0; i < 100; i++ {
		var recoverable bool
		err, recoverable = sts.SyncToTarget(blockNumber, blockHash, nil)
		if err == nil || !recoverable {
			break
		}
		time.Sleep(10 * time.Millisecond)
		// Try to sync for up to 10 seconds
	}

	if err != nil {
		return err
	}

	if size := ml.GetBlockchainSize(); size != blockNumber+1 {
		return fmt.Errorf("Blockchain should be caught up to block %d, but is only %d tall", blockNumber, size)
	}

	block, err := ml.GetBlock(blockNumber)

	if nil != err {
		return fmt.Errorf("Error retrieving last block in the mock chain.")
	}

	if stateHash, _ := ml.GetCurrentStateHash(); !bytes.Equal(stateHash, block.StateHash) {
		return fmt.Errorf("Current state does not validate against the latest block.")
	}

	return nil
}

type filterResult struct {
	triggered bool
	peerID    *protos.PeerID
	mutex     *sync.Mutex
}

func (res filterResult) wasTriggered() bool {
	res.mutex.Lock()
	defer res.mutex.Unlock()
	return res.triggered
}

func makeSimpleFilter(failureTrigger mockRequest, failureType mockResponse) (func(mockRequest, *protos.PeerID) mockResponse, *filterResult) {
	res := &filterResult{triggered: false, mutex: &sync.Mutex{}}
	return func(request mockRequest, peerID *protos.PeerID) mockResponse {
		//fmt.Println("Received a request", request, "for replicaId", replicaId)
		if request != failureTrigger {
			return Normal
		}

		res.mutex.Lock()
		defer res.mutex.Unlock()

		if !res.triggered {
			res.triggered = true
			res.peerID = peerID
		}

		if *peerID == *res.peerID {
			fmt.Println("Failing it with", failureType)
			return failureType
		}
		return Normal
	}, res

}

func TestStartupValidStateGenesis(t *testing.T) {
	mrls := createRemoteLedgers(2, 1) // No remote targets available

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(0, SimpleGetBlock(0))

	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, 0, 0, mrls); nil != err {
		t.Fatalf("Startup failure: %s", err)
	}

}

func TestStartupValidStateExisting(t *testing.T) {
	mrls := createRemoteLedgers(2, 1) // No remote targets available

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, nil, t)
	height := uint64(50)
	for i := uint64(0); i < height; i++ {
		ml.PutBlock(i, SimpleGetBlock(i))
	}
	ml.state = SimpleGetState(height - 1)

	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, height-1, height-1, mrls); nil != err {
		t.Fatalf("Startup failure: %s", err)
	}

}

func TestStartupInvalidStateGenesis(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(0, SimpleGetBlock(0))
	ml.state = ^ml.state // Ensure the state is wrong

	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, 0, 0, mrls); nil != err {
		t.Fatalf("Startup failure: %s", err)
	}

}

func TestStartupInvalidStateExisting(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, nil, t)
	height := uint64(50)
	for i := uint64(0); i < height; i++ {
		ml.PutBlock(i, SimpleGetBlock(i))
	}
	ml.state = ^SimpleGetState(height - 1) // Ensure the state is wrong

	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, height-1, height-1, mrls); nil != err {
		t.Fatalf("Startup failure: %s", err)
	}

}

func TestCatchupSimple(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(0, SimpleGetBlock(0))

	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
		t.Fatalf("Simplest case: %s", err)
	}

}

func TestCatchupWithLowMaxDeltas(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	// Test from blockheight of 1, with valid genesis block
	deltasTransferred := uint64(0)
	blocksTransferred := uint64(0)
	ml := NewMockLedger(mrls, func(request mockRequest, peerID *protos.PeerID) mockResponse {
		if request == SyncDeltas {
			deltasTransferred++
		}

		if request == SyncBlocks {
			blocksTransferred++
		}

		return Normal
	}, t)
	ml.PutBlock(0, SimpleGetBlock(0))

	sts := newTestStateTransfer(ml, mrls)
	maxRange := uint64(3)
	sts.maxStateDeltaRange = maxRange
	sts.maxBlockRange = maxRange
	defer sts.Stop()

	targetBlock := uint64(7)
	if err := executeStateTransfer(sts, ml, targetBlock, 10, mrls); nil != err {
		t.Fatalf("Without deltas case: %s", err)
	}

	existingBlocks := uint64(1)
	targetTransferred := (targetBlock - existingBlocks) / maxRange
	if (targetBlock-existingBlocks)%maxRange != 0 {
		targetTransferred++
	}

	if deltasTransferred != targetTransferred {
		t.Errorf("Expected %d state deltas transferred, got %d", targetTransferred, deltasTransferred)
	}

	if blocksTransferred != targetTransferred {
		t.Errorf("Expected %d state blocks transferred, got %d", targetTransferred, blocksTransferred)
	}

}

func TestCatchupWithoutDeltas(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	deltasTransferred := false

	// Test from blockheight of 1, with valid genesis block
	ml := NewMockLedger(mrls, func(request mockRequest, peerID *protos.PeerID) mockResponse {
		if request == SyncDeltas {
			deltasTransferred = true
		}

		return Normal
	}, t)
	ml.PutBlock(0, SimpleGetBlock(0))

	sts := NewCoordinatorImpl(newPartialStack(ml, mrls)).(*coordinatorImpl)
	sts.maxStateDeltas = 0

	done := make(chan struct{})
	go func() {
		sts.blockThread()
		close(done)
	}()

	if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
		t.Fatalf("Without deltas case: %s", err)
	}

	if deltasTransferred {
		t.Fatalf("State delta retrieval should not occur during this test")
	}

	sts.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Timed out waiting for block sync to complete")
	}

	for i := uint64(0); i <= 7; i++ {
		if _, err := ml.GetBlockByNumber(i); err != nil {
			t.Errorf("Expected block %d but got error %s", i, err)
		}
	}
}

func TestCatchupSyncBlocksErrors(t *testing.T) {
	for _, failureType := range AllFailures {
		mrls := createRemoteLedgers(1, 3)

		// Test from blockheight of 1 with valid genesis block
		// Timeouts of 10 milliseconds
		filter, result := makeSimpleFilter(SyncBlocks, failureType)
		ml := NewMockLedger(mrls, filter, t)

		ml.PutBlock(0, SimpleGetBlock(0))
		sts := newTestStateTransfer(ml, mrls)
		defer sts.Stop()
		sts.BlockRequestTimeout = 10 * time.Millisecond
		if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
			t.Fatalf("SyncBlocksErrors %s case: %s", failureType, err)
		}
		if !result.wasTriggered() {
			t.Fatalf("SyncBlocksErrors case never simulated a %v", failureType)
		}
	}
}

// Added for issue #676, for situations all potential sync targets fail, and sync is re-initiated, causing panic
func TestCatchupSyncBlocksAllErrors(t *testing.T) {
	blockNumber := uint64(10)

	for _, failureType := range AllFailures {
		mrls := createRemoteLedgers(1, 3)

		// Test from blockheight of 1 with valid genesis block
		// Timeouts of 10 milliseconds
		succeeding := &filterResult{triggered: false, mutex: &sync.Mutex{}}
		filter := func(request mockRequest, peerID *protos.PeerID) mockResponse {

			succeeding.mutex.Lock()
			defer succeeding.mutex.Unlock()
			if !succeeding.triggered {
				return failureType
			}

			return Normal
		}
		ml := NewMockLedger(mrls, filter, t)

		ml.PutBlock(0, SimpleGetBlock(0))
		sts := newTestStateTransfer(ml, mrls)
		defer sts.Stop()
		sts.BlockRequestTimeout = 10 * time.Millisecond

		for peerID := range mrls.remoteLedgers {
			mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = blockNumber + 1
		}

		blockHash := SimpleGetBlockHash(blockNumber)
		if err, _ := sts.SyncToTarget(blockNumber, blockHash, nil); err == nil {
			t.Fatalf("State transfer should not have completed yet")
		}

		succeeding.triggered = true
		if err, _ := sts.SyncToTarget(blockNumber, blockHash, nil); err != nil {
			t.Fatalf("Error completing state transfer")
		}

		if size := ml.GetBlockchainSize(); size != blockNumber+1 {
			t.Fatalf("Blockchain should be caught up to block %d, but is only %d tall", blockNumber, size)
		}

		block, err := ml.GetBlock(blockNumber)

		if nil != err {
			t.Fatalf("Error retrieving last block in the mock chain.")
		}

		if stateHash, _ := ml.GetCurrentStateHash(); !bytes.Equal(stateHash, block.StateHash) {
			t.Fatalf("Current state does not validate against the latest block.")
		}
	}
}

func TestCatchupMissingEarlyChain(t *testing.T) {
	mrls := createRemoteLedgers(1, 3)

	// Test from blockheight of 5 (with missing blocks 0-3)
	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(4, SimpleGetBlock(4))
	sts := newTestStateTransfer(ml, mrls)
	defer sts.Stop()
	if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
		t.Fatalf("MissingEarlyChain case: %s", err)
	}
}

func TestCatchupSyncSnapshotError(t *testing.T) {
	for _, failureType := range AllFailures {
		mrls := createRemoteLedgers(1, 3)

		// Test from blockheight of 5 (with missing blocks 0-3)
		// Timeouts of 1 second, also test corrupt snapshot
		filter, result := makeSimpleFilter(SyncSnapshot, failureType)
		ml := NewMockLedger(mrls, filter, t)
		ml.PutBlock(4, SimpleGetBlock(4))
		sts := newTestStateTransfer(ml, mrls)
		defer sts.Stop()
		sts.StateSnapshotRequestTimeout = 10 * time.Millisecond
		if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
			t.Fatalf("SyncSnapshotError %s case: %s", failureType, err)
		}
		if !result.wasTriggered() {
			t.Fatalf("SyncSnapshotError case never simulated a %s", failureType)
		}
	}
}

func TestCatchupSyncDeltasError(t *testing.T) {
	for _, failureType := range AllFailures {
		mrls := createRemoteLedgers(1, 3)

		// Test from blockheight of 5 (with missing blocks 0-3)
		// Timeouts of 1 second
		filter, result := makeSimpleFilter(SyncDeltas, failureType)
		ml := NewMockLedger(mrls, filter, t)
		ml.PutBlock(4, SimpleGetBlock(4))
		ml.state = SimpleGetState(4)
		sts := newTestStateTransfer(ml, mrls)
		defer sts.Stop()
		sts.StateDeltaRequestTimeout = 10 * time.Millisecond
		sts.StateSnapshotRequestTimeout = 10 * time.Millisecond
		if err := executeStateTransfer(sts, ml, 7, 10, mrls); nil != err {
			t.Fatalf("SyncDeltasError %s case: %s", failureType, err)
		}
		if !result.wasTriggered() {
			t.Fatalf("SyncDeltasError case never simulated a %s", failureType)
		}
	}
}

func executeBlockRecovery(ml *MockLedger, millisTimeout int, mrls *MockRemoteHashLedgerDirectory) error {

	sts := newTestThreadlessStateTransfer(ml, mrls)
	sts.BlockRequestTimeout = time.Duration(millisTimeout) * time.Millisecond
	sts.RecoverDamage = true

	w := make(chan struct{})

	go func() {
		for !sts.verifyAndRecoverBlockchain() {
		}
		w <- struct{}{}
	}()

	select {
	case <-time.After(time.Second * 2):
		return fmt.Errorf("Timed out waiting for blocks to replicate for blockchain")
	case <-w:
		// Do nothing, continue the test
	}

	if n, err := ml.VerifyBlockchain(7, 0); 0 != n || nil != err {
		return fmt.Errorf("Blockchain claims to be up to date, but does not verify")
	}

	return nil
}

func executeBlockRecoveryWithPanic(ml *MockLedger, millisTimeout int, mrls *MockRemoteHashLedgerDirectory) error {

	sts := newTestThreadlessStateTransfer(ml, mrls)
	sts.BlockRequestTimeout = time.Duration(millisTimeout) * time.Millisecond
	sts.RecoverDamage = false

	w := make(chan bool)

	go func() {
		defer func() {
			recover()
			w <- true
		}()
		for !sts.verifyAndRecoverBlockchain() {
		}
		w <- false
	}()

	select {
	case <-time.After(time.Second * 2):
		return fmt.Errorf("Timed out waiting for blocks to replicate for blockchain")
	case didPanic := <-w:
		// Do nothing, continue the test
		if !didPanic {
			return fmt.Errorf("Blockchain was supposed to panic on modification, but did not")
		}
	}

	return nil
}

func TestCatchupLaggingChains(t *testing.T) {
	mrls := createRemoteLedgers(0, 3)

	for peerID := range mrls.remoteLedgers {
		mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = 701
	}

	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(7, SimpleGetBlock(7))
	if err := executeBlockRecovery(ml, 10, mrls); nil != err {
		t.Fatalf("TestCatchupLaggingChains short chain failure: %s", err)
	}

	ml = NewMockLedger(mrls, nil, t)
	ml.PutBlock(200, SimpleGetBlock(200))
	// Use a large timeout here because the mock ledger is slow for large blocks
	if err := executeBlockRecovery(ml, 1000, mrls); nil != err {
		t.Fatalf("TestCatchupLaggingChains long chain failure: %s", err)
	}
}

func TestCatchupLaggingWithSmallMaxBlocks(t *testing.T) {
	mrls := createRemoteLedgers(0, 3)

	for peerID := range mrls.remoteLedgers {
		mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = 201
	}

	maxSyncBlocks := uint64(3)
	startingBlock := uint64(200)

	syncBlockTries := uint64(0)
	ml := NewMockLedger(mrls, func(request mockRequest, peerID *protos.PeerID) mockResponse {
		if request == SyncBlocks {
			syncBlockTries++
		}

		return Normal
	}, t)
	ml.PutBlock(startingBlock, SimpleGetBlock(startingBlock))

	sts := newTestThreadlessStateTransfer(ml, mrls)
	sts.BlockRequestTimeout = 1000 * time.Millisecond
	sts.RecoverDamage = true
	sts.maxBlockRange = maxSyncBlocks
	sts.blockVerifyChunkSize = maxSyncBlocks

	w := make(chan struct{})

	go func() {
		for !sts.verifyAndRecoverBlockchain() {
		}
		w <- struct{}{}
	}()

	select {
	case <-time.After(time.Second * 2):
		t.Fatalf("Timed out waiting for blocks to replicate for blockchain")
	case <-w:
		// Do nothing, continue the test
	}

	target := startingBlock / maxSyncBlocks
	if startingBlock%maxSyncBlocks != 0 {
		target++
	}

	if syncBlockTries != target {
		t.Fatalf("Expected %d calls to sync blocks, but got %d, this indicates maxBlockRange is not being respected", target, syncBlockTries)
	}
}

func TestCatchupLaggingChainsErrors(t *testing.T) {
	for _, failureType := range AllFailures {
		mrls := createRemoteLedgers(0, 3)

		for peerID := range mrls.remoteLedgers {
			mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = 701
		}

		filter, result := makeSimpleFilter(SyncBlocks, failureType)
		ml := NewMockLedger(mrls, filter, t)
		ml.PutBlock(7, SimpleGetBlock(7))
		if err := executeBlockRecovery(ml, 10, mrls); nil != err {
			t.Fatalf("TestCatchupLaggingChainsErrors %s short chain with timeout failure: %s", failureType, err)
		}
		if !result.wasTriggered() {
			t.Fatalf("TestCatchupLaggingChainsErrors short chain with timeout never simulated a %s", failureType)
		}
	}
}

func TestCatchupCorruptChains(t *testing.T) {
	mrls := createRemoteLedgers(0, 3)

	for peerID := range mrls.remoteLedgers {
		mrls.GetMockRemoteLedgerByPeerID(&peerID).blockHeight = 701
	}

	ml := NewMockLedger(mrls, nil, t)
	ml.PutBlock(7, SimpleGetBlock(7))
	ml.PutBlock(3, SimpleGetBlock(2))
	if err := executeBlockRecovery(ml, 10, mrls); nil != err {
		t.Fatalf("TestCatchupCorruptChains short chain failure: %s", err)
	}

	ml = NewMockLedger(mrls, nil, t)
	ml.PutBlock(7, SimpleGetBlock(7))
	ml.PutBlock(3, SimpleGetBlock(2))
	defer func() {
		//fmt.Println("Executing defer")
		// We expect a panic, this is great
		recover()
	}()
	if err := executeBlockRecoveryWithPanic(ml, 10, mrls); nil != err {
		t.Fatalf("TestCatchupCorruptChains short chain failure: %s", err)
	}
}

func TestBlockRangeOrdering(t *testing.T) {
	lowRange := &blockRange{
		highBlock: 10,
		lowBlock:  5,
	}

	highRange := &blockRange{
		highBlock: 15,
		lowBlock:  12,
	}

	bigRange := &blockRange{
		highBlock: 15,
		lowBlock:  9,
	}

	slice := blockRangeSlice([]*blockRange{lowRange, highRange, bigRange})

	sort.Sort(slice)

	if slice[0] != bigRange {
		t.Fatalf("Big range should come first")
	}

	if slice[1] != highRange {
		t.Fatalf("High range should come second")
	}

	if slice[2] != lowRange {
		t.Fatalf("Low range should come third")
	}
}
