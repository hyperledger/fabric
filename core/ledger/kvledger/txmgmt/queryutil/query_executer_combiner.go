/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryutil

import (
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util"
)

var logger = flogging.MustGetLogger("util")

//go:generate counterfeiter -o mock/query_executer.go -fake-name QueryExecuter . QueryExecuter

// QueryExecuter encapsulates query functions
type QueryExecuter interface {
	GetState(namespace, key string) (*statedb.VersionedValue, error)
	GetStateRangeScanIterator(namespace, startKey, endKey string) (statedb.ResultsIterator, error)
	GetPrivateDataHash(namespace, collection, key string) (*statedb.VersionedValue, error)
}

// QECombiner combines the query results from one or more underlying 'queryExecuters'
// In case, the same key is returned by multiple 'queryExecuters', the first 'queryExecuter'
// in the input is considered having the latest state of the key
type QECombiner struct {
	QueryExecuters []QueryExecuter // actual executers in decending order of priority
}

// GetState implements function in the interface ledger.SimpleQueryExecutor
func (c *QECombiner) GetState(namespace string, key string) ([]byte, error) {
	var vv *statedb.VersionedValue
	var val []byte
	var err error
	for _, qe := range c.QueryExecuters {
		if vv, err = qe.GetState(namespace, key); err != nil {
			return nil, err
		}
		if vv != nil {
			if !vv.IsDelete() {
				val = vv.Value
			}
			break
		}
	}
	return val, nil
}

// GetStateRangeScanIterator implements function in the interface ledger.SimpleQueryExecutor
func (c *QECombiner) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (commonledger.ResultsIterator, error) {
	var itrs []statedb.ResultsIterator
	for _, qe := range c.QueryExecuters {
		itr, err := qe.GetStateRangeScanIterator(namespace, startKey, endKey)
		if err != nil {
			for _, itr := range itrs {
				itr.Close()
			}
			return nil, err
		}
		itrs = append(itrs, itr)
	}
	itrCombiner, err := newItrCombiner(namespace, itrs)
	if err != nil {
		return nil, err
	}
	return itrCombiner, nil
}

func (c *QECombiner) GetPrivateDataHash(namespace, collection, key string) ([]byte, error) {
	var vv *statedb.VersionedValue
	var val []byte
	var err error
	for _, qe := range c.QueryExecuters {
		vv, err = qe.GetPrivateDataHash(namespace, collection, key)
		if err != nil {
			return nil, err
		}
		if vv != nil {
			if !vv.IsDelete() {
				val = vv.Value
			}
			break
		}
	}
	return val, nil
}

// UpdateBatchBackedQueryExecuter wraps an update batch for providing functions in the interface 'queryExecuter'
type UpdateBatchBackedQueryExecuter struct {
	UpdateBatch      *statedb.UpdateBatch
	HashUpdatesBatch *privacyenabledstate.HashedUpdateBatch
}

// GetState implements function in interface 'queryExecuter'
func (qe *UpdateBatchBackedQueryExecuter) GetState(ns, key string) (*statedb.VersionedValue, error) {
	return qe.UpdateBatch.Get(ns, key), nil
}

// GetStateRangeScanIterator implements function in interface 'queryExecuter'
func (qe *UpdateBatchBackedQueryExecuter) GetStateRangeScanIterator(namespace, startKey, endKey string) (statedb.ResultsIterator, error) {
	return qe.UpdateBatch.GetRangeScanIterator(namespace, startKey, endKey), nil
}

func (qe *UpdateBatchBackedQueryExecuter) GetPrivateDataHash(ns, coll, key string) (*statedb.VersionedValue, error) {
	keyHash := util.ComputeStringHash(key)
	return qe.HashUpdatesBatch.Get(ns, coll, string(keyHash)), nil
}
