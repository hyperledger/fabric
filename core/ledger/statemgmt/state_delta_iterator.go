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

package statemgmt

// StateDeltaIterator - An iterator implementation over state-delta
type StateDeltaIterator struct {
	updates         map[string]*UpdatedValue
	relevantKeys    []string
	currentKeyIndex int
	done            bool
}

// NewStateDeltaRangeScanIterator - return an iterator for performing a range scan over a state-delta object
func NewStateDeltaRangeScanIterator(delta *StateDelta, chaincodeID string, startKey string, endKey string) *StateDeltaIterator {
	updates := delta.GetUpdates(chaincodeID)
	return &StateDeltaIterator{updates, retrieveRelevantKeys(updates, startKey, endKey), -1, false}
}

func retrieveRelevantKeys(updates map[string]*UpdatedValue, startKey string, endKey string) []string {
	relevantKeys := []string{}
	if updates == nil {
		return relevantKeys
	}
	for k, v := range updates {
		if k >= startKey && (endKey == "" || k <= endKey) && !v.IsDelete() {
			relevantKeys = append(relevantKeys, k)
		}
	}
	return relevantKeys
}

// Next - see interface 'RangeScanIterator' for details
func (itr *StateDeltaIterator) Next() bool {
	itr.currentKeyIndex++
	if itr.currentKeyIndex < len(itr.relevantKeys) {
		return true
	}
	itr.currentKeyIndex--
	itr.done = true
	return false
}

// GetKeyValue - see interface 'RangeScanIterator' for details
func (itr *StateDeltaIterator) GetKeyValue() (string, []byte) {
	if itr.done {
		logger.Warning("Iterator used after it has been exhausted. Last retrieved value will be returned")
	}
	key := itr.relevantKeys[itr.currentKeyIndex]
	value := itr.updates[key].GetValue()
	return key, value
}

// Close - see interface 'RangeScanIterator' for details
func (itr *StateDeltaIterator) Close() {
}

// ContainsKey - checks wether the given key is present in the state-delta
func (itr *StateDeltaIterator) ContainsKey(key string) bool {
	_, ok := itr.updates[key]
	return ok
}
