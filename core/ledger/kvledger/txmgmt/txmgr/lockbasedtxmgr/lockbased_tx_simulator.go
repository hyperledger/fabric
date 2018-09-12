/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/pkg/errors"
)

// LockBasedTxSimulator is a transaction simulator used in `LockBasedTxMgr`
type lockBasedTxSimulator struct {
	lockBasedQueryExecutor
	rwsetBuilder              *rwsetutil.RWSetBuilder
	writePerformed            bool
	pvtdataQueriesPerformed   bool
	simulationResultsComputed bool
	paginatedQueriesPerformed bool
}

func newLockBasedTxSimulator(txmgr *LockBasedTxMgr, txid string) (*lockBasedTxSimulator, error) {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	helper := newQueryHelper(txmgr, rwsetBuilder)
	logger.Debugf("constructing new tx simulator txid = [%s]", txid)
	return &lockBasedTxSimulator{lockBasedQueryExecutor{helper, txid}, rwsetBuilder, false, false, false, false}, nil
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetState(ns string, key string, value []byte) error {
	if err := s.checkWritePrecondition(key, value); err != nil {
		return err
	}
	s.rwsetBuilder.AddToWriteSet(ns, key, value)
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetStateMetadata implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetStateMetadata(namespace, key string, metadata map[string][]byte) error {
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToMetadataWriteSet(namespace, key, metadata)
	return nil
}

// DeleteStateMetadata implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) DeleteStateMetadata(namespace, key string) error {
	return s.SetStateMetadata(namespace, key, nil)
}

// SetPrivateData implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetPrivateData(ns, coll, key string, value []byte) error {
	if err := s.helper.validateCollName(ns, coll); err != nil {
		return err
	}
	if err := s.checkWritePrecondition(key, value); err != nil {
		return err
	}
	s.writePerformed = true
	s.rwsetBuilder.AddToPvtAndHashedWriteSet(ns, coll, key, value)
	return nil
}

// DeletePrivateData implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) DeletePrivateData(ns, coll, key string) error {
	return s.SetPrivateData(ns, coll, key, nil)
}

// SetPrivateDataMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetPrivateDataMultipleKeys(ns, coll string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetPrivateData(ns, coll, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetPrivateDataRangeScanIterator implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.lockBasedQueryExecutor.GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey)
}

// SetPrivateDataMetadata implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetPrivateDataMetadata(namespace, collection, key string, metadata map[string][]byte) error {
	if err := s.helper.validateCollName(namespace, collection); err != nil {
		return err
	}
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToHashedMetadataWriteSet(namespace, collection, key, metadata)
	return nil
}

// DeletePrivateMetadata implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) DeletePrivateDataMetadata(namespace, collection, key string) error {
	return s.SetPrivateDataMetadata(namespace, collection, key, nil)
}

// ExecuteQueryOnPrivateData implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) ExecuteQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.lockBasedQueryExecutor.ExecuteQueryOnPrivateData(namespace, collection, query)
}

// GetStateRangeScanIteratorWithMetadata implements method in interface `ledger.QueryExecutor`
func (s *lockBasedTxSimulator) GetStateRangeScanIteratorWithMetadata(namespace string, startKey string, endKey string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.lockBasedQueryExecutor.GetStateRangeScanIteratorWithMetadata(namespace, startKey, endKey, metadata)
}

// ExecuteQueryWithMetadata implements method in interface `ledger.QueryExecutor`
func (s *lockBasedTxSimulator) ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.lockBasedQueryExecutor.ExecuteQueryWithMetadata(namespace, query, metadata)
}

// GetTxSimulationResults implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	if s.simulationResultsComputed {
		return nil, errors.New("this function should only be called once on a transaction simulator instance")
	}
	defer func() { s.simulationResultsComputed = true }()
	logger.Debugf("Simulation completed, getting simulation results")
	if s.helper.err != nil {
		return nil, s.helper.err
	}
	s.helper.addRangeQueryInfo()
	return s.rwsetBuilder.GetTxSimulationResults()
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) ExecuteUpdate(query string) error {
	return errors.New("not supported")
}

func (s *lockBasedTxSimulator) checkWritePrecondition(key string, value []byte) error {
	if err := s.helper.checkDone(); err != nil {
		return err
	}
	if err := s.checkPvtdataQueryPerformed(); err != nil {
		return err
	}
	if err := s.checkPaginatedQueryPerformed(); err != nil {
		return err
	}
	s.writePerformed = true
	if err := s.helper.txmgr.db.ValidateKeyValue(key, value); err != nil {
		return err
	}
	return nil
}

func (s *lockBasedTxSimulator) checkBeforePvtdataQueries() error {
	if s.writePerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Queries on pvt data is supported only in a read-only transaction", s.txid),
		}
	}
	s.pvtdataQueriesPerformed = true
	return nil
}

func (s *lockBasedTxSimulator) checkPvtdataQueryPerformed() error {
	if s.pvtdataQueriesPerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Transaction has already performed queries on pvt data. Writes are not allowed", s.txid),
		}
	}
	return nil
}

func (s *lockBasedTxSimulator) checkBeforePaginatedQueries() error {
	if s.writePerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Paginated queries are supported only in a read-only transaction", s.txid),
		}
	}
	s.paginatedQueriesPerformed = true
	return nil
}

func (s *lockBasedTxSimulator) checkPaginatedQueryPerformed() error {
	if s.paginatedQueriesPerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Transaction has already performed a paginated query. Writes are not allowed", s.txid),
		}
	}
	return nil
}
