/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/mock"
	"github.com/stretchr/testify/require"
)

func TestMetadataHintCorrectness(t *testing.T) {
	bookkeepingTestEnv := bookkeeping.NewTestEnv(t)
	defer bookkeepingTestEnv.Cleanup()
	bookkeeper := bookkeepingTestEnv.TestProvider.GetDBHandle("ledger1", bookkeeping.MetadataPresenceIndicator)

	metadataHint, err := newMetadataHint(bookkeeper)
	require.NoError(t, err)
	require.False(t, metadataHint.metadataEverUsedFor("ns1"))

	updates := NewUpdateBatch()
	updates.PubUpdates.PutValAndMetadata("ns1", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.PubUpdates.PutValAndMetadata("ns2", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 2))
	updates.PubUpdates.PutValAndMetadata("ns3", "key", []byte("value"), nil, version.NewHeight(1, 3))
	updates.HashUpdates.PutValAndMetadata("ns1_pvt", "key", "coll", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.HashUpdates.PutValAndMetadata("ns2_pvt", "key", "coll", []byte("value"), []byte("metadata"), version.NewHeight(1, 3))
	updates.HashUpdates.PutValAndMetadata("ns3_pvt", "key", "coll", []byte("value"), nil, version.NewHeight(1, 3))
	require.NoError(t, metadataHint.setMetadataUsedFlag(updates))

	t.Run("MetadataAddedInCurrentSession", func(t *testing.T) {
		require.True(t, metadataHint.metadataEverUsedFor("ns1"))
		require.True(t, metadataHint.metadataEverUsedFor("ns2"))
		require.True(t, metadataHint.metadataEverUsedFor("ns1_pvt"))
		require.True(t, metadataHint.metadataEverUsedFor("ns2_pvt"))
		require.False(t, metadataHint.metadataEverUsedFor("ns3"))
		require.False(t, metadataHint.metadataEverUsedFor("ns4"))
	})

	t.Run("MetadataFromPersistence", func(t *testing.T) {
		metadataHintFromPersistence, err := newMetadataHint(bookkeeper)
		require.NoError(t, err)
		require.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns1"))
		require.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns2"))
		require.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns1_pvt"))
		require.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns2_pvt"))
		require.False(t, metadataHintFromPersistence.metadataEverUsedFor("ns3"))
		require.False(t, metadataHintFromPersistence.metadataEverUsedFor("ns4"))
	})

	t.Run("MetadataIterErrorPath", func(t *testing.T) {
		bookkeepingTestEnv.TestProvider.Close()
		metadataHint, err := newMetadataHint(bookkeeper)
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
		require.Nil(t, metadataHint)
	})
}

func TestMetadataHintOptimizationSkippingGoingToDB(t *testing.T) {
	bookkeepingTestEnv := bookkeeping.NewTestEnv(t)
	defer bookkeepingTestEnv.Cleanup()
	bookkeeper := bookkeepingTestEnv.TestProvider.GetDBHandle("ledger1", bookkeeping.MetadataPresenceIndicator)

	mockVersionedDB := &mock.VersionedDB{}
	metadatahint, err := newMetadataHint(bookkeeper)
	require.NoError(t, err)
	db, err := NewDB(mockVersionedDB, "testledger", metadatahint)
	require.NoError(t, err)
	updates := NewUpdateBatch()
	updates.PubUpdates.PutValAndMetadata("ns1", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.PubUpdates.PutValAndMetadata("ns2", "key", []byte("value"), nil, version.NewHeight(1, 2))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(1, 3)))

	_, err = db.GetStateMetadata("ns1", "randomkey")
	require.NoError(t, err)
	require.Equal(t, 1, mockVersionedDB.GetStateCallCount())
	_, err = db.GetPrivateDataMetadataByHash("ns1", "randomColl", []byte("randomKeyhash"))
	require.NoError(t, err)
	require.Equal(t, 2, mockVersionedDB.GetStateCallCount())

	_, err = db.GetStateMetadata("ns2", "randomkey")
	require.NoError(t, err)
	_, err = db.GetPrivateDataMetadataByHash("ns2", "randomColl", []byte("randomKeyhash"))
	require.NoError(t, err)
	require.Equal(t, 2, mockVersionedDB.GetStateCallCount())

	_, err = db.GetStateMetadata("randomeNs", "randomkey")
	require.NoError(t, err)
	_, err = db.GetPrivateDataMetadataByHash("randomeNs", "randomColl", []byte("randomKeyhash"))
	require.NoError(t, err)
	require.Equal(t, 2, mockVersionedDB.GetStateCallCount())
}
