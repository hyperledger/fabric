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

package history

import "github.com/hyperledger/fabric/core/ledger"

// CouchDBHistQueryExecutor is a query executor used in `CouchDBHistMgr`
type CouchDBHistQueryExecutor struct {
	histmgr *CouchDBHistMgr
}

// GetTransactionsForKey implements method in interface `ledger.HistoryQueryExecutor`
func (q *CouchDBHistQueryExecutor) GetTransactionsForKey(namespace string, key string, includeValues bool, includeTransactions bool) (ledger.ResultsIterator, error) {
	//TODO  includeTransactions has not been implemented yet.
	scanner, err := q.histmgr.getTransactionsForNsKey(namespace, key, includeValues)
	if err != nil {
		return nil, err
	}
	return &qHistoryItr{scanner}, nil
}

type qHistoryItr struct {
	q *histScanner
}

// Next implements Next() method in ledger.ResultsIterator
func (itr *qHistoryItr) Next() (ledger.QueryResult, error) {
	historicValue, err := itr.q.next()
	if err != nil {
		return nil, err
	}
	if historicValue == nil {
		return nil, nil
	}
	//TODO Returning blockNumTrannum as TxID for now but eventually will return txID instead
	return &ledger.KeyModification{TxID: historicValue.blockNumTranNum, Value: historicValue.value}, nil
}

// Close implements Close() method in ledger.ResultsIterator
func (itr *qHistoryItr) Close() {
	itr.q.close()
}
