/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

func TestSystemNamespaceIsEmptyString(t *testing.T) {
	require.Equal(t, "", systemNamespace)
}

func TestUnderDeletionValue(t *testing.T) {
	require.Equal(t, []byte("UNDER_DELETION"), underDeletionKey)
}

func TestNewStoreProvider(t *testing.T) {
	tempdir := t.TempDir()

	storedir := filepath.Join(tempdir, "transientstore")
	p, err := NewStoreProvider(storedir)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestMarkStorageForDelete(t *testing.T) {
	env := initTestEnv(t)
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

	// the value should be a proto.Message containing a list with the doomed ledger ID
	delete := PendingDeleteStorageList{}
	err = proto.Unmarshal(val, &delete)
	require.NoError(t, err)
	require.NotNil(t, delete.List)
	require.Contains(t, delete.List, ledgerID)
}

func TestMultipleStoragesMarkedForDeletion(t *testing.T) {
	env := initTestEnv(t)

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
	require.Equal(t, 0, len(dl.List))

	// mark a deletion
	require.NoError(t, sp.markStorageForDelete(doomed1))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 1, len(dl.List))
	require.Contains(t, dl.List, doomed1)

	// mark it again - it should not contain duplicate entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 1, len(dl.List))
	require.Contains(t, dl.List, doomed1)

	// add multiple entries
	require.NoError(t, sp.markStorageForDelete(doomed2))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 2, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)

	require.NoError(t, sp.markStorageForDelete(doomed3))
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)
}

func TestUnmarkDeletionTag(t *testing.T) {
	env := initTestEnv(t)

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
	require.Equal(t, 0, len(dl.List))

	// doom some transient storage entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	require.NoError(t, sp.markStorageForDelete(doomed2))
	require.NoError(t, sp.markStorageForDelete(doomed3))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)

	// clear out the doomed flag for one of the storage
	require.NoError(t, sp.clearStorageDeletionStatus(doomed2))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 2, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.NotContains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)
}

func TestClearDeletionTagNotPresent(t *testing.T) {
	env := initTestEnv(t)

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
	require.Equal(t, 0, len(dl.List))

	// Check the boundary case of removing an invalid ledger from an empty set.
	require.NoError(t, sp.clearStorageDeletionStatus(invalid))

	// doom some transient storage entries
	require.NoError(t, sp.markStorageForDelete(doomed1))
	require.NoError(t, sp.markStorageForDelete(doomed2))
	require.NoError(t, sp.markStorageForDelete(doomed3))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)

	// Try again to clear the deletion flag for an invalid ledger.
	require.NotContains(t, dl.List, invalid)
	require.NoError(t, sp.clearStorageDeletionStatus(invalid))

	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 3, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)

	// Invalid should not have been doomed.
	require.NotContains(t, dl.List, invalid)
}

func TestProcessPendingStorageDeletions(t *testing.T) {
	env := initTestEnv(t)

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
	require.Equal(t, 3, len(dl.List))
	require.Contains(t, dl.List, doomed1)
	require.Contains(t, dl.List, doomed2)
	require.Contains(t, dl.List, doomed3)

	// process the pending deletions
	err = sp.processPendingStorageDeletions()
	require.NoError(t, err)

	// storages are no longer pending deletion
	dl, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(dl.List))
}

// Drop a storage without access to a provider.
func TestPackageDropTransientStorage(t *testing.T) {
	env := initTestEnv(t)

	populateTestStore(t, env.store)
	empty, err := env.store.db.IsEmpty()
	require.NoError(t, err)
	require.False(t, empty)

	// Storage must be closed before dropping.
	ledgerID := env.store.ledgerID
	env.storeProvider.Close()

	// drop the storage
	require.NoError(t, Drop(env.storedir, ledgerID))

	sp, err := NewStoreProvider(env.storedir)
	require.NoError(t, err)
	require.NotNil(t, sp)
	defer sp.Close()

	store, err := sp.OpenStore(ledgerID)
	require.NoError(t, err)
	require.NotNil(t, store)

	verifyStoreDropped(t, store)
}

func TestLockFileIsAdjacentToTransientStorageFolder(t *testing.T) {
	env := initTestEnv(t)

	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)
	require.NotNil(t, sp.fileLock)
	require.True(t, sp.fileLock.IsLocked())

	// we can't quite get to the path of the lock.  But we can construct a new lock with the correct path.
	lockPath := filepath.Join(env.tempdir, transientStorageLockName) // note: PARENT dir of the transient storage.
	fileLock := leveldbhelper.NewFileLock(lockPath)

	err := fileLock.Lock()
	require.ErrorContains(t, err, "lock is already acquired on file")
}

// Make sure that closing the transient storage releases the peer start file lock.
func TestCloseStoreReleasesFileLock(t *testing.T) {
	env := initTestEnv(t)

	require.NotNil(t, env.storeProvider)
	sp := env.storeProvider.(*storeProvider)

	// lock should be held and locked
	require.NotNil(t, sp.fileLock)
	require.True(t, sp.fileLock.IsLocked())
	require.ErrorContains(t, sp.fileLock.Lock(), "lock is already acquired on file")

	// after close the lock may be acquired
	sp.Close()
	require.False(t, sp.fileLock.IsLocked())

	require.NoError(t, sp.fileLock.Lock())
	sp.fileLock.Unlock()
}

// There may be only one transient storage provider open at a time.
func TestOneAndOnlyOneTransientStorageProviderMayBeOpened(t *testing.T) {
	env := initTestEnv(t)

	// env already has a storage provider opened.  Close it to make the double open case explicit.
	env.storeProvider.Close()

	// open the first provider
	sp, err := NewStoreProvider(env.storedir)
	require.NoError(t, err)
	require.NotNil(t, sp)

	// opening a second provider is an error
	_, err = NewStoreProvider(env.storedir)
	require.ErrorContains(t, err, "as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying: lock is already acquired on file")

	// After closing the provider it may be reopened.
	sp.Close()

	sp, err = NewStoreProvider(env.storedir)
	require.NoError(t, err)
	require.NotNil(t, sp)
	defer sp.Close()
}

// Drop() while the peer is open is a lock error.
func TestDropWithRunningPeerIsError(t *testing.T) {
	env := initTestEnv(t)

	// env maintains an opened transient storage provider.
	sp := env.storeProvider.(*storeProvider)
	require.NotNil(t, sp)
	require.True(t, sp.fileLock.IsLocked())

	// Dropping while a provider is open is an error.
	err := Drop(env.storedir, "some-magical-invalid-ledger-which-does-not-exist")
	require.Error(t, err, "as another peer node command is executing,"+
		" wait for that command to complete its execution or terminate it before retrying")
}

// Same test as above, but checks with a simulated lock and a closed provider.
func TestPackageDropWithPeerLockIsError(t *testing.T) {
	env := initTestEnv(t)

	env.storeProvider.Close()

	// lock is at the PARENT directory of the storage folder.
	lockPath := filepath.Join(env.tempdir, transientStorageLockName)
	fileLock := leveldbhelper.NewFileLock(lockPath)
	require.NoError(t, fileLock.Lock())
	defer fileLock.Unlock()

	require.ErrorContains(t, Drop(env.storedir, "drop-with-lock"),
		"as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying")
}

// Mimic the crash recovery scenario: pending transient stores are deleted at peer / provider launch
func TestProviderRestartAfterFailedDeletionScrubsPendingDeletions(t *testing.T) {
	env := initTestEnv(t)

	ledgerID := "vanishing-ledger"
	sp := env.storeProvider.(*storeProvider)

	// write some data to the transient storage
	populateTestStore(t, env.store)
	empty, err := env.store.db.IsEmpty()
	require.NoError(t, err)
	require.False(t, empty)

	// mark a storage for deletion, but do not actually delete it.
	require.NoError(t, sp.markStorageForDelete(ledgerID))
	doomed, err := sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 1, len(doomed.List))
	require.Contains(t, doomed.List, ledgerID)

	// close and re-open the provider.
	env.storeProvider.Close()

	// re-opening the provider will trigger the processPendingStorageDeletions()
	env.storeProvider, err = NewStoreProvider(env.storedir)
	require.NoError(t, err)
	sp = env.storeProvider.(*storeProvider)

	// nothing tagged for deletion
	doomed, err = sp.getStorageMarkedForDeletion()
	require.NoError(t, err)
	require.Equal(t, 0, len(doomed.List))

	// storage should be empty after re-opening
	store, err := sp.OpenStore(ledgerID)
	require.NoError(t, err)
	require.NotNil(t, store)

	verifyStoreDropped(t, store)
}

func TestDeleteTransientStorageIsCaseSensitive(t *testing.T) {
	env := initTestEnv(t)

	sp := env.storeProvider.(*storeProvider)

	store1ID := "case-sensitive-storage"
	store2ID := "CaSe-SeNsItIvE-sToRaGe"
	require.Equal(t, strings.ToLower(store1ID), strings.ToLower(store2ID))

	store1, err := sp.OpenStore(store1ID)
	require.NoError(t, err)
	require.NotNil(t, store1)
	populateTestStore(t, store1)

	store2, err := sp.OpenStore(store2ID)
	require.NoError(t, err)
	require.NotNil(t, store2)
	populateTestStore(t, store2)

	// drop store1.
	require.NoError(t, sp.deleteStore(store1ID))
	verifyStoreDropped(t, store1)

	// store2 should not have been dropped.
	empty, err := store2.db.IsEmpty()
	require.NoError(t, err)
	require.False(t, empty)
}

// Write some transactions into the transient storage.
func populateTestStore(t *testing.T, store *Store) {
	samplePvtSimResWithConfig := samplePvtDataWithConfigInfo(t)
	testTxid := "testTxid"
	numEntries := 5
	for i := 0; i < numEntries; i++ {
		require.NoError(t, store.Persist(testTxid, uint64(i), samplePvtSimResWithConfig))
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
