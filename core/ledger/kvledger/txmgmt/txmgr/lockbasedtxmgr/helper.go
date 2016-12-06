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
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

type queryHelper struct {
	txmgr       *LockBasedTxMgr
	rwset       *rwset.RWSet
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

func (h *queryHelper) getStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	h.checkDone()
	dbItr, err := h.txmgr.db.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &resultsItr{DBItr: dbItr, RWSet: h.rwset}, nil
}

func (h *queryHelper) executeQuery(query string) (ledger.ResultsIterator, error) {
	dbItr, err := h.txmgr.db.ExecuteQuery(query)
	if err != nil {
		return nil, err
	}
	return &resultsItr{DBItr: dbItr, RWSet: h.rwset}, nil
}

func (h *queryHelper) done() {
	h.doneInvoked = true
	h.txmgr.commitRWLock.RUnlock()
}

func (h *queryHelper) checkDone() {
	if h.doneInvoked {
		panic("This instance should not be used after calling Done()")
	}
}

type resultsItr struct {
	DBItr statedb.ResultsIterator
	RWSet *rwset.RWSet
}

// Next implements method in interface ledger.ResultsIterator
func (itr *resultsItr) Next() (ledger.QueryResult, error) {
	versionedKV, err := itr.DBItr.Next()
	if err != nil {
		return nil, err
	}
	if versionedKV == nil {
		return nil, nil
	}
	if itr.RWSet != nil {
		itr.RWSet.AddToReadSet(versionedKV.Namespace, versionedKV.Key, versionedKV.Version)
	}
	return &ledger.KV{Key: versionedKV.Key, Value: versionedKV.Value}, nil
}

// Close implements method in interface ledger.ResultsIterator
func (itr *resultsItr) Close() {
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
