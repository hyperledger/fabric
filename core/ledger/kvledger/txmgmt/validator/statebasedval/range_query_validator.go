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

package statebasedval

import (
	"bytes"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

type rangeQueryValidator interface {
	init(rqInfo *kvrwset.RangeQueryInfo, itr statedb.ResultsIterator) error
	validate() (bool, error)
}

type rangeQueryResultsValidator struct {
	rqInfo *kvrwset.RangeQueryInfo
	itr    statedb.ResultsIterator
}

func (v *rangeQueryResultsValidator) init(rqInfo *kvrwset.RangeQueryInfo, itr statedb.ResultsIterator) error {
	v.rqInfo = rqInfo
	v.itr = itr
	return nil
}

// validate iterates through the results that are present in the range-query-info (captured during simulation)
// and the iterator (latest view of the `committed state` i.e., db + updates). At first mismatch between the results
// from the two sources (the range-query-info and the iterator), the validate returns false.
func (v *rangeQueryResultsValidator) validate() (bool, error) {
	rqResults := v.rqInfo.GetRawReads().GetKvReads()
	itr := v.itr
	var result statedb.QueryResult
	var err error
	if result, err = itr.Next(); err != nil {
		return false, err
	}
	if len(rqResults) == 0 {
		return result == nil, nil
	}
	for i := 0; i < len(rqResults); i++ {
		kvRead := rqResults[i]
		logger.Debugf("comparing kvRead=[%#v] to queryResponse=[%#v]", kvRead, result)
		if result == nil {
			logger.Debugf("Query response nil. Key [%s] got deleted", kvRead.Key)
			return false, nil
		}
		versionedKV := result.(*statedb.VersionedKV)
		if versionedKV.Key != kvRead.Key {
			logger.Debugf("key name mismatch: Key in rwset = [%s], key in query results = [%s]", kvRead.Key, versionedKV.Key)
			return false, nil
		}
		if !version.AreSame(versionedKV.Version, convertToVersionHeight(kvRead.Version)) {
			logger.Debugf(`Version mismatch for key [%s]: Version in rwset = [%#v], latest version = [%#v]`,
				versionedKV.Key, versionedKV.Version, kvRead.Version)
			return false, nil
		}
		if result, err = itr.Next(); err != nil {
			return false, err
		}
	}
	if result != nil {
		// iterator is not exhausted - which means that there are extra results in the given range
		logger.Debugf("Extra result = [%#v]", result)
		return false, nil
	}
	return true, nil
}

type rangeQueryHashValidator struct {
	rqInfo        *kvrwset.RangeQueryInfo
	itr           statedb.ResultsIterator
	resultsHelper *rwsetutil.RangeQueryResultsHelper
}

func (v *rangeQueryHashValidator) init(rqInfo *kvrwset.RangeQueryInfo, itr statedb.ResultsIterator) error {
	v.rqInfo = rqInfo
	v.itr = itr
	var err error
	v.resultsHelper, err = rwsetutil.NewRangeQueryResultsHelper(true, rqInfo.GetReadsMerkleHashes().MaxDegree)
	return err
}

// validate iterates through the iterator (latest view of the `committed state` i.e., db + updates)
// and starts building the merkle tree incrementally (Both, during simulation and during validation,
// rwset.RangeQueryResultsHelper is used for building the merkle tree incrementally ).
//
// This function also keeps comparing the under-construction merkle tree with the merkle tree
// summary present in the range-query-info (built during simulation).
// This function returns false on first mismatch between the nodes of the two merkle trees
// at the desired level (the maxLevel of the merkle tree in range-query-info).
func (v *rangeQueryHashValidator) validate() (bool, error) {
	itr := v.itr
	lastMatchedIndex := -1
	inMerkle := v.rqInfo.GetReadsMerkleHashes()
	var merkle *kvrwset.QueryReadsMerkleSummary
	logger.Debugf("inMerkle: %#v", inMerkle)
	for {
		var result statedb.QueryResult
		var err error
		if result, err = itr.Next(); err != nil {
			return false, err
		}
		logger.Debugf("Processing result = %#v", result)
		if result == nil {
			if _, merkle, err = v.resultsHelper.Done(); err != nil {
				return false, err
			}
			equals := inMerkle.Equal(merkle)
			logger.Debugf("Combined iterator exhausted. merkle=%#v, equals=%t", merkle, equals)
			return equals, nil
		}
		versionedKV := result.(*statedb.VersionedKV)
		v.resultsHelper.AddResult(rwsetutil.NewKVRead(versionedKV.Key, versionedKV.Version))
		merkle := v.resultsHelper.GetMerkleSummary()

		if merkle.MaxLevel < inMerkle.MaxLevel {
			logger.Debugf("Hashes still under construction. Noting to compare yet. Need more results. Continuing...")
			continue
		}
		if lastMatchedIndex == len(merkle.MaxLevelHashes)-1 {
			logger.Debugf("Need more results to build next entry [index=%d] at level [%d]. Continuing...",
				lastMatchedIndex+1, merkle.MaxLevel)
			continue
		}
		if len(merkle.MaxLevelHashes) > len(inMerkle.MaxLevelHashes) {
			logger.Debugf("Entries exceeded from what are present in the incoming merkleSummary. Validation failed")
			return false, nil
		}
		lastMatchedIndex++
		if !bytes.Equal(merkle.MaxLevelHashes[lastMatchedIndex], inMerkle.MaxLevelHashes[lastMatchedIndex]) {
			logger.Debugf("Hashes does not match at index [%d]. Validation failed", lastMatchedIndex)
			return false, nil
		}
	}
}

func convertToVersionHeight(v *kvrwset.Version) *version.Height {
	return version.NewHeight(v.BlockNum, v.TxNum)
}
