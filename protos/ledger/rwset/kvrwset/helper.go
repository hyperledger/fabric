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

package kvrwset

import (
	"bytes"
)

// SetRawReads sets the 'readsInfo' field to raw KVReads performed by the query
func (rqi *RangeQueryInfo) SetRawReads(kvReads []*KVRead) {
	rqi.ReadsInfo = &RangeQueryInfo_RawReads{&QueryReads{kvReads}}
}

// SetMerkelSummary sets the 'readsInfo' field to merkle summary of the raw KVReads of query results
func (rqi *RangeQueryInfo) SetMerkelSummary(merkleSummary *QueryReadsMerkleSummary) {
	rqi.ReadsInfo = &RangeQueryInfo_ReadsMerkleHashes{merkleSummary}
}

// Equal verifies whether the give MerkleSummary is equals to this
func (ms *QueryReadsMerkleSummary) Equal(anotherMS *QueryReadsMerkleSummary) bool {
	if anotherMS == nil {
		return false
	}
	if ms.MaxDegree != anotherMS.MaxDegree ||
		ms.MaxLevel != anotherMS.MaxLevel ||
		len(ms.MaxLevelHashes) != len(anotherMS.MaxLevelHashes) {
		return false
	}
	for i := 0; i < len(ms.MaxLevelHashes); i++ {
		if !bytes.Equal(ms.MaxLevelHashes[i], anotherMS.MaxLevelHashes[i]) {
			return false
		}
	}
	return true
}
