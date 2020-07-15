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
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRequests(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	// create the ledger
	ledgerID := "testsnapshotrequests"
	blockHeight := uint64(10)

	// create a ledger genesis block
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.Create(gb)
	require.NoError(t, err)
	defer l.Close()

	bcInfo, _ := l.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	// submit a request at height 5
	kvledger := l.(*kvLedger)
	require.NoError(t, l.SubmitSnapshotRequest(5))
	require.Equal(t, uint64(5), kvledger.snapshotRequestBookkeeper.smallestRequestHeight)

	// commit 10 blocks
	testutilCommitBlocks(t, l, bg, 1, blockHeight, gbHash)

	// wait until request height=5 is done
	listSnapshotDirs := func() bool {
		exists, _ := kvledger.snapshotExists(5)
		return exists
	}
	require.Eventually(t, listSnapshotDirs, time.Minute, 3*time.Second)

	// submit a snapshot request at height=0 so that generateSnapshot is triggered
	require.NoError(t, l.SubmitSnapshotRequest(0))

	// wait until request height=0 is done
	listSnapshotDirs = func() bool {
		exists, _ := kvledger.snapshotExists(blockHeight)
		return exists
	}
	require.Eventually(t, listSnapshotDirs, time.Minute, 3*time.Second)

	exists, err := kvledger.snapshotExists(3)
	require.Error(t, err, "abc")
	require.False(t, exists)

	// submit snapshot requests higher than block height
	require.NoError(t, l.SubmitSnapshotRequest(100))
	require.Equal(t, uint64(100), kvledger.snapshotRequestBookkeeper.smallestRequestHeight)

	require.NoError(t, l.SubmitSnapshotRequest(15))
	require.Equal(t, uint64(15), kvledger.snapshotRequestBookkeeper.smallestRequestHeight)

	require.NoError(t, l.SubmitSnapshotRequest(50))
	require.Equal(t, uint64(15), kvledger.snapshotRequestBookkeeper.smallestRequestHeight)

	requestHeights, err := l.PendingSnapshotRequests()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{15, 50, 100})

	provider.Close()

	// reopen the provider and ledger
	provider2 := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider2.Close()
	l2, err := provider2.Open(ledgerID)
	require.NoError(t, err)
	kvledger2 := l2.(*kvLedger)

	requestHeights, err = l2.PendingSnapshotRequests()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{15, 50, 100})
	require.Equal(t, uint64(15), kvledger2.snapshotRequestBookkeeper.smallestRequestHeight)

	// cancel snapshot requests
	require.NoError(t, l2.CancelSnapshotRequest(100))
	require.Equal(t, uint64(15), kvledger2.snapshotRequestBookkeeper.smallestRequestHeight)

	require.NoError(t, l2.CancelSnapshotRequest(15))
	require.Equal(t, uint64(50), kvledger2.snapshotRequestBookkeeper.smallestRequestHeight)

	require.NoError(t, l2.CancelSnapshotRequest(50))
	require.Equal(t, defaultSmallestHeight, kvledger2.snapshotRequestBookkeeper.smallestRequestHeight)

	requestHeights, err = l2.PendingSnapshotRequests()
	require.NoError(t, err)
	require.ElementsMatch(t, requestHeights, []uint64{})
}

func TestSnapshotRequests_ErrorPaths(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testsnapshotrequests-errorpaths"
	blockHeight := uint64(5)
	// create a ledger genesis block
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	l, err := provider.Create(gb)
	require.NoError(t, err)
	defer l.Close()

	bcInfo, _ := l.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	// commit blocks
	testutilCommitBlocks(t, l, bg, 1, blockHeight, gbHash)

	require.NoError(t, l.SubmitSnapshotRequest(20))
	require.EqualError(t, l.SubmitSnapshotRequest(20), "duplicate snapshot request for height 20")

	require.EqualError(t, l.SubmitSnapshotRequest(3), fmt.Sprintf("requested snapshot height 3 cannot be less than the current block height %d", blockHeight))

	require.EqualError(t, l.CancelSnapshotRequest(100), "no pending snapshot request has height 100")

	provider.Close()

	err = l.SubmitSnapshotRequest(20)
	require.Contains(t, err.Error(), "leveldb: closed")

	err = l.CancelSnapshotRequest(1)
	require.Contains(t, err.Error(), "leveldb: closed")

	_, err = l.PendingSnapshotRequests()
	require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
}

func testutilCommitBlocks(t *testing.T, l ledger.PeerLedger, bg *testutil.BlockGenerator, startBlockNum, blockHeight uint64, previousBlockHash []byte) {
	var block *common.Block
	for i := startBlockNum; i < blockHeight; i++ {
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
}
