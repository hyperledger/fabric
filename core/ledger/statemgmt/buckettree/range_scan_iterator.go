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

package buckettree

import (
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/tecbot/gorocksdb"
)

// RangeScanIterator implements the interface 'statemgmt.RangeScanIterator'
type RangeScanIterator struct {
	dbItr               *gorocksdb.Iterator
	chaincodeID         string
	startKey            string
	endKey              string
	currentBucketNumber int
	currentKey          string
	currentValue        []byte
	done                bool
}

func newRangeScanIterator(chaincodeID string, startKey string, endKey string) (*RangeScanIterator, error) {
	dbItr := db.GetDBHandle().GetStateCFIterator()
	itr := &RangeScanIterator{
		dbItr:       dbItr,
		chaincodeID: chaincodeID,
		startKey:    startKey,
		endKey:      endKey,
	}
	itr.seekForStartKeyWithinBucket(1)
	return itr, nil
}

// Next - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) Next() bool {
	if itr.done {
		return false
	}

	for itr.dbItr.Valid() {

		// making a copy of key-value bytes because, underlying key bytes are reused by itr.
		// no need to free slices as iterator frees memory when closed.
		keyBytes := statemgmt.Copy(itr.dbItr.Key().Data())
		valueBytes := statemgmt.Copy(itr.dbItr.Value().Data())

		dataNode := unmarshalDataNodeFromBytes(keyBytes, valueBytes)
		dataKey := dataNode.dataKey
		chaincodeID, key := statemgmt.DecodeCompositeKey(dataNode.getCompositeKey())
		value := dataNode.value
		logger.Debugf("Evaluating data-key = %s", dataKey)

		bucketNumber := dataKey.bucketKey.bucketNumber
		if bucketNumber > itr.currentBucketNumber {
			itr.seekForStartKeyWithinBucket(bucketNumber)
			continue
		}

		if chaincodeID == itr.chaincodeID && (itr.endKey == "" || key <= itr.endKey) {
			logger.Debugf("including data-key = %s", dataKey)
			itr.currentKey = key
			itr.currentValue = value
			itr.dbItr.Next()
			return true
		}

		itr.seekForStartKeyWithinBucket(bucketNumber + 1)
		continue
	}
	itr.done = true
	return false
}

func (itr *RangeScanIterator) seekForStartKeyWithinBucket(bucketNumber int) {
	itr.currentBucketNumber = bucketNumber
	datakeyBytes := minimumPossibleDataKeyBytes(bucketNumber, itr.chaincodeID, itr.startKey)
	itr.dbItr.Seek(datakeyBytes)
}

// GetKeyValue - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) GetKeyValue() (string, []byte) {
	return itr.currentKey, itr.currentValue
}

// Close - see interface 'statemgmt.RangeScanIterator' for details
func (itr *RangeScanIterator) Close() {
	itr.dbItr.Close()
}
