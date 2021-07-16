/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

func TestSystemNamespaceIsEmptyString(t *testing.T) {
	require.Equal(t, "", systemNamespace)
}

func TestUnderDeletionValue(t *testing.T) {
	require.Equal(t, []byte("UNDER_DELETION"), underDeletionKey)
}

func TestMarkStorageForDelete(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()
	ledgerID := "doomed-transient-storage"

	// env.storeProvider is an interface type StoreProvider.  Use a golang
	// type assertion to cast this as a *storeProvider, so that we may test
	// the package level functions received by this struct.
	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)

	// level db used to store UNDER_DELETION status
	syshandle := sp.dbProvider.GetDBHandle(systemNamespace)
	require.NotNil(t, syshandle)

	isEmpty, err := syshandle.IsEmpty()
	require.NoError(t, err)
	require.True(t, isEmpty)

	// Mark the transient storage for deletion.
	err = sp.markStorageForDelete(ledgerID)
	require.NoError(t, err)

	// sysdb should now have a key UNDER_DELETION containing a proto message
	isEmpty, err = syshandle.IsEmpty()
	require.NoError(t, err)
	require.False(t, isEmpty)

	val, err := syshandle.Get(underDeletionKey)
	require.NoError(t, err)
	require.NotNil(t, val)

	// the value should be a json array containing the doomed ledger ID
	var deleteList []string
	err = json.Unmarshal(val, &deleteList)
	require.NoError(t, err)
	require.NotNil(t, deleteList)
	require.Contains(t, deleteList, ledgerID)
}

func TestMultipleStoragesMarkedForDeletion(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	doomed1 := "doomed-1"
	doomed2 := "doomed-2"
	doomed3 := "doomed-3"

	// env.storeProvider is an interface type StoreProvider.  Use a golang
	// type assertion to cast this as a *storeProvider, so that we may test
	// the package level functions received by this struct.
	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)

	// Delete list is empty to start.
	dl, err := sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(dl))

	// mark a deletion
	require.NoError(t, sp.markStorageForDelete(doomed1))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 1, len(dl))
	require.Contains(t, dl, doomed1)

	// mark it again - it should not contain duplicate entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 1, len(dl))
	require.Contains(t, dl, doomed1)

	// add multiple entries
	require.NoError(t, sp.markStorageForDelete(doomed2))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 2, len(dl))
	require.Contains(t, dl, doomed1)
	require.Contains(t, dl, doomed2)

	require.NoError(t, sp.markStorageForDelete(doomed3))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl))
	require.Contains(t, dl, doomed1)
	require.Contains(t, dl, doomed2)
	require.Contains(t, dl, doomed3)
}

func TestUnmarkDeletionTag(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	doomed1 := "doomed-1"
	doomed2 := "doomed-2"
	doomed3 := "doomed-3"

	// env.storeProvider is an interface type StoreProvider.  Use a golang
	// type assertion to cast this as a *storeProvider, so that we may test
	// the package level functions received by this struct.
	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)

	// Delete list is empty to start.
	dl, err := sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(dl))

	// doom some transient storage entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	require.NoError(t, sp.markStorageForDelete(doomed2))
	require.NoError(t, sp.markStorageForDelete(doomed3))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl))
	require.Contains(t, dl, doomed1)
	require.Contains(t, dl, doomed2)
	require.Contains(t, dl, doomed3)

	// clear out the doomed flag for one of the storage
	require.NoError(t, sp.clearStorageDeletionStatus(doomed2))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 2, len(dl))
	require.Contains(t, dl, doomed1)
	require.NotContains(t, dl, doomed2)
	require.Contains(t, dl, doomed3)
}

func TestClearDeletionTagNotPresent(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	doomed1 := "doomed-1"
	doomed2 := "doomed-2"
	doomed3 := "doomed-3"
	invalid := "invalid-storage-does-not-exist"

	// env.storeProvider is an interface type StoreProvider.  Use a golang
	// type assertion to cast this as a *storeProvider, so that we may test
	// the package level functions received by this struct.
	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)

	// Delete list is empty to start.
	dl, err := sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(dl))

	// doom some transient storage entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	require.NoError(t, sp.markStorageForDelete(doomed2))
	require.NoError(t, sp.markStorageForDelete(doomed3))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl))
	require.Contains(t, dl, doomed1)
	require.Contains(t, dl, doomed2)
	require.Contains(t, dl, doomed3)

	// Invalid should not have been doomed.
	require.NotContains(t, dl, invalid)
}

func TestProcessPendingStorageDeletions(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)

	doomed1 := "doomed-1"
	doomed2 := "doomed-2"
	doomed3 := "doomed-3"

	// Set up some pending deletions
	require.NoError(t, sp.markStorageForDelete(doomed1))
	require.NoError(t, sp.markStorageForDelete(doomed2))
	require.NoError(t, sp.markStorageForDelete(doomed3))

	dl, err := sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl))
	require.Contains(t, dl, doomed1)
	require.Contains(t, dl, doomed2)
	require.Contains(t, dl, doomed3)

	// process the pending deletions
	err = sp.processPendingStorageDeletions()
	require.NoError(t, err)

	// storages are no longer pending deletion
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(dl))
}

// Drop a storage without access to a provider.
func TestPackageDropStore(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	populateTestStore(t, env.store)
	empty, err := env.store.db.IsEmpty()
	require.NoError(t, err)
	require.False(t, empty)

	// Storage must be closed before dropping.
	ledgerID := env.store.ledgerID
	env.storeProvider.Close()

	// drop the storage
	require.NoError(t, Drop(env.tempdir, ledgerID))

	sp, err := NewStoreProvider(env.tempdir)
	require.NoError(t, err)
	require.NotNil(t, sp)
	defer sp.Close()

	store, err := sp.OpenStore(ledgerID)
	require.NoError(t, err)
	require.NotNil(t, store)

	verifyStoreDropped(t, store)
}

// Peer must be shut down
func TestPackageDropWithPeerLockIsError(t *testing.T) {
	env := initTestEnv(t)
	defer env.cleanup()

	env.storeProvider.Close()

	lockPath := filepath.Join(env.tempdir, "fileLock")
	fileLock := leveldbhelper.NewFileLock(lockPath)
	require.NoError(t, fileLock.Lock())
	defer fileLock.Unlock()

	require.ErrorContains(t, Drop(env.tempdir, "drop-with-lock"),
		"as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying: lock is already acquired on file")
}

// Write some transactions into the transient storage.
func populateTestStore(t *testing.T, store *Store) {
	samplePvtSimResWithConfig := samplePvtDataWithConfigInfo(t)
	testTxid := "testTxid"
	numEntries := 5
	for i := 0; i < numEntries; i++ {
		store.Persist(testTxid, uint64(i), samplePvtSimResWithConfig)
	}
}

// After dropping a store, verify it is empty
func verifyStoreDropped(t *testing.T, store *Store) {
	height, err := store.GetMinTransientBlkHt()
	require.Error(t, err, "Transient store is empty")
	require.Equal(t, height, uint64(0))

	isEmpty, err := store.db.IsEmpty()
	require.NoError(t, err)
	require.True(t, isEmpty)
}
