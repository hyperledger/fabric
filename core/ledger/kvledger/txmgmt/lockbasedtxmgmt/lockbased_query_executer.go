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

package lockbasedtxmgmt

import (
	"errors"

	"github.com/hyperledger/fabric/core/ledger"
)

// RWLockQueryExecutor is a query executor used in `LockBasedTxMgr`
type RWLockQueryExecutor struct {
	txmgr *LockBasedTxMgr
	done  bool
}

// GetState implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetState(ns string, key string) ([]byte, error) {
	q.checkDone()
	var value []byte
	var err error
	if value, _, err = q.txmgr.getCommittedValueAndVersion(ns, key); err != nil {
		return nil, err
	}
	return value, nil
}

// GetStateMultipleKeys implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	var results [][]byte
	var value []byte
	var err error
	for _, key := range keys {
		if value, err = q.GetState(namespace, key); err != nil {
			return nil, err
		}
		results = append(results, value)
	}
	return results, nil
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
// can be supplied as empty strings. However, a full scan shuold be used judiciously for performance reasons.
func (q *RWLockQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	q.checkDone()
	scanner, err := q.txmgr.getCommittedRangeScanner(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &qKVItr{scanner}, nil
}

// GetTransactionsForKey - implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetTransactionsForKey(namespace string, key string) (ledger.ResultsIterator, error) {
	return nil, errors.New("Not yet implemented")
}

// ExecuteQuery implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) ExecuteQuery(query string) (ledger.ResultsIterator, error) {
	return nil, errors.New("Not supported by KV data model")
}

// Done implements method in interface `ledger.TxSimulator`
func (q *RWLockQueryExecutor) Done() {
	q.done = true
	q.txmgr.commitRWLock.RUnlock()
}

func (q *RWLockQueryExecutor) checkDone() {
	if q.done {
		panic("This method should not be called after calling Done()")
	}
}

type qKVItr struct {
	s *kvScanner
}

// Next implements Next() method in ledger.ResultsIterator
func (itr *qKVItr) Next() (ledger.QueryResult, error) {
	committedKV, err := itr.s.next()
	if err != nil {
		return nil, err
	}
	if committedKV == nil {
		return nil, nil
	}
	if committedKV.isDelete() {
		return itr.Next()
	}
	return &ledger.KV{Key: committedKV.key, Value: committedKV.value}, nil
}

// Close implements Close() method in ledger.ResultsIterator
func (itr *qKVItr) Close() {
	itr.s.close()
}
