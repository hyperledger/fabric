/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import "github.com/hyperledger/fabric/common/ledger"

//go:generate counterfeiter -o mock/ledger_reader.go -fake-name LedgerReader . LedgerReader
//go:generate counterfeiter -o mock/ledger_manager.go -fake-name LedgerManager . LedgerManager

// LedgerManager provides access to the ledger infrastructure
type LedgerManager interface {
	// Returns a LedgerReader for the passed channel, an error otherwise
	GetLedgerReader(channel string) (LedgerReader, error)
}

// LedgerReader interface, used to read from a ledger.
type LedgerReader interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)

	// GetStateRangeScanIterator returns an iterator that contains all the Key-values between given Key ranges.
	// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available Key
	// and an empty endKey refers to the last available Key. For scanning all the keys, both the startKey and the endKey
	// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
	// The returned ResultsIterator contains results of type *KV which is defined in protos/ledger/queryresult.
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error)

	// Done releases resources occupied by the LedgerReader
	Done()
}

//go:generate counterfeiter -o mock/ledger_writer.go -fake-name LedgerWriter . LedgerWriter

// LedgerWriter interface, used to read from, and write to, a ledger.
type LedgerWriter interface {
	LedgerReader
	// SetState sets the given value for the given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	SetState(namespace string, key string, value []byte) error
}

//go:generate counterfeiter -o mock/results_iterator.go -fake-name ResultsIterator . ResultsIterator

// ResultsIterator - an iterator for query result set
type ResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (ledger.QueryResult, error)
	// Close releases resources occupied by the iterator
	Close()
}
