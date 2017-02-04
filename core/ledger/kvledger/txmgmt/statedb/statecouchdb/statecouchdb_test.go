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

package statecouchdb

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {

	//call a helper method to load the core.yaml, will be used to detect if CouchDB is enabled
	ledgertestutil.SetupCoreYAMLConfig("./../../../../../../peer")

	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "127.0.0.1:5984")
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/statedb/statecouchdb")

	os.Exit(m.Run())
}

func TestBasicRW(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("TestDB")
		defer env.Cleanup("TestDB")
		commontests.TestBasicRW(t, env.DBProvider)

	}
}

func TestMultiDBBasicRW(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("TestDB1")
		env.Cleanup("TestDB2")
		defer env.Cleanup("TestDB1")
		defer env.Cleanup("TestDB2")
		commontests.TestMultiDBBasicRW(t, env.DBProvider)

	}
}

/* TODO add delete support in couchdb and then convert key value of nil to a couch delete. This will resolve TestDeletes
func TestDeletes(t *testing.T) {
	env := NewTestVDBEnv(t)
	env.Cleanup("TestDB")
	defer env.Cleanup("TestDB")
	commontests.TestDeletes(t, env.DBProvider)
}
*/

func TestIterator(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("TestDB")
		defer env.Cleanup("TestDB")
		commontests.TestIterator(t, env.DBProvider)

	}
}

func TestEncodeDecodeValueAndVersion(t *testing.T) {
	testValueAndVersionEncoding(t, []byte("value1"), version.NewHeight(1, 2))
	testValueAndVersionEncoding(t, []byte{}, version.NewHeight(50, 50))
}

func testValueAndVersionEncoding(t *testing.T, value []byte, version *version.Height) {
	encodedValue := statedb.EncodeValue(value, version)
	val, ver := statedb.DecodeValue(encodedValue)
	testutil.AssertEquals(t, val, value)
	testutil.AssertEquals(t, ver, version)
}

func TestCompositeKey(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		testCompositeKey(t, "ns", "key")
		testCompositeKey(t, "ns", "")

	}
}

func testCompositeKey(t *testing.T, ns string, key string) {
	compositeKey := constructCompositeKey(ns, key)
	t.Logf("compositeKey=%#v", compositeKey)
	ns1, key1 := splitCompositeKey(compositeKey)
	testutil.AssertEquals(t, ns1, ns)
	testutil.AssertEquals(t, key1, key)
}

// The following tests are unique to couchdb, they are not used in leveldb
//  query test
func TestQuery(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("TestDB")
		defer env.Cleanup("TestDB")
		commontests.TestQuery(t, env.DBProvider)

	}
}
