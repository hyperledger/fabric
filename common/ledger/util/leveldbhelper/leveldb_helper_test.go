/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leveldbhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
	db.Put([]byte("key"), []byte("value"), false)
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
	db.Get([]byte("key"))
}

func TestLevelDBHelper(t *testing.T) {
	env := newTestDBEnv(t, testDBPath)
	//defer env.cleanup()
	db := env.db

	db.Open()
	// second time open should not have any side effect
	db.Open()
	db.Put([]byte("key1"), []byte("value1"), false)
	db.Put([]byte("key2"), []byte("value2"), true)
	db.Put([]byte("key3"), []byte("value3"), true)

	val, _ := db.Get([]byte("key2"))
	assert.Equal(t, "value2", string(val))

	db.Delete([]byte("key1"), false)
	db.Delete([]byte("key2"), true)

	val1, err1 := db.Get([]byte("key1"))
	assert.NoError(t, err1, "")
	assert.Equal(t, "", string(val1))

	val2, err2 := db.Get([]byte("key2"))
	assert.NoError(t, err2, "")
	assert.Equal(t, "", string(val2))

	db.Close()
	// second time open should not have any side effect
	db.Close()

	val3, err3 := db.Get([]byte("key3"))
	assert.Error(t, err3)

	db.Open()
	batch := &leveldb.Batch{}
	batch.Put([]byte("key1"), []byte("value1"))
	batch.Put([]byte("key2"), []byte("value2"))
	batch.Delete([]byte("key3"))
	db.WriteBatch(batch, true)

	val1, err1 = db.Get([]byte("key1"))
	assert.NoError(t, err1, "")
	assert.Equal(t, "value1", string(val1))

	val2, err2 = db.Get([]byte("key2"))
	assert.NoError(t, err2, "")
	assert.Equal(t, "value2", string(val2))

	val3, err3 = db.Get([]byte("key3"))
	assert.NoError(t, err3, "")
	assert.Equal(t, "", string(val3))

	keys := []string{}
	itr := db.GetIterator(nil, nil)
	for itr.Next() {
		keys = append(keys, string(itr.Key()))
	}
	assert.Equal(t, []string{"key1", "key2"}, keys)
}

func TestFileLock(t *testing.T) {
	// create 1st fileLock manager
	fileLockPath := testDBPath + "/fileLock"
	fileLock1 := NewFileLock(fileLockPath)
	assert.Nil(t, fileLock1.db)
	assert.Equal(t, fileLock1.filePath, fileLockPath)

	// acquire the file lock using the fileLock manager 1
	err := fileLock1.Lock()
	assert.NoError(t, err)
	assert.NotNil(t, fileLock1.db)

	// create 2nd fileLock manager
	fileLock2 := NewFileLock(fileLockPath)
	assert.Nil(t, fileLock2.db)
	assert.Equal(t, fileLock2.filePath, fileLockPath)

	// try to acquire the file lock again using the fileLock2
	// would result in an error
	err = fileLock2.Lock()
	expectedErr := fmt.Sprintf("lock is already acquired on file %s", fileLockPath)
	assert.EqualError(t, err, expectedErr)
	assert.Nil(t, fileLock2.db)

	// release the file lock acquired using fileLock1
	fileLock1.Unlock()
	assert.Nil(t, fileLock1.db)

	// As the fileLock1 has released the lock,
	// the fileLock2 can acquire the lock.
	err = fileLock2.Lock()
	assert.NoError(t, err)
	assert.NotNil(t, fileLock2.db)

	// release the file lock acquired using fileLock 2
	fileLock2.Unlock()
	assert.Nil(t, fileLock1.db)

	// unlock can be called multiple times and it is safe
	fileLock2.Unlock()
	assert.Nil(t, fileLock1.db)

	// cleanup
	assert.NoError(t, os.RemoveAll(fileLockPath))
}

func TestCreateDBInEmptyDir(t *testing.T) {
	assert.NoError(t, os.RemoveAll(testDBPath), "")
	assert.NoError(t, os.MkdirAll(testDBPath, 0775), "")
	db := CreateDB(&Conf{testDBPath})
	defer db.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Panic is not expected when opening db in an existing empty dir. %s", r)
		}
	}()
	db.Open()
}

func TestCreateDBInNonEmptyDir(t *testing.T) {
	assert.NoError(t, os.RemoveAll(testDBPath), "")
	assert.NoError(t, os.MkdirAll(testDBPath, 0775), "")
	file, err := os.Create(filepath.Join(testDBPath, "dummyfile.txt"))
	assert.NoError(t, err, "")
	file.Close()
	db := CreateDB(&Conf{testDBPath})
	defer db.Close()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("A panic is expected when opening db in an existing non-empty dir. %s", r)
		}
	}()
	db.Open()
}
