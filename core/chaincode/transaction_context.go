/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type TransactionContext struct {
	ChainID              string
	SignedProp           *pb.SignedProposal
	Proposal             *pb.Proposal
	ResponseNotifier     chan *pb.ChaincodeMessage
	TXSimulator          ledger.TxSimulator
	HistoryQueryExecutor ledger.HistoryQueryExecutor

	// tracks open iterators used for range queries
	queryMutex          sync.Mutex
	queryIteratorMap    map[string]commonledger.ResultsIterator
	pendingQueryResults map[string]*PendingQueryResult
}

func (t *TransactionContext) InitializeQueryContext(queryID string, iter commonledger.ResultsIterator) {
	t.queryMutex.Lock()
	if t.queryIteratorMap == nil {
		t.queryIteratorMap = map[string]commonledger.ResultsIterator{}
	}
	if t.pendingQueryResults == nil {
		t.pendingQueryResults = map[string]*PendingQueryResult{}
	}
	t.queryIteratorMap[queryID] = iter
	t.pendingQueryResults[queryID] = &PendingQueryResult{}
	t.queryMutex.Unlock()
}

func (t *TransactionContext) GetQueryIterator(queryID string) commonledger.ResultsIterator {
	t.queryMutex.Lock()
	iter := t.queryIteratorMap[queryID]
	t.queryMutex.Unlock()
	return iter
}

func (t *TransactionContext) GetPendingQueryResult(queryID string) *PendingQueryResult {
	t.queryMutex.Lock()
	result := t.pendingQueryResults[queryID]
	t.queryMutex.Unlock()
	return result
}

func (t *TransactionContext) CleanupQueryContext(queryID string) {
	t.queryMutex.Lock()
	defer t.queryMutex.Unlock()
	iter := t.queryIteratorMap[queryID]
	if iter != nil {
		iter.Close()
	}
	delete(t.queryIteratorMap, queryID)
	delete(t.pendingQueryResults, queryID)
}

func (t *TransactionContext) CloseQueryIterators() {
	t.queryMutex.Lock()
	defer t.queryMutex.Unlock()
	for _, iter := range t.queryIteratorMap {
		iter.Close()
	}
}
