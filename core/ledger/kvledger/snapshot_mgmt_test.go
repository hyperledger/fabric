/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRequestBookKeeper(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testsnapshotrequestbookkeeper"
	dbHandle := provider.bookkeepingProvider.GetDBHandle(ledgerID, bookkeeping.SnapshotRequest)
	bookkeeper, err := newSnapshotRequestBookkeeper(dbHandle)
	require.NoError(t, err)

	// add requests and verify smallestRequestHeight
	require.NoError(t, bookkeeper.add(100))
	require.Equal(t, uint64(100), bookkeeper.smallestRequestHeight)

	require.NoError(t, bookkeeper.add(15))
	require.Equal(t, uint64(15), bookkeeper.smallestRequestHeight)

	require.NoError(t, bookkeeper.add(50))
	require.Equal(t, uint64(15), bookkeeper.smallestRequestHeight)

	requestHeights, err := bookkeeper.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{15, 50, 100})

	for _, height := range []uint64{15, 50, 100} {
		exist, err := bookkeeper.exist(height)
		require.NoError(t, err)
		require.True(t, exist)
	}

	exist, err := bookkeeper.exist(10)
	require.NoError(t, err)
	require.False(t, exist)

	provider.Close()

	// reopen the provider and verify snapshotRequestBookkeeper is initialized correctly
	provider2 := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider2.Close()
	dbHandle2 := provider2.bookkeepingProvider.GetDBHandle(ledgerID, bookkeeping.SnapshotRequest)
	bookkeeper2, err := newSnapshotRequestBookkeeper(dbHandle2)
	require.NoError(t, err)

	requestHeights, err = bookkeeper2.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{15, 50, 100})

	require.Equal(t, uint64(15), bookkeeper2.smallestRequestHeight)

	// delete requests and verify smallest request height
	require.NoError(t, bookkeeper2.delete(100))
	require.Equal(t, uint64(15), bookkeeper2.smallestRequestHeight)

	require.NoError(t, bookkeeper2.delete(15))
	require.Equal(t, uint64(50), bookkeeper2.smallestRequestHeight)

	require.NoError(t, bookkeeper2.delete(50))
	require.Equal(t, defaultSmallestHeight, bookkeeper2.smallestRequestHeight)

	requestHeights, err = bookkeeper2.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{})
}

func TestSnapshotRequestBookKeeperErrorPaths(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	dbHandle := provider.bookkeepingProvider.GetDBHandle("testrequestbookkeepererrorpaths", bookkeeping.SnapshotRequest)
	bookkeeper2, err := newSnapshotRequestBookkeeper(dbHandle)
	require.NoError(t, err)

	require.NoError(t, bookkeeper2.add(20))
	require.EqualError(t, bookkeeper2.add(20), "duplicate snapshot request for height 20")
	require.EqualError(t, bookkeeper2.delete(100), "no snapshot request exists for height 100")

	provider.Close()

	_, err = newSnapshotRequestBookkeeper(dbHandle)
	require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")

	err = bookkeeper2.add(20)
	require.Contains(t, err.Error(), "leveldb: closed")

	err = bookkeeper2.delete(1)
	require.Contains(t, err.Error(), "leveldb: closed")

	_, err = bookkeeper2.list()
	require.Contains(t, err.Error(), "leveldb: closed")

	_, err = bookkeeper2.exist(20)
	require.Contains(t, err.Error(), "leveldb: closed")
}

func TestSnapshotRequests(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	// create a ledger with genesis block
	ledgerID := "testsnapshotrequests"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer l.Close()
	kvledger := l.(*kvLedger)

	// Test 1: submit requests in parallel and verify PendingSnapshotRequest
	for _, height := range []uint64{100, 5, 3, 10, 30} {
		go func(requestedHeight uint64) {
			require.NoError(t, l.SubmitSnapshotRequest(requestedHeight))
		}(height)
	}
	// wait until all requests are submitted
	requestsUpdated := func() bool {
		requests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{3, 5, 10, 30, 100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)

	// Test 2: cancel requests in parallel and verify PendingSnapshotRequest
	for _, height := range []uint64{3, 30} {
		go func(requestedHeight uint64) {
			require.NoError(t, l.CancelSnapshotRequest(requestedHeight))
		}(height)
	}
	// wait until all requests are cancelled
	requestsUpdated = func() bool {
		requests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{5, 10, 100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)

	// Test 3: commit blocks and verify snapshots are generated for height=5 and height=10
	lastBlock := testutilCommitBlocks(t, l, bg, 10, gbHash)
	// verify snapshot has been generated for height=5
	exists, err := kvledger.snapshotExists(5)
	require.NoError(t, err)
	require.True(t, exists)
	// verify snapshot is eventually generated height=10
	snapshotExists := func() bool {
		exists, err := kvledger.snapshotExists(10)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// Test 4: commit blocks and submit a request at height=0 to trigger snapshot generation
	lastBlock = testutilCommitBlocks(t, l, bg, 20, protoutil.BlockHeaderHash(lastBlock.Header))
	require.NoError(t, l.SubmitSnapshotRequest(0))
	// wait until snapshot is generated for height=20
	snapshotExists = func() bool {
		exists, err := kvledger.snapshotExists(20)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// wait until previous snapshotDone event deletes the request since it is in a separate goroutine
	requestsUpdated = func() bool {
		requests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)

	// prepare to test recoverSnapshot when a ledger is reopened
	// commit blocks to height 25 and add a request at height 25 to the leveldb directly
	// snapshot should not be generated and will be recovered after the ledger is reopened
	testutilCommitBlocks(t, l, bg, 25, protoutil.BlockHeaderHash(lastBlock.Header))
	require.NoError(t, kvledger.snapshotMgr.snapshotRequestBookkeeper.dbHandle.Put(encodeSnapshotRequestKey(25), []byte{}, true))
	exists, err = kvledger.snapshotExists(25)
	require.NoError(t, err)
	require.False(t, exists)

	// reopen the provider and ledger
	l.Close()
	provider.Close()
	provider2 := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider2.Close()
	l2, err := provider2.Open(ledgerID)
	require.NoError(t, err)
	kvledger2 := l2.(*kvLedger)

	// Test 5: verify snapshot height=25 is recovered and pending snapshot requests are correct
	snapshotExists = func() bool {
		exists, err := kvledger2.snapshotExists(25)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// wait until previous snapshotDone event deletes the request since it is in a separate goroutine
	requestsUpdated = func() bool {
		requests, err := l2.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)
}

func TestSnapshotMgrShutdown(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testsnapshotmgrshutdown"
	_, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	kvledger := l.(*kvLedger)

	// close ledger to shutdown snapshotMgr
	l.Close()

	// verify snapshotMgr.stopped and channels are stopped
	require.True(t, kvledger.snapshotMgr.stopped)
	require.PanicsWithError(
		t,
		"send on closed channel",
		func() { kvledger.snapshotMgr.events <- &event{typ: commitStart, height: 1} },
	)
	require.PanicsWithError(
		t,
		"send on closed channel",
		func() { kvledger.snapshotMgr.commitProceed <- struct{}{} },
	)
	require.PanicsWithError(
		t,
		"send on closed channel",
		func() { kvledger.snapshotMgr.requestResponses <- &requestResponse{} },
	)

	// close ledger again is fine
	l.Close()
}

func TestSnapshotRequestsErrorPaths(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})

	// create a ledger with genesis block
	ledgerID := "testsnapshotrequestserrorpaths"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer l.Close()

	// commit blocks and submit a request for the block height so that a snapshot is generated
	testutilCommitBlocks(t, l, bg, 5, gbHash)
	require.NoError(t, l.SubmitSnapshotRequest(5))

	// wait until snapshot height=5 is generated
	kvledger := l.(*kvLedger)
	snapshotExists := func() bool {
		exists, err := kvledger.snapshotExists(5)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// verify various error paths
	require.EqualError(t, l.SubmitSnapshotRequest(5), "snapshot already generated for block height 5")

	require.EqualError(t, l.SubmitSnapshotRequest(3), "requested snapshot height 3 cannot be less than the current block height 5")

	require.NoError(t, l.SubmitSnapshotRequest(20))
	require.EqualError(t, l.SubmitSnapshotRequest(20), "duplicate snapshot request for height 20")

	require.EqualError(t, l.CancelSnapshotRequest(100), "no snapshot request exists for height 100")

	provider.Close()

	_, err = provider.Open(ledgerID)
	require.Contains(t, err.Error(), "leveldb: closed")

	err = l.SubmitSnapshotRequest(20)
	require.Contains(t, err.Error(), "leveldb: closed")

	err = l.CancelSnapshotRequest(1)
	require.Contains(t, err.Error(), "leveldb: closed")

	_, err = l.PendingSnapshotRequests()
	require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
}

func equal(slice1 []uint64, slice2 []uint64) bool {
	if len(slice1) != len(slice2) {
		return false
	}
	for i := range slice1 {
		if slice1[i] != slice2[i] {
			return false
		}
	}
	return true
}

func testutilCommitBlocks(t *testing.T, l ledger.PeerLedger, bg *testutil.BlockGenerator, newBlockHeight uint64, previousBlockHash []byte) *common.Block {
	bcInfo, err := l.GetBlockchainInfo()
	require.NoError(t, err)
	startBlockNum := bcInfo.Height

	var block *common.Block
	for i := startBlockNum; i < newBlockHeight; i++ {
		txid := util.GenerateUUID()
		simulator, err := l.NewTxSimulator(txid)
		require.NoError(t, err)
		require.NoError(t, simulator.SetState("ns1", fmt.Sprintf("key%d", i), []byte(fmt.Sprintf("value%d", i))))
		simulator.Done()
		require.NoError(t, err)
		simRes, err := simulator.GetTxSimulationResults()
		require.NoError(t, err)
		pubSimBytes, err := simRes.GetPubSimulationBytes()
		require.NoError(t, err)
		block = bg.NextBlock([][]byte{pubSimBytes})
		require.NoError(t, l.CommitLegacy(&ledger.BlockAndPvtData{Block: block}, &ledger.CommitOptions{}))

		bcInfo, err := l.GetBlockchainInfo()
		require.NoError(t, err)
		blockHash := protoutil.BlockHeaderHash(block.Header)
		require.Equal(t, &common.BlockchainInfo{
			Height: uint64(i + 1), CurrentBlockHash: blockHash, PreviousBlockHash: previousBlockHash,
		}, bcInfo)
		previousBlockHash = blockHash
	}

	return block
}
