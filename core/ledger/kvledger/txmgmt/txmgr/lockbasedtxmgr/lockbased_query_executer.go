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
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
)

// LockBasedQueryExecutor is a query executor used in `LockBasedTxMgr`
type lockBasedQueryExecutor struct {
	helper *queryHelper
	id     string
}

func newQueryExecutor(txmgr *LockBasedTxMgr) *lockBasedQueryExecutor {
	helper := &queryHelper{txmgr: txmgr, rwsetBuilder: nil}
	id := util.GenerateUUID()
	logger.Debugf("constructing new query executor [%s]", id)
	return &lockBasedQueryExecutor{helper, id}
}

// GetState implements method in interface `ledger.QueryExecutor`
func (q *lockBasedQueryExecutor) GetState(ns string, key string) ([]byte, error) {
	return q.helper.getState(ns, key)
}

// GetStateMultipleKeys implements method in interface `ledger.QueryExecutor`
func (q *lockBasedQueryExecutor) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	return q.helper.getStateMultipleKeys(namespace, keys)
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
// can be supplied as empty strings. However, a full scan shuold be used judiciously for performance reasons.
func (q *lockBasedQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	return q.helper.getStateRangeScanIterator(namespace, startKey, endKey)
}

// ExecuteQuery implements method in interface `ledger.QueryExecutor`
func (q *lockBasedQueryExecutor) ExecuteQuery(namespace, query string) (ledger.ResultsIterator, error) {
	return q.helper.executeQuery(namespace, query)
}

// Done implements method in interface `ledger.QueryExecutor`
func (q *lockBasedQueryExecutor) Done() {
	logger.Debugf("Done with transaction simulation / query execution [%s]", q.id)
	q.helper.done()
}
