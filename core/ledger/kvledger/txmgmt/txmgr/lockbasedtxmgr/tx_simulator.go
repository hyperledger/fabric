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

// TxSimulator is a transaction simulator used in `LockBasedTxMgr`
type TxSimulator struct {
	*QueryExecutor
	rwsetBuilder              *rwsetutil.RWSetBuilder
	writePerformed            bool
	pvtdataQueriesPerformed   bool
	simulationResultsComputed bool
	paginatedQueriesPerformed bool
}

func newTxSimulator(txmgr *LockBasedTxMgr, txid string, hasher ledger.Hasher) (*TxSimulator, error) {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	qe := newQueryExecutor(txmgr, txid, rwsetBuilder, true, hasher)
	logger.Debugf("constructing new tx simulator txid = [%s]", txid)
	return &TxSimulator{qe, rwsetBuilder, false, false, false, false}, nil
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetState(ns string, key string, value []byte) error {
	if err := s.checkWritePrecondition(key, value); err != nil {
		return err
	}
	s.rwsetBuilder.AddToWriteSet(ns, key, value)
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetStateMetadata implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetStateMetadata(namespace, key string, metadata map[string][]byte) error {
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToMetadataWriteSet(namespace, key, metadata)
	return nil
}

// DeleteStateMetadata implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) DeleteStateMetadata(namespace, key string) error {
	return s.SetStateMetadata(namespace, key, nil)
}

// SetPrivateData implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetPrivateData(ns, coll, key string, value []byte) error {
	if err := s.QueryExecutor.validateCollName(ns, coll); err != nil {
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
func (s *TxSimulator) DeletePrivateData(ns, coll, key string) error {
	return s.SetPrivateData(ns, coll, key, nil)
}

// SetPrivateDataMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetPrivateDataMultipleKeys(ns, coll string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetPrivateData(ns, coll, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetPrivateDataRangeScanIterator implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.QueryExecutor.GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey)
}

// SetPrivateDataMetadata implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) SetPrivateDataMetadata(namespace, collection, key string, metadata map[string][]byte) error {
	if err := s.QueryExecutor.validateCollName(namespace, collection); err != nil {
		return err
	}
	if err := s.checkWritePrecondition(key, nil); err != nil {
		return err
	}
	s.rwsetBuilder.AddToHashedMetadataWriteSet(namespace, collection, key, metadata)
	return nil
}

// DeletePrivateMetadata implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) DeletePrivateDataMetadata(namespace, collection, key string) error {
	return s.SetPrivateDataMetadata(namespace, collection, key, nil)
}

// ExecuteQueryOnPrivateData implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) ExecuteQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	if err := s.checkBeforePvtdataQueries(); err != nil {
		return nil, err
	}
	return s.QueryExecutor.ExecuteQueryOnPrivateData(namespace, collection, query)
}

// GetStateRangeScanIteratorWithPagination implements method in interface `ledger.QueryExecutor`
func (s *TxSimulator) GetStateRangeScanIteratorWithPagination(namespace string, startKey string,
	endKey string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.QueryExecutor.GetStateRangeScanIteratorWithPagination(namespace, startKey, endKey, pageSize)
}

// ExecuteQueryWithPagination implements method in interface `ledger.QueryExecutor`
func (s *TxSimulator) ExecuteQueryWithPagination(namespace, query, bookmark string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := s.checkBeforePaginatedQueries(); err != nil {
		return nil, err
	}
	return s.QueryExecutor.ExecuteQueryWithPagination(namespace, query, bookmark, pageSize)
}

// GetTxSimulationResults implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	if s.simulationResultsComputed {
		return nil, errors.New("this function should only be called once on a transaction simulator instance")
	}
	defer func() { s.simulationResultsComputed = true }()
	logger.Debugf("Simulation completed, getting simulation results")
	if s.QueryExecutor.err != nil {
		return nil, s.QueryExecutor.err
	}
	s.QueryExecutor.addRangeQueryInfo()
	return s.rwsetBuilder.GetTxSimulationResults()
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *TxSimulator) ExecuteUpdate(query string) error {
	return errors.New("not supported")
}

func (s *TxSimulator) checkWritePrecondition(key string, value []byte) error {
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
	if err := s.QueryExecutor.txmgr.db.ValidateKeyValue(key, value); err != nil {
		return err
	}
	return nil
}

func (s *TxSimulator) checkBeforePvtdataQueries() error {
	if s.writePerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Queries on pvt data is supported only in a read-only transaction", s.txid),
		}
	}
	s.pvtdataQueriesPerformed = true
	return nil
}

func (s *TxSimulator) checkPvtdataQueryPerformed() error {
	if s.pvtdataQueriesPerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Transaction has already performed queries on pvt data. Writes are not allowed", s.txid),
		}
	}
	return nil
}

func (s *TxSimulator) checkBeforePaginatedQueries() error {
	if s.writePerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Paginated queries are supported only in a read-only transaction", s.txid),
		}
	}
	s.paginatedQueriesPerformed = true
	return nil
}

func (s *TxSimulator) checkPaginatedQueryPerformed() error {
	if s.paginatedQueriesPerformed {
		return &txmgr.ErrUnsupportedTransaction{
			Msg: fmt.Sprintf("txid [%s]: Transaction has already performed a paginated query. Writes are not allowed", s.txid),
		}
	}
	return nil
}
