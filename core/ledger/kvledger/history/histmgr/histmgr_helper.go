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

package histmgr

import (
	"bytes"
	"strconv"
)

var compositeKeySep = []byte{0x00}

//ConstructCompositeKey builds the History Key of:  "namespace key blocknum trannum"", with namespace being the chaincode id
func ConstructCompositeKey(ns string, key string, blocknum uint64, trannum uint64) string {
	// TODO - We will likely want sortable varint encoding, rather then a simple number, in order to support sorted key scans
	var buffer bytes.Buffer
	buffer.WriteString(ns)
	buffer.WriteByte(0)
	buffer.WriteString(key)
	buffer.WriteByte(0)
	buffer.WriteString(strconv.Itoa(int(blocknum)))
	buffer.WriteByte(0)
	buffer.WriteString(strconv.Itoa(int(trannum)))

	return buffer.String()
}

//ConstructPartialCompositeKey builds a partial History Key to be used for query
func ConstructPartialCompositeKey(ns string, key string, endkey bool) []byte {
	compositeKey := []byte(ns)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, []byte(key)...)
	if endkey {
		compositeKey = append(compositeKey, []byte("1")...)
	}
	return compositeKey
}

//SplitCompositeKey splits the key to build the query results
func SplitCompositeKey(compositePartialKey []byte, compositeKey []byte) (string, string) {
	split := bytes.SplitN(compositeKey, compositePartialKey, 2)
	return string(split[0]), string(split[1])
}
