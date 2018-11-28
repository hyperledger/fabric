/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"github.com/hyperledger/fabric/common/ledger"
)

// A MemoryLedger is an in-memory ledger of transactions and unspent outputs.
// This implementation is only meant for testing.
type MemoryLedger struct {
	entries map[string][]byte
}

// NewMemoryLedger creates a new MemoryLedger
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{
		entries: make(map[string][]byte),
	}
}

// GetState gets the value for given namespace and Key. For a chaincode, the namespace corresponds to the chaincodeID
func (p *MemoryLedger) GetState(namespace string, key string) ([]byte, error) {
	value := p.entries[key]

	return value, nil
}

// SetState sets the given value for the given namespace and Key. For a chaincode, the namespace corresponds to the chaincodeID
func (p *MemoryLedger) SetState(namespace string, key string, value []byte) error {
	p.entries[key] = value

	return nil
}

// GetStateRangeScanIterator gets the values for a given namespace that lie in an interval determined by startKey and endKey.
// this is a mock function.
func (p *MemoryLedger) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	return nil, nil
}

// Done releases resources occupied by the MemoryLedger
func (p *MemoryLedger) Done() {
	// No resources to be released for MemoryLedger
}
