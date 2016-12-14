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

package stateleveldb

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

var testDBPath = "/tmp/fabric/core/ledger/versioneddb/levelimpl"

func TestBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t, testDBPath)
	defer env.Cleanup()
	commontests.TestBasicRW(t, env.DB)
}

func TestDeletes(t *testing.T) {
	env := NewTestVDBEnv(t, testDBPath)
	defer env.Cleanup()
	commontests.TestDeletes(t, env.DB)
}

func TestIterator(t *testing.T) {
	env := NewTestVDBEnv(t, testDBPath)
	defer env.Cleanup()
	commontests.TestIterator(t, env.DB)
}

func TestEncodeDecodeValueAndVersion(t *testing.T) {
	testValueAndVersionEncodeing(t, []byte("value1"), version.NewHeight(1, 2))
	testValueAndVersionEncodeing(t, []byte{}, version.NewHeight(50, 50))
}

func testValueAndVersionEncodeing(t *testing.T, value []byte, version *version.Height) {
	encodedValue := encodeValue(value, version)
	val, ver := decodeValue(encodedValue)
	testutil.AssertEquals(t, val, value)
	testutil.AssertEquals(t, ver, version)
}

func TestCompositeKey(t *testing.T) {
	testCompositeKey(t, "ns", "key")
	testCompositeKey(t, "ns", "")
}

func testCompositeKey(t *testing.T, ns string, key string) {
	compositeKey := constructCompositeKey(ns, key)
	t.Logf("compositeKey=%#v", compositeKey)
	ns1, key1 := splitCompositeKey(compositeKey)
	testutil.AssertEquals(t, ns1, ns)
	testutil.AssertEquals(t, key1, key)
}
