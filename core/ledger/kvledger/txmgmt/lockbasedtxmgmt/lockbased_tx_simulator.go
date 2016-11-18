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

package lockbasedtxmgmt

import (
	"errors"
	"reflect"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	logging "github.com/op/go-logging"
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

// LockBasedTxSimulator is a transaction simulator used in `LockBasedTxMgr`
type LockBasedTxSimulator struct {
	RWLockQueryExecutor
	rwMap map[string]*nsRWs
}

func (s *LockBasedTxSimulator) getOrCreateNsRWHolder(ns string) *nsRWs {
	var nsRWs *nsRWs
	var ok bool
	if nsRWs, ok = s.rwMap[ns]; !ok {
		nsRWs = newNsRWs()
		s.rwMap[ns] = nsRWs
	}
	return nsRWs
}

// GetState implements method in interface `ledger.TxSimulator`
func (s *LockBasedTxSimulator) GetState(ns string, key string) ([]byte, error) {
	s.checkDone()
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

	nsRWs.readMap[key] = &kvReadCache{txmgmt.NewKVRead(key, version), value}
	return value, nil
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
// can be supplied as empty strings. However, a full scan shuold be used judiciously for performance reasons.
// TODO: The range scan queries still do not support Read-Your_Write (RYW)
// semantics as it is still not agreed upon whether we want RYW model or not.
func (s *LockBasedTxSimulator) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.ResultsIterator, error) {
	s.checkDone()
	scanner, err := s.txmgr.getCommittedRangeScanner(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &sKVItr{scanner, s}, nil
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *LockBasedTxSimulator) SetState(ns string, key string, value []byte) error {
	s.checkDone()
	nsRWs := s.getOrCreateNsRWHolder(ns)
	kvWrite, ok := nsRWs.writeMap[key]
	if ok {
		kvWrite.SetValue(value)
		return nil
	}
	nsRWs.writeMap[key] = txmgmt.NewKVWrite(key, value)
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *LockBasedTxSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

func (s *LockBasedTxSimulator) getTxReadWriteSet() *txmgmt.TxReadWriteSet {
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
func (s *LockBasedTxSimulator) GetTxSimulationResults() ([]byte, error) {
	logger.Debugf("Simulation completed, getting simulation results")
	return s.getTxReadWriteSet().Marshal()
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *LockBasedTxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *LockBasedTxSimulator) ExecuteUpdate(query string) error {
	return errors.New("Not supported by KV data model")
}

type sKVItr struct {
	scanner   *kvScanner
	simulator *LockBasedTxSimulator
}

// Next implements Next() method in ledger.ResultsIterator
func (itr *sKVItr) Next() (ledger.QueryResult, error) {
	committedKV, err := itr.scanner.next()
	if err != nil {
		return nil, err
	}
	if committedKV == nil {
		return nil, nil
	}
	if committedKV.isDelete() {
		return itr.Next()
	}
	nsRWs := itr.simulator.getOrCreateNsRWHolder(itr.scanner.namespace)
	nsRWs.readMap[committedKV.key] = &kvReadCache{
		&txmgmt.KVRead{Key: committedKV.key, Version: committedKV.version}, committedKV.value}
	return &ledger.KV{Key: committedKV.key, Value: committedKV.value}, nil
}

// Close implements Close() method in ledger.ResultsIterator
func (itr *sKVItr) Close() {
	itr.scanner.close()
}
