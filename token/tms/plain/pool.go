/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"sync"

	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms"
	"github.com/pkg/errors"
)

// OutputNotFoundError is returned when an entry was not found in the pool.
type OutputNotFoundError struct {
	ID string
}

func (o *OutputNotFoundError) Error() string {
	return fmt.Sprintf("entry not found: %s", o.ID)
}

// TxNotFoundError is returned when a transaction was not found in the pool.
type TxNotFoundError struct {
	TxID string
}

func (p *TxNotFoundError) Error() string {
	return fmt.Sprintf("transaction not found: %s", p.TxID)
}

// A MemoryPool is an in-memory ledger of transactions and unspent outputs.
// This implementation is only meant for testing.
type MemoryPool struct {
	mutex   sync.RWMutex
	entries map[string]*token.PlainOutput
	history map[string]*token.TokenTransaction
}

// NewMemoryPool creates a new MemoryPool
func NewMemoryPool() *MemoryPool {
	return &MemoryPool{
		entries: map[string]*token.PlainOutput{},
		history: map[string]*token.TokenTransaction{},
	}
}

// Check if a proposed update can be committed.
func (p *MemoryPool) checkUpdate(transactionData []tms.TransactionData) error {
	for _, td := range transactionData {
		action := td.Tx.GetPlainAction()
		if action == nil {
			return errors.Errorf("check update failed for transaction '%s': missing token action", td.TxID)
		}

		err := p.checkAction(action, td.TxID)
		if err != nil {
			return errors.WithMessage(err, "check update failed")
		}

		if p.history[td.TxID] != nil {
			return errors.Errorf("transaction already exists: %s", td.TxID)
		}
	}

	return nil
}

// CommitUpdate commits transaction data into the pool.
func (p *MemoryPool) CommitUpdate(transactionData []tms.TransactionData) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	err := p.checkUpdate(transactionData)
	if err != nil {
		return err
	}

	for _, td := range transactionData {
		p.commitAction(td.Tx.GetPlainAction(), td.TxID)
		p.history[td.TxID] = cloneTransaction(td.Tx)
	}

	return nil
}

func (p *MemoryPool) checkAction(plainAction *token.PlainTokenAction, txID string) error {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		return p.checkImportAction(action.PlainImport, txID)
	default:
		return errors.Errorf("unknown plain token action: %T", action)
	}
}

func (p *MemoryPool) commitAction(plainAction *token.PlainTokenAction, txID string) {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		p.commitImportAction(action.PlainImport, txID)
	}
}

func (p *MemoryPool) checkImportAction(importAction *token.PlainImport, txID string) error {
	for i := range importAction.GetOutputs() {
		entryID := calculateOutputID(txID, i)
		if p.entries[entryID] != nil {
			return errors.Errorf("pool entry already exists: %s", entryID)
		}
	}
	return nil
}

func (p *MemoryPool) commitImportAction(importAction *token.PlainImport, txID string) {
	for i, entry := range importAction.GetOutputs() {
		entryID := calculateOutputID(txID, i)
		p.addEntry(entryID, entry)
	}
}

// Add a new entry into the pool.
func (p *MemoryPool) addEntry(entryID string, entry *token.PlainOutput) {
	p.entries[entryID] = cloneOutput(entry)
}

func cloneOutput(po *token.PlainOutput) *token.PlainOutput {
	clone := proto.Clone(po)
	return clone.(*token.PlainOutput)
}

func cloneTransaction(tt *token.TokenTransaction) *token.TokenTransaction {
	clone := proto.Clone(tt)
	return clone.(*token.TokenTransaction)
}

// OutputByID gets an output by its ID.
func (p *MemoryPool) OutputByID(id string) (*token.PlainOutput, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	output := p.entries[id]
	if output == nil {
		return nil, &OutputNotFoundError{ID: id}
	}

	return output, nil
}

// TxByID gets a transaction by its transaction ID.
// If no transaction exists with the given ID, a TxNotFoundError is returned.
func (p *MemoryPool) TxByID(txID string) (*token.TokenTransaction, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	tt := p.history[txID]
	if tt == nil {
		return nil, &TxNotFoundError{TxID: txID}
	}

	return tt, nil
}

// Iterator returns an iterator of the unspent outputs based on a copy of the pool.
func (p *MemoryPool) Iterator() *PoolIterator {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	keys := make([]string, len(p.entries))
	i := 0
	for k := range p.entries {
		keys[i] = k
		i++
	}

	return &PoolIterator{pool: p, keys: keys, current: 0}
}

// PoolIterator is an iterator for iterating through the items in the pool.
type PoolIterator struct {
	pool    *MemoryPool
	keys    []string
	current int
}

// Next gets the next output from the pool, or io.EOF if there are no more outputs.
func (it *PoolIterator) Next() (string, *token.PlainOutput, error) {
	if it.current >= len(it.keys) {
		return "", nil, io.EOF
	}

	entryID := it.keys[it.current]
	it.current++
	entry, err := it.pool.OutputByID(entryID)

	return entryID, entry, err
}

// HistoryIterator creates a new HistoryIterator for iterating through the transaction history.
func (p *MemoryPool) HistoryIterator() *HistoryIterator {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	keys := make([]string, len(p.history))
	i := 0
	for k := range p.history {
		keys[i] = k
		i++
	}

	return &HistoryIterator{pool: p, keys: keys, current: 0}
}

// HistoryIterator is an iterator for iterating through the transaction history.
type HistoryIterator struct {
	pool    *MemoryPool
	keys    []string
	current int
}

// Next gets the next transaction from the history, or io.EOF if there are no more transactions.
func (it *HistoryIterator) Next() (string, *token.TokenTransaction, error) {
	if it.current >= len(it.keys) {
		return "", nil, io.EOF
	}

	txID := it.keys[it.current]
	it.current++
	tx, err := it.pool.TxByID(txID)

	return txID, tx, err
}

// Calculate an ID for an individual output in a transaction, as a function of
// the transaction ID, and the index of the output
func calculateOutputID(tmsTxID string, index int) string {
	return fmt.Sprintf("%s.%d", tmsTxID, index)
}
