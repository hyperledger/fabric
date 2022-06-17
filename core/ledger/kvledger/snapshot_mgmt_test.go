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
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testsnapshotrequestbookkeeper"
	dbHandle := provider.bookkeepingProvider.GetDBHandle(ledgerID, bookkeeping.SnapshotRequest)
	bookkeeper, err := newSnapshotRequestBookkeeper("test-ledger", dbHandle)
	require.NoError(t, err)

	// add requests and verify smallestRequest
	require.NoError(t, bookkeeper.add(100))
	require.Equal(t, uint64(100), bookkeeper.smallestRequestBlockNum)

	require.NoError(t, bookkeeper.add(15))
	require.Equal(t, uint64(15), bookkeeper.smallestRequestBlockNum)

	require.NoError(t, bookkeeper.add(50))
	require.Equal(t, uint64(15), bookkeeper.smallestRequestBlockNum)

	requestBlockNums, err := bookkeeper.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestBlockNums, []uint64{15, 50, 100})

	for _, blockNumber := range []uint64{15, 50, 100} {
		exist, err := bookkeeper.exist(blockNumber)
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
	bookkeeper2, err := newSnapshotRequestBookkeeper("test-ledger", dbHandle2)
	require.NoError(t, err)

	requestBlockNums, err = bookkeeper2.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestBlockNums, []uint64{15, 50, 100})

	require.Equal(t, uint64(15), bookkeeper2.smallestRequestBlockNum)

	// delete requests and verify smallest request
	require.NoError(t, bookkeeper2.delete(100))
	require.Equal(t, uint64(15), bookkeeper2.smallestRequestBlockNum)

	require.NoError(t, bookkeeper2.delete(15))
	require.Equal(t, uint64(50), bookkeeper2.smallestRequestBlockNum)

	require.NoError(t, bookkeeper2.delete(50))
	require.Equal(t, defaultSmallestBlockNumber, bookkeeper2.smallestRequestBlockNum)

	requestBlockNums, err = bookkeeper2.list()
	require.NoError(t, err)
	require.ElementsMatch(t, requestBlockNums, []uint64{})
}

func TestSnapshotRequestBookKeeperErrorPaths(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	dbHandle := provider.bookkeepingProvider.GetDBHandle("testrequestbookkeepererrorpaths", bookkeeping.SnapshotRequest)
	bookkeeper2, err := newSnapshotRequestBookkeeper("test-ledger", dbHandle)
	require.NoError(t, err)

	require.NoError(t, bookkeeper2.add(20))
	require.EqualError(t, bookkeeper2.add(20), "duplicate snapshot request for block number 20")
	require.EqualError(t, bookkeeper2.delete(100), "no snapshot request exists for block number 100")

	provider.Close()

	_, err = newSnapshotRequestBookkeeper("test-ledger", dbHandle)
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
	conf := testConfig(t)
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
	for _, blockNumber := range []uint64{100, 5, 3, 10, 30} {
		go func(blockNumber uint64) {
			require.NoError(t, l.SubmitSnapshotRequest(blockNumber))
		}(blockNumber)
	}
	// wait until all requests are submitted
	requestsUpdated := func() bool {
		requests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{3, 5, 10, 30, 100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)

	// Test 2: cancel requests in parallel and verify PendingSnapshotRequest
	for _, blockNumber := range []uint64{3, 30} {
		go func(blockNumber uint64) {
			require.NoError(t, l.CancelSnapshotRequest(blockNumber))
		}(blockNumber)
	}
	// wait until all requests are cancelled
	requestsUpdated = func() bool {
		requests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return equal(requests, []uint64{5, 10, 100})
	}
	require.Eventually(t, requestsUpdated, time.Minute, 100*time.Millisecond)

	// Test 3: commit blocks and verify snapshots are generated for blocknumber=5 and blocknumber=10
	lastBlock := testutilCommitBlocks(t, l, bg, 10, gbHash)
	// verify snapshot has been generated for blocknumber=5
	exists, err := kvledger.snapshotExists(5)
	require.NoError(t, err)
	require.True(t, exists)
	// verify snapshot is eventually generated blocknumber=10
	snapshotExists := func() bool {
		exists, err := kvledger.snapshotExists(10)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// Test 4: commit blocks and submit a request with default value (blocknumber=0) to trigger snapshot generation
	// for the latest committed block
	lastBlock = testutilCommitBlocks(t, l, bg, 20, protoutil.BlockHeaderHash(lastBlock.Header))
	require.NoError(t, l.SubmitSnapshotRequest(0))
	// wait until snapshot is generated for blocknumber=20
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
	// commit blocks upto block number 25 and add a request for block number 25 to the leveldb directly
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

	// Test 5: verify snapshot for blocknumber=25 is generated at startup time and pending snapshot requests are correct
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

func TestSnapshotMgmtConcurrency(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testsnapshotmgmtconcurrency"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	kvledger := l.(*kvLedger)
	defer kvledger.Close()

	testutilCommitBlocks(t, l, bg, 5, gbHash)

	// Artificially, send event to background goroutine to indicate that commit for block 6 has started
	// and then submit snapshot request for block 0, while not sending the event for commit done for block 6
	kvledger.snapshotMgr.events <- &event{typ: commitStart, blockNumber: 6}
	<-kvledger.snapshotMgr.commitProceed

	require.NoError(t, kvledger.SubmitSnapshotRequest(0))
	require.Eventually(t,
		func() bool {
			r, err := kvledger.snapshotMgr.snapshotRequestBookkeeper.smallestRequest()
			require.NoError(t, err)
			return r == 6
		},
		10*time.Millisecond, 1*time.Millisecond,
	)
}

func TestSnapshotMgrShutdown(t *testing.T) {
	conf := testConfig(t)
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
		func() { kvledger.snapshotMgr.events <- &event{typ: commitStart, blockNumber: 1} },
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
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})

	// create a ledger with genesis block
	ledgerID := "testsnapshotrequestserrorpaths"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer l.Close()

	// commit blocks and submit a request
	testutilCommitBlocks(t, l, bg, 5, gbHash)
	require.NoError(t, l.SubmitSnapshotRequest(5))

	// wait until snapshot blocknumber=5 is generated
	kvledger := l.(*kvLedger)
	snapshotExists := func() bool {
		exists, err := kvledger.snapshotExists(5)
		require.NoError(t, err)
		return exists
	}
	require.Eventually(t, snapshotExists, time.Minute, 100*time.Millisecond)

	// verify various error paths
	require.EqualError(t, l.SubmitSnapshotRequest(5), "snapshot already generated for block number 5")

	require.EqualError(t, l.SubmitSnapshotRequest(3), "requested snapshot for block number 3 cannot be less than the last committed block number 5")

	require.NoError(t, l.SubmitSnapshotRequest(20))
	require.EqualError(t, l.SubmitSnapshotRequest(20), "duplicate snapshot request for block number 20")

	require.EqualError(t, l.CancelSnapshotRequest(100), "no snapshot request exists for block number 100")

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

func testutilCommitBlocks(t *testing.T, l ledger.PeerLedger, bg *testutil.BlockGenerator, finalBlockNum uint64, previousBlockHash []byte) *common.Block {
	bcInfo, err := l.GetBlockchainInfo()
	require.NoError(t, err)
	startBlockNum := bcInfo.Height

	var block *common.Block
	for i := startBlockNum; i <= finalBlockNum; i++ {
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
