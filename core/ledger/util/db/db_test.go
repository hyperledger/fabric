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

package db

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestDBBasicWriteAndReads(t *testing.T) {
	testDBPath := "/tmp/test/hyperledger/fabric/core/ledger/util/db"
	if err := os.RemoveAll(testDBPath); err != nil {
		t.Fatalf("Error:%s", err)
	}
	dbConf := &Conf{testDBPath}
	defer func() { os.RemoveAll(testDBPath) }()
	db := CreateDB(dbConf)
	db.Open()
	defer db.Close()
	db.Put([]byte("key1"), []byte("value1"), false)
	db.Put([]byte("key2"), []byte("value2"), false)
	db.Put([]byte("key3"), []byte("value3"), false)
	val, err := db.Get([]byte("key1"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value1"))

	val, err = db.Get([]byte("key2"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value2"))

	val, err = db.Get([]byte("key3"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value3"))
}
