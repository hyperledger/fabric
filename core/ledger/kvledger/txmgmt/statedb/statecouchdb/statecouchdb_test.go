/*
Copyright IBM Corp. 2016, 2017 All Rights Reserved.

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
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {

	// Read the core.yaml file for default config.
	ledgertestutil.SetupCoreYAMLConfig()
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/statedb/statecouchdb")

	// Switch to CouchDB
	viper.Set("ledger.state.stateDatabase", "CouchDB")

	// both vagrant and CI have couchdb configured at host "couchdb"
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "couchdb:5984")
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 10)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)

	//run the actual test
	result := m.Run()

	//revert to default goleveldb
	viper.Set("ledger.state.stateDatabase", "goleveldb")
	os.Exit(result)
}

func TestBasicRW(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("testbasicrw")
		defer env.Cleanup("testbasicrw")
		commontests.TestBasicRW(t, env.DBProvider)

	}
}

func TestMultiDBBasicRW(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("testmultidbbasicrw")
		env.Cleanup("testmultidbbasicrw2")
		defer env.Cleanup("testmultidbbasicrw")
		defer env.Cleanup("testmultidbbasicrw2")
		commontests.TestMultiDBBasicRW(t, env.DBProvider)

	}
}

func TestDeletes(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {
		env := NewTestVDBEnv(t)
		env.Cleanup("testdeletes")
		defer env.Cleanup("testdeletes")
		commontests.TestDeletes(t, env.DBProvider)
	}
}

func TestIterator(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := NewTestVDBEnv(t)
		env.Cleanup("testiterator")
		defer env.Cleanup("testiterator")
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
		env.Cleanup("testquery")
		defer env.Cleanup("testquery")
		commontests.TestQuery(t, env.DBProvider)

	}
}

func TestGetStateMultipleKeys(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() == true {
		env := NewTestVDBEnv(t)
		env.Cleanup("testgetmultiplekeys")
		defer env.Cleanup("testgetmultiplekeys")
		commontests.TestGetStateMultipleKeys(t, env.DBProvider)
	}
}
