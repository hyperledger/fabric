/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package couchdbtxmgmt

import (
	"errors"
	"reflect"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	logging "github.com/op/go-logging"
)

type kvReadCache struct {
	kvRead      *rwset.KVRead
	cachedValue []byte
}

type nsRWs struct {
	readMap  map[string]*kvReadCache
	writeMap map[string]*rwset.KVWrite
}

func newNsRWs() *nsRWs {
	return &nsRWs{make(map[string]*kvReadCache), make(map[string]*rwset.KVWrite)}
}

// CouchDBTxSimulator is a transaction simulator used in `CouchDBTxMgr`
type CouchDBTxSimulator struct {
	CouchDBQueryExecutor
	rwMap map[string]*nsRWs
	done  bool
}

func (s *CouchDBTxSimulator) getOrCreateNsRWHolder(ns string) *nsRWs {
	var nsRWs *nsRWs
	var ok bool
	if nsRWs, ok = s.rwMap[ns]; !ok {
		nsRWs = newNsRWs()
		s.rwMap[ns] = nsRWs
	}
	return nsRWs
}

// GetState implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) GetState(ns string, key string) ([]byte, error) {
	logger.Debugf("===COUCHDB=== Entering CouchDBTxSimulator.GetState()")
	logger.Debugf("Get state [%s:%s]", ns, key)
	nsRWs := s.getOrCreateNsRWHolder(ns)
	// check if it was written
	kvWrite, ok := nsRWs.writeMap[key]
	if ok {
		// trace the first 200 bytes of value only, in case it is huge
		if logger.IsEnabledFor(logging.DEBUG) {
			if len(kvWrite.Value) < 200 {
				logger.Debugf("Returing value for key=[%s:%s] from write set", ns, key, kvWrite.Value)
			} else {
				logger.Debugf("Returing value for key=[%s:%s] from write set", ns, key, kvWrite.Value[0:200])
			}
		}
		return kvWrite.Value, nil
	}
	// check if it was read
	readCache, ok := nsRWs.readMap[key]
	if ok {
		// trace the first 200 bytes of value only, in case it is huge
		if logger.IsEnabledFor(logging.DEBUG) {
			if len(readCache.cachedValue) < 200 {
				logger.Debugf("Returing value for key=[%s:%s] from read set", ns, key, readCache.cachedValue)
			} else {
				logger.Debugf("Returing value for key=[%s:%s] from read set", ns, key, readCache.cachedValue[0:200])
			}
		}
		return readCache.cachedValue, nil
	}

	// read from storage
	value, version, err := s.txmgr.getCommittedValueAndVersion(ns, key)

	if err != nil {
		return nil, err
	}

	// trace the first 200 bytes of value only, in case it is huge
	if value != nil && logger.IsEnabledFor(logging.DEBUG) {
		if len(value) < 200 {
			logger.Debugf("===Read state from storage key=[%s:%s], value=[%#v], version=[%d]", ns, key, value, version)
		} else {
			logger.Debugf("===Read state from storage key=[%s:%s], value=[%#v...], version=[%d]", ns, key, value[0:200], version)
		}
	}

	nsRWs.readMap[key] = &kvReadCache{rwset.NewKVRead(key, version), value}
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxSimulator.GetState()")
	return value, nil
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
func (s *CouchDBTxSimulator) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	//s.checkDone()
	scanner, err := s.txmgr.getRangeScanner(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &sKVItr{scanner, s}, nil
}

// ExecuteQuery implements method in interface `ledger.QueryExecutor`
func (s *CouchDBTxSimulator) ExecuteQuery(query string) (ledger.ResultsIterator, error) {
	scanner, err := s.txmgr.getQuery(query)
	if err != nil {
		return nil, err
	}
	return &sQueryItr{scanner, s}, nil
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) SetState(ns string, key string, value []byte) error {
	logger.Debugf("===COUCHDB=== Entering CouchDBTxSimulator.SetState()")
	if s.done {
		panic("This method should not be called after calling Done()")
	}
	nsRWs := s.getOrCreateNsRWHolder(ns)
	kvWrite, ok := nsRWs.writeMap[key]
	if ok {
		kvWrite.SetValue(value)
		return nil
	}
	nsRWs.writeMap[key] = rwset.NewKVWrite(key, value)
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxSimulator.SetState()")
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

// Done implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) Done() {
	s.done = true
	s.txmgr.commitRWLock.RUnlock()
}

func (s *CouchDBTxSimulator) getTxReadWriteSet() *rwset.TxReadWriteSet {
	txRWSet := &rwset.TxReadWriteSet{}
	sortedNamespaces := getSortedKeys(s.rwMap)
	for _, ns := range sortedNamespaces {
		//Get namespace specific read-writes
		nsReadWriteMap := s.rwMap[ns]
		//add read set
		reads := []*rwset.KVRead{}
		sortedReadKeys := getSortedKeys(nsReadWriteMap.readMap)
		for _, key := range sortedReadKeys {
			reads = append(reads, nsReadWriteMap.readMap[key].kvRead)
		}

		//add write set
		writes := []*rwset.KVWrite{}
		sortedWriteKeys := getSortedKeys(nsReadWriteMap.writeMap)
		for _, key := range sortedWriteKeys {
			writes = append(writes, nsReadWriteMap.writeMap[key])
		}
		nsRWs := &rwset.NsReadWriteSet{NameSpace: ns, Reads: reads, Writes: writes}
		txRWSet.NsRWs = append(txRWSet.NsRWs, nsRWs)
	}

	// trace the first 2000 characters of RWSet only, in case it is huge
	if logger.IsEnabledFor(logging.DEBUG) {
		txRWSetString := txRWSet.String()
		if len(txRWSetString) < 2000 {
			logger.Debugf("txRWSet = [%s]", txRWSetString)
		} else {
			logger.Debugf("txRWSet = [%s...]", txRWSetString[0:2000])
		}
	}

	return txRWSet
}

func getSortedKeys(m interface{}) []string {
	mapVal := reflect.ValueOf(m)
	keyVals := mapVal.MapKeys()
	keys := []string{}
	for _, keyVal := range keyVals {
		keys = append(keys, keyVal.String())
	}
	return keys
}

// GetTxSimulationResults implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) GetTxSimulationResults() ([]byte, error) {
	logger.Debugf("Simulation completed, getting simulation results")
	return s.getTxReadWriteSet().Marshal()
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// CopyState implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) CopyState(sourceNamespace string, targetNamespace string) error {
	return errors.New("Not yet implemented")
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) ExecuteUpdate(query string) error {
	return errors.New("Not supported by KV data model")
}

type sKVItr struct {
	scanner   *kvScanner
	simulator *CouchDBTxSimulator
}

type sQueryItr struct {
	scanner   *queryScanner
	simulator *CouchDBTxSimulator
}

// Next implements Next() method in ledger.ResultsIterator
// Returns the next item in the result set. The `QueryResult` is expected to be nil when
// the iterator gets exhausted
func (itr *sKVItr) Next() (ledger.QueryResult, error) {
	kv, err := itr.scanner.next()
	if err != nil {
		return nil, err
	}
	if kv == nil {
		return nil, nil
	}

	// Get existing cache for RW at the namespace of the result set if it exists.  If none exists, then create it.
	nsRWs := itr.simulator.getOrCreateNsRWHolder(itr.scanner.namespace)
	nsRWs.readMap[kv.key] = &kvReadCache{
		&rwset.KVRead{Key: kv.key, Version: kv.version}, kv.value}

	return &ledger.KV{Key: kv.key, Value: kv.value}, nil
}

// Close implements Close() method in ledger.ResultsIterator
// which releases resources occupied by the iterator.
func (itr *sKVItr) Close() {
	itr.scanner.close()
}

// Next implements Next() method in ledger.ResultsIterator
func (itr *sQueryItr) Next() (ledger.QueryResult, error) {
	queryRecord, err := itr.scanner.next()
	if err != nil {
		return nil, err
	}
	if queryRecord == nil {
		return nil, nil
	}

	// Get existing cache for RW at the namespace of the result set if it exists.  If none exists, then create it.
	nsRWs := itr.simulator.getOrCreateNsRWHolder(queryRecord.namespace)
	nsRWs.readMap[queryRecord.key] = &kvReadCache{
		&rwset.KVRead{Key: queryRecord.key, Version: queryRecord.version}, queryRecord.record}

	return &ledger.QueryRecord{Namespace: queryRecord.namespace, Key: queryRecord.key, Record: queryRecord.record}, nil
}

// Close implements Close() method in ledger.ResultsIterator
func (itr *sQueryItr) Close() {
	itr.scanner.close()
}
