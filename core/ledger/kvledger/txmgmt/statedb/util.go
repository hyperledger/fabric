/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

import "github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"

//EncodeValue appends the value to the version, allows storage of version and value in binary form
func EncodeValue(value []byte, version *version.Height) []byte {
	encodedValue := version.ToBytes()
	if value != nil {
		encodedValue = append(encodedValue, value...)
	}
	return encodedValue
}

//DecodeValue separates the version and value from a binary value
func DecodeValue(encodedValue []byte) ([]byte, *version.Height) {
	version, n := version.NewHeightFromBytes(encodedValue)
	value := encodedValue[n:]
	return value, version
}
