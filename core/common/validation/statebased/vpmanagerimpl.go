/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("vscc")

/**********************************************************************************************************/
/**********************************************************************************************************/

type ledgerKeyID struct {
	cc   string
	coll string
	key  string
}

func newLedgerKeyID(cc, coll, key string) *ledgerKeyID {
	return &ledgerKeyID{cc, coll, key}
}

/**********************************************************************************************************/
/**********************************************************************************************************/

// txDependency provides synchronization mechanisms for a transaction's
// dependencies at a vscc scope, where transactions are validated on a per-
// namespace basis:
// -) the pair waitForDepInserted() / signalDepInserted() is used to sync on
//    insertion of dependencies
// -) the pair waitForAndRetrieveValidationResult() / signalValidationResult()
//    is used to sync on the validation results for a given namespace
type txDependency struct {
	mutex               sync.Mutex
	cond                *sync.Cond
	validationResultMap map[string]error
	depInserted         chan struct{}
}

func newTxDependency() *txDependency {
	txd := &txDependency{
		depInserted:         make(chan struct{}),
		validationResultMap: make(map[string]error),
	}
	txd.cond = sync.NewCond(&txd.mutex)
	return txd
}

// waitForDepInserted waits until dependencies introduced by a transaction
// have been inserted. The function returns as soon as
// d.depInserted has been closed by signalDepInserted
func (d *txDependency) waitForDepInserted() {
	<-d.depInserted
}

// signalDepInserted signals that transactions dependencies introduced
// by transaction d.txNum have been inserted. The function
// closes d.depInserted, causing all callers of waitForDepInserted to
// return. This function can only be called once on this object
func (d *txDependency) signalDepInserted() {
	close(d.depInserted)
}

// waitForAndRetrieveValidationResult returns the validation results
// for namespace `ns` - possibly waiting for the corresponding call
// to signalValidationResult to finish first.
func (d *txDependency) waitForAndRetrieveValidationResult(ns string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	err, ok := d.validationResultMap[ns]
	if ok {
		return err
	}

	for !ok {
		d.cond.Wait()
		err, ok = d.validationResultMap[ns]
	}

	return err
}

// signalValidationResult signals that validation of namespace `ns`
// for transaction `d.txnum` completed with error `err`. Results
// are cached into a map. We also broadcast a conditional variable
// to wake up possible callers of waitForAndRetrieveValidationResult
func (d *txDependency) signalValidationResult(ns string, err error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.validationResultMap[ns] = err
	d.cond.Broadcast()
}

/**********************************************************************************************************/
/**********************************************************************************************************/

// validationContext captures the all dependencies within a single block
type validationContext struct {
	// mutex ensures that only one goroutine at a
	// time will modify blockHeight, depsByTxnumMap
	// or depsByLedgerKeyIDMap
	mutex                sync.RWMutex
	blockHeight          uint64
	depsByTxnumMap       map[uint64]*txDependency
	depsByLedgerKeyIDMap map[ledgerKeyID]map[uint64]*txDependency
}

func (c *validationContext) forBlock(newHeight uint64) *validationContext {
	c.mutex.RLock()
	curHeight := c.blockHeight
	c.mutex.RUnlock()

	if curHeight > newHeight {
		logger.Panicf("programming error: block with number %d validated after block with number %d", newHeight, curHeight)
	}

	// block 0 is the genesis block, and so the first block that comes here
	// will actually be block 1, forcing a reset
	if curHeight < newHeight {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		if c.blockHeight < newHeight {
			c.blockHeight = newHeight
			c.depsByLedgerKeyIDMap = map[ledgerKeyID]map[uint64]*txDependency{}
			c.depsByTxnumMap = map[uint64]*txDependency{}
		}
	}

	return c
}

func (c *validationContext) addDependency(kid *ledgerKeyID, txnum uint64, dep *txDependency) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// create map if necessary
	_, ok := c.depsByLedgerKeyIDMap[*kid]
	if !ok {
		c.depsByLedgerKeyIDMap[*kid] = map[uint64]*txDependency{}
	}

	c.depsByLedgerKeyIDMap[*kid][txnum] = dep
}

func (c *validationContext) dependenciesForTxnum(kid *ledgerKeyID, txnum uint64) []*txDependency {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var deps []*txDependency

	dl, in := c.depsByLedgerKeyIDMap[*kid]
	if in {
		deps = make([]*txDependency, 0, len(dl))
		for depTxnum, dep := range dl {
			if depTxnum < txnum {
				deps = append(deps, dep)
			}
		}
	}

	return deps
}

func (c *validationContext) getOrCreateDependencyByTxnum(txnum uint64) *txDependency {
	c.mutex.RLock()
	dep, ok := c.depsByTxnumMap[txnum]
	c.mutex.RUnlock()

	if !ok {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		dep, ok = c.depsByTxnumMap[txnum]
		if !ok {
			dep = newTxDependency()
			c.depsByTxnumMap[txnum] = dep
		}
	}

	return dep
}

func (c *validationContext) waitForValidationResults(kid *ledgerKeyID, blockNum uint64, txnum uint64) error {
	// in the code below we see whether any transaction in this block
	// that precedes txnum introduces a dependency. We do so by
	// extracting from the map all txDependency instances for txnum
	// strictly lower than ours and retrieving their validation
	// result. If the validation result of *any* of them is a nil
	// error, we have a dependency. Otherwise we have no dependency.
	// Note that depsMap is iterated in non-predictable order.
	// This does not violate correctness, since the hasDependencies
	// should return true if *any* dependency has been introduced

	// we proceed in two steps:
	// 1) while holding the mutex, we get a snapshot of all dependencies
	//    that affect us and put them in a local slice; we then release
	//    the mutex
	// 2) we traverse the slice of dependencies and for each, retrieve
	//    the validartion result
	// The two step approach is required to avoid a deadlock where the
	// consumer (the caller of this function) holds the mutex and thus
	// prevents the producer (the caller of signalValidationResult) to
	// produce the result.

	for _, dep := range c.dependenciesForTxnum(kid, txnum) {
		if valErr := dep.waitForAndRetrieveValidationResult(kid.cc); valErr == nil {
			return &ValidationParameterUpdatedError{
				CC:     kid.cc,
				Coll:   kid.coll,
				Key:    kid.key,
				Height: blockNum,
				Txnum:  txnum,
			}
		}
	}
	return nil
}

/**********************************************************************************************************/
/**********************************************************************************************************/

type KeyLevelValidationParameterManagerImpl struct {
	StateFetcher  validation.StateFetcher
	validationCtx validationContext
}

// ExtractValidationParameterDependency implements the method of
// the same name of the KeyLevelValidationParameterManager interface
// Note that this function doesn't take any namespace argument. This is
// because we want to inspect all namespaces for which this transaction
// modifies metadata.
func (m *KeyLevelValidationParameterManagerImpl) ExtractValidationParameterDependency(blockNum, txNum uint64, rwsetBytes []byte) {
	vCtx := m.validationCtx.forBlock(blockNum)

	// this object represents the dependency that transaction (blockNum, txNum) introduces
	dep := vCtx.getOrCreateDependencyByTxnum(txNum)

	rwset := &rwsetutil.TxRwSet{}
	err := rwset.FromProtoBytes(rwsetBytes)
	// note that we silently discard broken read-write
	// sets - ledger will invalidate them anyway
	if err == nil {
		// here we cycle through all metadata updates generated by this transaction
		// and signal that transaction (blockNum, txNum) modifies them so that
		// all subsequent transaction know they have to wait for validation of
		// transaction (blockNum, txNum) before they can continue
		for _, rws := range rwset.NsRwSets {
			for _, mw := range rws.KvRwSet.MetadataWrites {
				// record the fact that this key has a dependency on our tx
				vCtx.addDependency(newLedgerKeyID(rws.NameSpace, "", mw.Key), txNum, dep)
			}

			for _, cw := range rws.CollHashedRwSets {
				for _, mw := range cw.HashedRwSet.MetadataWrites {
					// record the fact that this (pvt) key has a dependency on our tx
					vCtx.addDependency(newLedgerKeyID(rws.NameSpace, cw.CollectionName, string(mw.KeyHash)), txNum, dep)
				}
			}
		}
	} else {
		logger.Warningf("unmarshalling the read write set returned error '%s', skipping", err)
	}

	// signal that we have introduced all dependencies for this transaction
	dep.signalDepInserted()
}

// GetValidationParameterForKey implements the method of
// the same name of the KeyLevelValidationParameterManager interface
func (m *KeyLevelValidationParameterManagerImpl) GetValidationParameterForKey(cc, coll, key string, blockNum, txNum uint64) ([]byte, error) {
	vCtx := m.validationCtx.forBlock(blockNum)

	// wait until all txes before us have introduced dependencies
	for i := int64(txNum) - 1; i >= 0; i-- {
		txdep := vCtx.getOrCreateDependencyByTxnum(uint64(i))
		txdep.waitForDepInserted()
	}

	// wait until the validation results for all dependencies in the cc namespace are available
	// bail, if the validation parameter has been updated in the meantime
	err := vCtx.waitForValidationResults(newLedgerKeyID(cc, coll, key), blockNum, txNum)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	// if we're here, it means that it is safe to retrieve validation
	// parameters for the requested key from the ledger

	state, err := m.StateFetcher.FetchState()
	if err != nil {
		err = errors.WithMessage(err, "could not retrieve ledger")
		logger.Errorf(err.Error())
		return nil, err
	}
	defer state.Done()

	var mdMap map[string][]byte
	if coll == "" {
		mdMap, err = state.GetStateMetadata(cc, key)
		if err != nil {
			err = errors.WithMessage(err, fmt.Sprintf("could not retrieve metadata for %s:%s", cc, key))
			logger.Errorf(err.Error())
			return nil, err
		}
	} else {
		mdMap, err = state.GetPrivateDataMetadataByHash(cc, coll, []byte(key))
		if err != nil {
			err = errors.WithMessage(err, fmt.Sprintf("could not retrieve metadata for %s:%s:%x", cc, coll, []byte(key)))
			logger.Errorf(err.Error())
			return nil, err
		}
	}

	return mdMap[pb.MetaDataKeys_VALIDATION_PARAMETER.String()], nil
}

// SetTxValidationCode implements the method of the same name of
// the KeyLevelValidationParameterManager interface. Note that
// this function receives a namespace argument so that it records
// the validation result for this transaction and for this chaincode.
func (m *KeyLevelValidationParameterManagerImpl) SetTxValidationResult(ns string, blockNum, txNum uint64, err error) {
	vCtx := m.validationCtx.forBlock(blockNum)

	// this object represents the dependency that the transaction of our caller introduces
	dep := vCtx.getOrCreateDependencyByTxnum(txNum)

	// signal the validation status of this tx
	dep.signalValidationResult(ns, err)
}
