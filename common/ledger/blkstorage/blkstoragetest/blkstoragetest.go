/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstoragetest

import (
	"crypto/sha256"
	"hash"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var (
	testNewHashFunc = func() (hash.Hash, error) {
		return sha256.New(), nil
	}

	attrsToIndex = []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
	}
)

// BootstrapBlockstoreFromSnapshot does the following:
// - create a block store using the provided blocks
// - generate a snapshot from the block store
// - bootstrap another block store from the snapshot
func BootstrapBlockstoreFromSnapshot(t *testing.T, ledgerName string, blocks []*common.Block) (*blkstorage.BlockStore, func()) {
	require.NotEqual(t, 0, len(blocks))

	testDir := t.TempDir()
	snapshotDir := filepath.Join(testDir, "snapshot")
	require.NoError(t, os.Mkdir(snapshotDir, 0o755))

	conf := blkstorage.NewConf(testDir, 0)
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	provider, err := blkstorage.NewProvider(conf, indexConfig, &disabled.Provider{})
	require.NoError(t, err)

	// create an original store from the provided blocks so that we can create a snapshot
	originalBlkStore, err := provider.Open(ledgerName + "original")
	require.NoError(t, err)

	for _, block := range blocks {
		require.NoError(t, originalBlkStore.AddBlock(block))
	}

	_, err = originalBlkStore.ExportTxIds(snapshotDir, testNewHashFunc)
	require.NoError(t, err)

	lastBlockInSnapshot := blocks[len(blocks)-1]
	snapshotInfo := &blkstorage.SnapshotInfo{
		LastBlockHash:     protoutil.BlockHeaderHash(lastBlockInSnapshot.Header),
		LastBlockNum:      lastBlockInSnapshot.Header.Number,
		PreviousBlockHash: lastBlockInSnapshot.Header.PreviousHash,
	}

	err = provider.ImportFromSnapshot(ledgerName, snapshotDir, snapshotInfo)
	require.NoError(t, err)
	blockStore, err := provider.Open(ledgerName)
	require.NoError(t, err)

	cleanup := func() {
		provider.Close()
	}
	return blockStore, cleanup
}
