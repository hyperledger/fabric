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

package statedb

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

// TestEncodeString tests encoding and decoding a string value
func TestEncodeDecodeString(t *testing.T) {

	bytesString1 := []byte("value1")
	version1 := version.NewHeight(1, 1)

	encodedValue := EncodeValue(bytesString1, version1)
	decodedValue, decodedVersion := DecodeValue(encodedValue)

	testutil.AssertEquals(t, decodedValue, bytesString1)

	testutil.AssertEquals(t, decodedVersion, version1)

}

// TestEncodeDecodeJSON tests encoding and decoding a JSON value
func TestEncodeDecodeJSON(t *testing.T) {

	bytesJSON2 := []byte(`{"asset_name":"marble1","color":"blue","size":"35","owner":"jerry"}`)
	version2 := version.NewHeight(1, 1)

	encodedValue := EncodeValue(bytesJSON2, version2)
	decodedValue, decodedVersion := DecodeValue(encodedValue)

	testutil.AssertEquals(t, decodedValue, bytesJSON2)

	testutil.AssertEquals(t, decodedVersion, version2)

}
