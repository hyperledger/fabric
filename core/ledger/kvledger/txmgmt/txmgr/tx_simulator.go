/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

// txSimulator is a transaction simulator used in `LockBasedTxMgr`
type txSimulator struct {
	*queryExecutor
	rwsetBuilder              *rwsetutil.RWSetBuilder
	writePerformed            bool
	pvtdataQueriesPerformed   bool
	simulationResultsComputed bool
	paginatedQueriesPerformed bool
}

func newTxSimulator(txmgr *LockBasedTxMgr, txid string, hashFunc rwsetutil.HashFunc) (*txSimulator, error) {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	qe := newQueryExecutor(txmgr, txid, rwsetBuilder, true, hashFunc)
	logger.Debugf("constructing new tx simulator txid = [%s]", txid)
	return &txSimulator{qe, rwsetBuilder, false, false, false, false}, nil
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetState(ns string, key string, value []byte) error {
	if err := s.checkWritePrecondition(key, value); err != nil {
		return err
	}
	s.rwsetBuilder.AddToWriteSet(ns, key, value)
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *txSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetStateMetadata implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetStateMetadata(namespace, key string, metadata map[string][]byte) error {
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToMetadataWriteSet(namespace, key, metadata)
	return nil
}

// DeleteStateMetadata implements method in interface `ledger.TxSimulator`
func (s *txSimulator) DeleteStateMetadata(namespace, key string) error {
	return s.SetStateMetadata(namespace, key, nil)
}

// SetPrivateData implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetPrivateData(ns, coll, key string, value []byte) error {
	if err := s.queryExecutor.validateCollName(ns, coll); err != nil {
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
func (s *txSimulator) DeletePrivateData(ns, coll, key string) error {
	return s.SetPrivateData(ns, coll, key, nil)
}

// SetPrivateDataMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetPrivateDataMultipleKeys(ns, coll string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetPrivateData(ns, coll, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetPrivateDataRangeScanIterator implements method in interface `ledger.TxSimulator`
func (s *txSimulator) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.queryExecutor.GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey)
}

// SetPrivateDataMetadata implements method in interface `ledger.TxSimulator`
func (s *txSimulator) SetPrivateDataMetadata(namespace, collection, key string, metadata map[string][]byte) error {
	if err := s.queryExecutor.validateCollName(namespace, collection); err != nil {
		return err
	}
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToHashedMetadataWriteSet(namespace, collection, key, metadata)
	return nil
}

// DeletePrivateMetadata implements method in interface `ledger.TxSimulator`
func (s *txSimulator) DeletePrivateDataMetadata(namespace, collection, key string) error {
	return s.SetPrivateDataMetadata(namespace, collection, key, nil)
}

// ExecuteQueryOnPrivateData implements method in interface `ledger.TxSimulator`
func (s *txSimulator) ExecuteQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.queryExecutor.ExecuteQueryOnPrivateData(namespace, collection, query)
}

// GetStateRangeScanIteratorWithPagination implements method in interface `ledger.QueryExecutor`
func (s *txSimulator) GetStateRangeScanIteratorWithPagination(namespace string, startKey string,
	endKey string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.queryExecutor.GetStateRangeScanIteratorWithPagination(namespace, startKey, endKey, pageSize)
}

// ExecuteQueryWithPagination implements method in interface `ledger.QueryExecutor`
func (s *txSimulator) ExecuteQueryWithPagination(namespace, query, bookmark string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.queryExecutor.ExecuteQueryWithPagination(namespace, query, bookmark, pageSize)
}

// GetTxSimulationResults implements method in interface `ledger.TxSimulator`
func (s *txSimulator) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	if s.simulationResultsComputed {
		return nil, errors.New("this function should only be called once on a transaction simulator instance")
	}
	defer func() { s.simulationResultsComputed = true }()
	logger.Debugf("Simulation completed, getting simulation results")
	if s.queryExecutor.err != nil {
		return nil, s.queryExecutor.err
	}
	s.queryExecutor.addRangeQueryInfo()
	return s.rwsetBuilder.GetTxSimulationResults()
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *txSimulator) ExecuteUpdate(query string) error {
	return errors.New("not supported")
}

func (s *txSimulator) checkWritePrecondition(key string, value []byte) error {
	if err := s.checkDone(); err != nil {
		return err
	}
	if err := s.checkPvtdataQueryPerformed(); err != nil {
		return err
	}
	if err := s.checkPaginatedQueryPerformed(); err != nil {
		return err
	}
	s.writePerformed = true
	return s.queryExecutor.txmgr.db.ValidateKeyValue(key, value)
}

func (s *txSimulator) checkBeforePvtdataQueries() error {
	if s.writePerformed {
		return errors.Errorf("txid [%s]: unsuppored transaction. Queries on pvt data is supported only in a read-only transaction", s.txid)
	}
	s.pvtdataQueriesPerformed = true
	return nil
}

func (s *txSimulator) checkPvtdataQueryPerformed() error {
	if s.pvtdataQueriesPerformed {
		return errors.Errorf("txid [%s]: unsuppored transaction. Transaction has already performed queries on pvt data. Writes are not allowed", s.txid)
	}
	return nil
}

func (s *txSimulator) checkBeforePaginatedQueries() error {
	if s.writePerformed {
		return errors.Errorf("txid [%s]: unsuppored transaction. Paginated queries are supported only in a read-only transaction", s.txid)
	}
	s.paginatedQueriesPerformed = true
	return nil
}

func (s *txSimulator) checkPaginatedQueryPerformed() error {
	if s.paginatedQueriesPerformed {
		return errors.Errorf("txid [%s]: unsuppored transaction. Transaction has already performed a paginated query. Writes are not allowed", s.txid)
	}
	return nil
}
