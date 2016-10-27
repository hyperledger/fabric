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

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
)

type kvReadCache struct {
	kvRead      *txmgmt.KVRead
	cachedValue []byte
}

type nsRWs struct {
	readMap  map[string]*kvReadCache
	writeMap map[string]*txmgmt.KVWrite
}

func newNsRWs() *nsRWs {
	return &nsRWs{make(map[string]*kvReadCache), make(map[string]*txmgmt.KVWrite)}
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
		logger.Debugf("Returing value for key=[%s:%s] from write set", ns, key, kvWrite.Value)
		return kvWrite.Value, nil
	}
	// check if it was read
	readCache, ok := nsRWs.readMap[key]
	if ok {
		logger.Debugf("Returing value for key=[%s:%s] from read set", ns, key, readCache.cachedValue)
		return readCache.cachedValue, nil
	}

	// read from storage
	value, version, err := s.txmgr.getCommittedValueAndVersion(ns, key)
	logger.Debugf("Read state from storage key=[%s:%s], value=[%#v], version=[%d]", ns, key, value, version)
	if err != nil {
		return nil, err
	}

	if value != nil {
		jsonString := string(value[:])
		logger.Debugf("===COUCHDB=== GetState() Read jsonString from storage:\n%s\n", jsonString)
	}

	nsRWs.readMap[key] = &kvReadCache{txmgmt.NewKVRead(key, version), value}
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxSimulator.GetState()")
	return value, nil
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
	nsRWs.writeMap[key] = txmgmt.NewKVWrite(key, value)
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

func (s *CouchDBTxSimulator) getTxReadWriteSet() *txmgmt.TxReadWriteSet {
	txRWSet := &txmgmt.TxReadWriteSet{}
	sortedNamespaces := getSortedKeys(s.rwMap)
	for _, ns := range sortedNamespaces {
		//Get namespace specific read-writes
		nsReadWriteMap := s.rwMap[ns]
		//add read set
		reads := []*txmgmt.KVRead{}
		sortedReadKeys := getSortedKeys(nsReadWriteMap.readMap)
		for _, key := range sortedReadKeys {
			reads = append(reads, nsReadWriteMap.readMap[key].kvRead)
		}

		//add write set
		writes := []*txmgmt.KVWrite{}
		sortedWriteKeys := getSortedKeys(nsReadWriteMap.writeMap)
		for _, key := range sortedWriteKeys {
			writes = append(writes, nsReadWriteMap.writeMap[key])
		}
		nsRWs := &txmgmt.NsReadWriteSet{NameSpace: ns, Reads: reads, Writes: writes}
		txRWSet.NsRWs = append(txRWSet.NsRWs, nsRWs)
	}
	logger.Debugf("txRWSet = [%s]", txRWSet)
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
	return s.getTxReadWriteSet().Marshal()
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	return errors.New("Not yet implemented")
}

// CopyState implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) CopyState(sourceNamespace string, targetNamespace string) error {
	return errors.New("Not yet implemented")
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *CouchDBTxSimulator) ExecuteUpdate(query string) error {
	return errors.New("Not supported by KV data model")
}
