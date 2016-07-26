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

package trie

import (
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/tecbot/gorocksdb"
)

// RangeScanIterator implements the interface 'statemgmt.RangeScanIterator'
type RangeScanIterator struct {
	dbItr        *gorocksdb.Iterator
	chaincodeID  string
	endKey       string
	currentKey   string
	currentValue []byte
	done         bool
}

func newRangeScanIterator(chaincodeID string, startKey string, endKey string) (*RangeScanIterator, error) {
	dbItr := db.GetDBHandle().GetStateCFIterator()
	encodedStartKey := newTrieKey(chaincodeID, startKey).getEncodedBytes()
	dbItr.Seek(encodedStartKey)
	return &RangeScanIterator{dbItr, chaincodeID, endKey, "", nil, false}, nil
}

// Next - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) Next() bool {
	if itr.done {
		return false
	}
	for ; itr.dbItr.Valid(); itr.dbItr.Next() {

		// making a copy of key-value bytes because, underlying key bytes are reused by itr.
		// no need to free slices as iterator frees memory when closed.
		trieKeyBytes := statemgmt.Copy(itr.dbItr.Key().Data())
		trieNodeBytes := statemgmt.Copy(itr.dbItr.Value().Data())
		value := unmarshalTrieNodeValue(trieNodeBytes)
		if value == nil {
			continue
		}

		// found an actual key
		currentCompositeKey := trieKeyEncoderImpl.decodeTrieKeyBytes(statemgmt.Copy(trieKeyBytes))
		currentChaincodeID, currentKey := statemgmt.DecodeCompositeKey(currentCompositeKey)
		if currentChaincodeID == itr.chaincodeID && (itr.endKey == "" || currentKey <= itr.endKey) {
			itr.currentKey = currentKey
			itr.currentValue = value
			itr.dbItr.Next()
			return true
		}

		// retrieved all the keys in the given range
		break
	}
	itr.done = true
	return false
}

// GetKeyValue - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) GetKeyValue() (string, []byte) {
	return itr.currentKey, itr.currentValue
}

// Close - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) Close() {
	itr.dbItr.Close()
}
