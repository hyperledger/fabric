/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/stretchr/testify/assert"
)

func TestMetadataHintCorrectness(t *testing.T) {
	bookkeepingTestEnv := bookkeeping.NewTestEnv(t)
	defer bookkeepingTestEnv.Cleanup()
	bookkeeper := bookkeepingTestEnv.TestProvider.GetDBHandle("ledger1", bookkeeping.MetadataPresenceIndicator)

	metadataHint := newMetadataHint(bookkeeper)
	assert.False(t, metadataHint.metadataEverUsedFor("ns1"))

	updates := NewUpdateBatch()
	updates.PubUpdates.PutValAndMetadata("ns1", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.PubUpdates.PutValAndMetadata("ns2", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 2))
	updates.PubUpdates.PutValAndMetadata("ns3", "key", []byte("value"), nil, version.NewHeight(1, 3))
	updates.HashUpdates.PutValAndMetadata("ns1_pvt", "key", "coll", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.HashUpdates.PutValAndMetadata("ns2_pvt", "key", "coll", []byte("value"), []byte("metadata"), version.NewHeight(1, 3))
	updates.HashUpdates.PutValAndMetadata("ns3_pvt", "key", "coll", []byte("value"), nil, version.NewHeight(1, 3))
	metadataHint.setMetadataUsedFlag(updates)

	t.Run("MetadataAddedInCurrentSession", func(t *testing.T) {
		assert.True(t, metadataHint.metadataEverUsedFor("ns1"))
		assert.True(t, metadataHint.metadataEverUsedFor("ns2"))
		assert.True(t, metadataHint.metadataEverUsedFor("ns1_pvt"))
		assert.True(t, metadataHint.metadataEverUsedFor("ns2_pvt"))
		assert.False(t, metadataHint.metadataEverUsedFor("ns3"))
		assert.False(t, metadataHint.metadataEverUsedFor("ns4"))
	})

	t.Run("MetadataFromPersistence", func(t *testing.T) {
		metadataHintFromPersistence := newMetadataHint(bookkeeper)
		assert.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns1"))
		assert.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns2"))
		assert.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns1_pvt"))
		assert.True(t, metadataHintFromPersistence.metadataEverUsedFor("ns2_pvt"))
		assert.False(t, metadataHintFromPersistence.metadataEverUsedFor("ns3"))
		assert.False(t, metadataHintFromPersistence.metadataEverUsedFor("ns4"))
	})
}

func TestMetadataHintOptimizationSkippingGoingToDB(t *testing.T) {
	bookkeepingTestEnv := bookkeeping.NewTestEnv(t)
	defer bookkeepingTestEnv.Cleanup()
	bookkeeper := bookkeepingTestEnv.TestProvider.GetDBHandle("ledger1", bookkeeping.MetadataPresenceIndicator)

	mockVersionedDB := &mock.VersionedDB{}
	db, err := NewCommonStorageDB(mockVersionedDB, "testledger", newMetadataHint(bookkeeper))
	assert.NoError(t, err)
	updates := NewUpdateBatch()
	updates.PubUpdates.PutValAndMetadata("ns1", "key", []byte("value"), []byte("metadata"), version.NewHeight(1, 1))
	updates.PubUpdates.PutValAndMetadata("ns2", "key", []byte("value"), nil, version.NewHeight(1, 2))
	db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(1, 3))

	db.GetStateMetadata("ns1", "randomkey")
	assert.Equal(t, 1, mockVersionedDB.GetStateCallCount())
	db.GetPrivateDataMetadataByHash("ns1", "randomColl", []byte("randomKeyhash"))
	assert.Equal(t, 2, mockVersionedDB.GetStateCallCount())

	db.GetStateMetadata("ns2", "randomkey")
	db.GetPrivateDataMetadataByHash("ns2", "randomColl", []byte("randomKeyhash"))
	assert.Equal(t, 2, mockVersionedDB.GetStateCallCount())

	db.GetStateMetadata("randomeNs", "randomkey")
	db.GetPrivateDataMetadataByHash("randomeNs", "randomColl", []byte("randomKeyhash"))
	assert.Equal(t, 2, mockVersionedDB.GetStateCallCount())
}
