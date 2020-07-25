/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/mock"
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
