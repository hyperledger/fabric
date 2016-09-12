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
	"testing"

	"github.com/hyperledger/fabric/core/ledgernext/testutil"
)

func TestDBBasicWriteAndReads(t *testing.T) {
	dbConf := &Conf{"/tmp/v2/test/db", []string{"cf1", "cf2"}, false}
	db := CreateDB(dbConf)
	db.Open()
	defer db.Close()
	db.Put(db.GetCFHandle("cf1"), []byte("key1"), []byte("value1"))
	db.Put(db.GetCFHandle("cf2"), []byte("key2"), []byte("value2"))
	db.Put(db.GetDefaultCFHandle(), []byte("key3"), []byte("value3"))
	val, err := db.Get(db.GetCFHandle("cf1"), []byte("key1"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value1"))

	val, err = db.Get(db.GetCFHandle("cf2"), []byte("key2"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value2"))

	val, err = db.Get(db.GetDefaultCFHandle(), []byte("key3"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value3"))
}
