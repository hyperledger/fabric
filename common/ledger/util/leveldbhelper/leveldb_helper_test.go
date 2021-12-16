/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package leveldbhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
)

func TestLevelDBHelperWriteWithoutOpen(t *testing.T) {
	env := newTestDBEnv(t, testDBPath)
	defer env.cleanup()
	db := env.db
	defer func() {
		if recover() == nil {
			t.Fatalf("A panic is expected when writing to db before opening")
		}
	}()
	require.NoError(t, db.Put([]byte("key"), []byte("value"), false))
}

func TestLevelDBHelperReadWithoutOpen(t *testing.T) {
	env := newTestDBEnv(t, testDBPath)
	defer env.cleanup()
	db := env.db
	defer func() {
		if recover() == nil {
			t.Fatalf("A panic is expected when writing to db before opening")
		}
	}()
	_, err := db.Get([]byte("key"))
	require.NoError(t, err)
}

func TestLevelDBHelper(t *testing.T) {
	env := newTestDBEnv(t, testDBPath)
	// defer env.cleanup()
	db := env.db

	db.Open()
	// second time open should not have any side effect
	db.Open()
	IsEmpty, err := db.IsEmpty()
	require.NoError(t, err)
	require.True(t, IsEmpty)
	require.NoError(t, db.Put([]byte("key1"), []byte("value1"), false))
	require.NoError(t, db.Put([]byte("key2"), []byte("value2"), true))
	require.NoError(t, db.Put([]byte("key3"), []byte("value3"), true))

	val, _ := db.Get([]byte("key2"))
	require.Equal(t, "value2", string(val))

	require.NoError(t, db.Delete([]byte("key1"), false))
	require.NoError(t, db.Delete([]byte("key2"), true))

	val1, err1 := db.Get([]byte("key1"))
	require.NoError(t, err1, "")
	require.Equal(t, "", string(val1))

	val2, err2 := db.Get([]byte("key2"))
	require.NoError(t, err2, "")
	require.Equal(t, "", string(val2))

	db.Close()
	// second time Close should not have any side effect
	db.Close()

	_, err = db.IsEmpty()
	require.Error(t, err)

	val3, err3 := db.Get([]byte("key3"))
	require.Error(t, err3)
	require.Equal(t, "", string(val3))

	db.Open()
	IsEmpty, err = db.IsEmpty()
	require.NoError(t, err)
	require.False(t, IsEmpty)

	batch := &leveldb.Batch{}
	batch.Put([]byte("key1"), []byte("value1"))
	batch.Put([]byte("key2"), []byte("value2"))
	batch.Delete([]byte("key3"))
	require.NoError(t, db.WriteBatch(batch, true))

	val1, err1 = db.Get([]byte("key1"))
	require.NoError(t, err1, "")
	require.Equal(t, "value1", string(val1))

	val2, err2 = db.Get([]byte("key2"))
	require.NoError(t, err2, "")
	require.Equal(t, "value2", string(val2))

	val3, err3 = db.Get([]byte("key3"))
	require.NoError(t, err3, "")
	require.Equal(t, "", string(val3))

	keys := []string{}
	itr := db.GetIterator(nil, nil)
	for itr.Next() {
		keys = append(keys, string(itr.Key()))
	}
	require.Equal(t, []string{"key1", "key2"}, keys)
}

func TestFileLock(t *testing.T) {
	// create 1st fileLock manager
	fileLockPath := testDBPath + "/fileLock"
	fileLock1 := NewFileLock(fileLockPath)
	require.Nil(t, fileLock1.db)
	require.Equal(t, fileLock1.filePath, fileLockPath)

	// acquire the file lock using the fileLock manager 1
	err := fileLock1.Lock()
	require.NoError(t, err)
	require.NotNil(t, fileLock1.db)

	// create 2nd fileLock manager
	fileLock2 := NewFileLock(fileLockPath)
	require.Nil(t, fileLock2.db)
	require.Equal(t, fileLock2.filePath, fileLockPath)

	// try to acquire the file lock again using the fileLock2
	// would result in an error
	err = fileLock2.Lock()
	expectedErr := fmt.Sprintf("lock is already acquired on file %s", fileLockPath)
	require.EqualError(t, err, expectedErr)
	require.Nil(t, fileLock2.db)

	// release the file lock acquired using fileLock1
	fileLock1.Unlock()
	require.Nil(t, fileLock1.db)

	// As the fileLock1 has released the lock,
	// the fileLock2 can acquire the lock.
	err = fileLock2.Lock()
	require.NoError(t, err)
	require.NotNil(t, fileLock2.db)

	// release the file lock acquired using fileLock 2
	fileLock2.Unlock()
	require.Nil(t, fileLock1.db)

	// unlock can be called multiple times and it is safe
	fileLock2.Unlock()
	require.Nil(t, fileLock1.db)

	// cleanup
	require.NoError(t, os.RemoveAll(fileLockPath))
}

func TestFileLockLockUnlockLock(t *testing.T) {
	// create an open lock
	lockPath := testDBPath + "/fileLock"
	lock := NewFileLock(lockPath)
	require.Nil(t, lock.db)
	require.Equal(t, lock.filePath, lockPath)
	require.False(t, lock.IsLocked())

	defer lock.Unlock()
	defer os.RemoveAll(lockPath)

	// lock
	require.NoError(t, lock.Lock())
	require.True(t, lock.IsLocked())

	// lock
	require.ErrorContains(t, lock.Lock(), "lock is already acquired")

	// unlock
	lock.Unlock()
	require.False(t, lock.IsLocked())

	// lock - this should not error
	require.NoError(t, lock.Lock())
	require.True(t, lock.IsLocked())
}

func TestCreateDBInEmptyDir(t *testing.T) {
	require.NoError(t, os.RemoveAll(testDBPath), "")
	require.NoError(t, os.MkdirAll(testDBPath, 0o775), "")
	db := CreateDB(&Conf{DBPath: testDBPath})
	defer db.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Panic is not expected when opening db in an existing empty dir. %s", r)
		}
	}()
	db.Open()
}

func TestCreateDBInNonEmptyDir(t *testing.T) {
	require.NoError(t, os.RemoveAll(testDBPath), "")
	require.NoError(t, os.MkdirAll(testDBPath, 0o775), "")
	file, err := os.Create(filepath.Join(testDBPath, "dummyfile.txt"))
	require.NoError(t, err, "")
	file.Close()
	db := CreateDB(&Conf{DBPath: testDBPath})
	defer db.Close()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("A panic is expected when opening db in an existing non-empty dir. %s", r)
		}
	}()
	db.Open()
}
