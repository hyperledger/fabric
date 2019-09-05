/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
)

type TransactionContext struct {
	ChannelID            string
	NamespaceID          string
	SignedProp           *pb.SignedProposal
	Proposal             *pb.Proposal
	ResponseNotifier     chan *pb.ChaincodeMessage
	TXSimulator          ledger.TxSimulator
	HistoryQueryExecutor ledger.HistoryQueryExecutor
	CollectionStore      privdata.CollectionStore
	IsInitTransaction    bool

	// tracks open iterators used for range queries
	queryMutex          sync.Mutex
	queryIteratorMap    map[string]commonledger.ResultsIterator
	pendingQueryResults map[string]*PendingQueryResult
	totalReturnCount    map[string]*int32

	// cache used to save the result of collection acl
	// as a transactionContext is created for every chaincode
	// invoke (even in case of chaincode-calling-chaincode,
	// we do not need to store the namespace in the map and
	// collection alone is sufficient.
	CollectionACLCache CollectionACLCache
}

// CollectionACLCache encapsulates a cache that stores read
// write permission on a collection
type CollectionACLCache map[string]*readWritePermission

type readWritePermission struct {
	read, write bool
}

func (c CollectionACLCache) put(collection string, rwPermission *readWritePermission) {
	c[collection] = rwPermission
}

func (c CollectionACLCache) get(collection string) *readWritePermission {
	return c[collection]
}

func (t *TransactionContext) InitializeCollectionACLCache() {
	t.CollectionACLCache = make(CollectionACLCache)
}

func (t *TransactionContext) InitializeQueryContext(queryID string, iter commonledger.ResultsIterator) {
	t.queryMutex.Lock()
	if t.queryIteratorMap == nil {
		t.queryIteratorMap = map[string]commonledger.ResultsIterator{}
	}
	if t.pendingQueryResults == nil {
		t.pendingQueryResults = map[string]*PendingQueryResult{}
	}
	if t.totalReturnCount == nil {
		t.totalReturnCount = map[string]*int32{}
	}
	t.queryIteratorMap[queryID] = iter
	t.pendingQueryResults[queryID] = &PendingQueryResult{}
	zeroValue := int32(0)
	t.totalReturnCount[queryID] = &zeroValue
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

func (t *TransactionContext) GetTotalReturnCount(queryID string) *int32 {
	t.queryMutex.Lock()
	result := t.totalReturnCount[queryID]
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
	delete(t.totalReturnCount, queryID)
}

func (t *TransactionContext) CleanupQueryContextWithBookmark(queryID string) string {
	t.queryMutex.Lock()
	defer t.queryMutex.Unlock()
	iter := t.queryIteratorMap[queryID]
	bookmark := ""
	if iter != nil {
		if queryResultIterator, ok := iter.(commonledger.QueryResultsIterator); ok {
			bookmark = queryResultIterator.GetBookmarkAndClose()
		}
	}
	delete(t.queryIteratorMap, queryID)
	delete(t.pendingQueryResults, queryID)
	delete(t.totalReturnCount, queryID)
	return bookmark
}

func (t *TransactionContext) CloseQueryIterators() {
	t.queryMutex.Lock()
	defer t.queryMutex.Unlock()
	for _, iter := range t.queryIteratorMap {
		iter.Close()
	}
}
