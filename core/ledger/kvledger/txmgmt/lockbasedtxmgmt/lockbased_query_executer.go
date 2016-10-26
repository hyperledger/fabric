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
}

// GetState implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetState(ns string, key string) ([]byte, error) {
	var value []byte
	var err error
	if value, _, err = q.txmgr.getCommittedValueAndVersion(ns, key); err != nil {
		return nil, err
	}
	return value, nil
}

// GetStateMultipleKeys implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	return nil, errors.New("Not yet implemented")
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	return nil, errors.New("Not yet implemented")
}

// GetTransactionsForKey - implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) GetTransactionsForKey(namespace string, key string) (ledger.ResultsIterator, error) {
	return nil, errors.New("Not yet implemented")
}

// ExecuteQuery implements method in interface `ledger.QueryExecutor`
func (q *RWLockQueryExecutor) ExecuteQuery(query string) (ledger.ResultsIterator, error) {
	return nil, errors.New("Not supported by KV data model")
}
