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

package state

import (
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
)

// CompositeRangeScanIterator - an implementation of interface 'statemgmt.RangeScanIterator'
// This provides a wrapper on top of more than one underlying iterators
type CompositeRangeScanIterator struct {
	itrs             []statemgmt.RangeScanIterator
	currentItrNumber int
}

func newCompositeRangeScanIterator(
	txDeltaItr *statemgmt.StateDeltaIterator,
	batchDeltaItr *statemgmt.StateDeltaIterator,
	implItr statemgmt.RangeScanIterator) statemgmt.RangeScanIterator {
	itrs := make([]statemgmt.RangeScanIterator, 3)
	itrs[0] = txDeltaItr
	itrs[1] = batchDeltaItr
	itrs[2] = implItr
	return &CompositeRangeScanIterator{itrs, 0}
}

// Next - see interface 'statemgmt.RangeScanIterator' for details
// The specific implementation below starts from first underlying iterator and
// after exhausting the first underlying iterator, move to the second underlying iterator.
// The implementation repeats this until last underlying iterator has been exhausted
// In addition, the key-value from an underlying iterator are skipped if the key is found
// in any of the preceding iterators
func (itr *CompositeRangeScanIterator) Next() bool {
	currentItrNumber := itr.currentItrNumber
	currentItr := itr.itrs[currentItrNumber]
	logger.Debugf("Operating on iterator number = %d", currentItrNumber)
	keyAvailable := currentItr.Next()
	for keyAvailable {
		key, _ := currentItr.GetKeyValue()
		logger.Debugf("Retrieved key = %s", key)
		skipKey := false
		for i := currentItrNumber - 1; i >= 0; i-- {
			logger.Debugf("Evaluating key = %s in itr number = %d. currentItrNumber = %d", key, i, currentItrNumber)
			previousItr := itr.itrs[i]
			if previousItr.(*statemgmt.StateDeltaIterator).ContainsKey(key) {
				skipKey = true
				break
			}
		}
		if skipKey {
			logger.Debugf("Skipping key = %s", key)
			keyAvailable = currentItr.Next()
			continue
		}
		break
	}

	if keyAvailable || currentItrNumber == 2 {
		logger.Debug("Returning for current key")
		return keyAvailable
	}

	logger.Debug("Moving to next iterator")
	itr.currentItrNumber++
	return itr.Next()
}

// GetKeyValue - see interface 'statemgmt.RangeScanIterator' for details
func (itr *CompositeRangeScanIterator) GetKeyValue() (string, []byte) {
	return itr.itrs[itr.currentItrNumber].GetKeyValue()
}

// Close - see interface 'statemgmt.RangeScanIterator' for details
func (itr *CompositeRangeScanIterator) Close() {
	itr.itrs[2].Close()
}
