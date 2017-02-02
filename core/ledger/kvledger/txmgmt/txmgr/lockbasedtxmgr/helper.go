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

package lockbasedtxmgr

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

type queryHelper struct {
	txmgr       *LockBasedTxMgr
	rwset       *rwset.RWSet
	itrs        []*resultsItr
	doneInvoked bool
}

func (h *queryHelper) getState(ns string, key string) ([]byte, error) {
	h.checkDone()
	versionedValue, err := h.txmgr.db.GetState(ns, key)
	if err != nil {
		return nil, err
	}
	val, ver := decomposeVersionedValue(versionedValue)
	if h.rwset != nil {
		h.rwset.AddToReadSet(ns, key, ver)
	}
	return val, nil
}

func (h *queryHelper) getStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	h.checkDone()
	versionedValues, err := h.txmgr.db.GetStateMultipleKeys(namespace, keys)
	if err != nil {
		return nil, nil
	}
	values := make([][]byte, len(versionedValues))
	for i, versionedValue := range versionedValues {
		val, ver := decomposeVersionedValue(versionedValue)
		if h.rwset != nil {
			h.rwset.AddToReadSet(namespace, keys[i], ver)
		}
		values[i] = val
	}
	return values, nil
}

func (h *queryHelper) getStateRangeScanIterator(namespace string, startKey string, endKey string) (commonledger.ResultsIterator, error) {
	h.checkDone()
	itr, err := newResultsItr(namespace, startKey, endKey, h.txmgr.db, h.rwset)
	if err != nil {
		return nil, err
	}
	h.itrs = append(h.itrs, itr)
	return itr, nil
}

func (h *queryHelper) executeQuery(query string) (commonledger.ResultsIterator, error) {
	dbItr, err := h.txmgr.db.ExecuteQuery(query)
	if err != nil {
		return nil, err
	}
	return &queryResultsItr{DBItr: dbItr, RWSet: h.rwset}, nil
}

func (h *queryHelper) done() {
	if h.doneInvoked {
		return
	}
	defer h.txmgr.commitRWLock.RUnlock()
	h.doneInvoked = true
	for _, itr := range h.itrs {
		itr.Close()
		if h.rwset != nil {
			h.rwset.AddToRangeQuerySet(itr.ns, itr.rangeQueryInfo)
		}
	}
}

func (h *queryHelper) checkDone() {
	if h.doneInvoked {
		panic("This instance should not be used after calling Done()")
	}
}

// resultsItr implements interface ledger.ResultsIterator
// this wraps the actual db iterator and intercept the calls
// to build rangeQueryInfo in the ReadWriteSet that is used
// for performing phantom read validation during commit
type resultsItr struct {
	ns             string
	endKey         string
	dbItr          statedb.ResultsIterator
	rwSet          *rwset.RWSet
	rangeQueryInfo *rwset.RangeQueryInfo
}

func newResultsItr(ns string, startKey string, endKey string, db statedb.VersionedDB, rwSet *rwset.RWSet) (*resultsItr, error) {
	dbItr, err := db.GetStateRangeScanIterator(ns, startKey, endKey)
	if err != nil {
		return nil, err
	}
	// In the range query info, just set the StartKey.
	// Set the EndKey later below in the Next() method.
	rqInfo := &rwset.RangeQueryInfo{StartKey: startKey}
	return &resultsItr{ns, endKey, dbItr, rwSet, rqInfo}, nil
}

// Next implements method in interface ledger.ResultsIterator
// Before returning the next result, update the EndKey and ItrExhausted in rangeQueryInfo
// If we set the EndKey in the constructor (as we do for the StartKey) to what is
// supplied in the original query, we may be capturing the unnecessary longer range if the
// caller decides to stop iterating at some intermidiate point. Alternatively, we could have
// set the EndKey and ItrExhausted in the Close() function but it may not be desirable to change
// transactional behaviour based on whether the Close() was invoked or not
func (itr *resultsItr) Next() (commonledger.QueryResult, error) {
	queryResult, err := itr.dbItr.Next()
	if err != nil {
		return nil, err
	}
	itr.updateRangeQueryInfo(queryResult)
	if queryResult == nil {
		return nil, nil
	}
	versionedKV := queryResult.(*statedb.VersionedKV)
	return &ledger.KV{Key: versionedKV.Key, Value: versionedKV.Value}, nil
}

// updateRangeQueryInfo updates two attributes of the rangeQueryInfo
// 1) The EndKey - set to either a) latest key that is to be returned to the caller (if the iterator is not exhausted)
//                                  because, we do not know if the caller is again going to invoke Next() or not.
//                            or b) the last key that was supplied in the original query (if the iterator is exhausted)
// 2) The ItrExhausted - set to true if the iterator is going to return nil as a result of the Next() call
func (itr *resultsItr) updateRangeQueryInfo(queryResult statedb.QueryResult) {
	if itr.rwSet == nil {
		return
	}

	if queryResult == nil {
		// caller scanned till the iterator got exhausted.
		// So, set the endKey to the actual endKey supplied in the query
		itr.rangeQueryInfo.ItrExhausted = true
		itr.rangeQueryInfo.EndKey = itr.endKey
		return
	}
	versionedKV := queryResult.(*statedb.VersionedKV)
	itr.rangeQueryInfo.AddResult(rwset.NewKVRead(versionedKV.Key, versionedKV.Version))
	// Set the end key to the latest key retrieved by the caller.
	// Because, the caller may actually not invoke the Next() function again
	itr.rangeQueryInfo.EndKey = versionedKV.Key
}

// Close implements method in interface ledger.ResultsIterator
func (itr *resultsItr) Close() {
	itr.dbItr.Close()
}

type queryResultsItr struct {
	DBItr statedb.ResultsIterator
	RWSet *rwset.RWSet
}

// Next implements method in interface ledger.ResultsIterator
func (itr *queryResultsItr) Next() (commonledger.QueryResult, error) {

	queryResult, err := itr.DBItr.Next()
	if err != nil {
		return nil, err
	}
	if queryResult == nil {
		return nil, nil
	}
	versionedQueryRecord := queryResult.(*statedb.VersionedQueryRecord)
	logger.Debugf("queryResultsItr.Next() returned a record:%s", string(versionedQueryRecord.Record))

	if itr.RWSet != nil {
		itr.RWSet.AddToReadSet(versionedQueryRecord.Namespace, versionedQueryRecord.Key, versionedQueryRecord.Version)
	}
	return &ledger.QueryRecord{Namespace: versionedQueryRecord.Namespace, Key: versionedQueryRecord.Key, Record: versionedQueryRecord.Record}, nil
}

// Close implements method in interface ledger.ResultsIterator
func (itr *queryResultsItr) Close() {
	itr.DBItr.Close()
}

func decomposeVersionedValue(versionedValue *statedb.VersionedValue) ([]byte, *version.Height) {
	var value []byte
	var ver *version.Height
	if versionedValue != nil {
		value = versionedValue.Value
		ver = versionedValue.Version
	}
	return value, ver
}
