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

package rwset

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("rwset")

type nsRWs struct {
	readMap          map[string]*KVRead //for mvcc validation
	writeMap         map[string]*KVWrite
	rangeQueriesMap  map[rangeQueryKey]*RangeQueryInfo //for phantom read validation
	rangeQueriesKeys []rangeQueryKey
}

func newNsRWs() *nsRWs {
	return &nsRWs{make(map[string]*KVRead), make(map[string]*KVWrite), make(map[rangeQueryKey]*RangeQueryInfo), nil}
}

type rangeQueryKey struct {
	startKey     string
	endKey       string
	itrExhausted bool
}

// RWSet maintains the read-write set
type RWSet struct {
	rwMap map[string]*nsRWs
}

// NewRWSet constructs a new instance of RWSet
func NewRWSet() *RWSet {
	return &RWSet{make(map[string]*nsRWs)}
}

// AddToReadSet adds a key and corresponding version to the read-set
func (rws *RWSet) AddToReadSet(ns string, key string, version *version.Height) {
	nsRWs := rws.getOrCreateNsRW(ns)
	nsRWs.readMap[key] = NewKVRead(key, version)
}

// AddToWriteSet adds a key and value to the write-set
func (rws *RWSet) AddToWriteSet(ns string, key string, value []byte) {
	nsRWs := rws.getOrCreateNsRW(ns)
	nsRWs.writeMap[key] = NewKVWrite(key, value)
}

// AddToRangeQuerySet adds a range query info for performing phantom read validation
func (rws *RWSet) AddToRangeQuerySet(ns string, rqi *RangeQueryInfo) {
	nsRWs := rws.getOrCreateNsRW(ns)
	key := rangeQueryKey{rqi.StartKey, rqi.EndKey, rqi.ItrExhausted}
	_, ok := nsRWs.rangeQueriesMap[key]
	if !ok {
		nsRWs.rangeQueriesMap[key] = rqi
		nsRWs.rangeQueriesKeys = append(nsRWs.rangeQueriesKeys, key)
	}
}

// GetFromWriteSet return the value of a key from the write-set
func (rws *RWSet) GetFromWriteSet(ns string, key string) ([]byte, bool) {
	nsRWs, ok := rws.rwMap[ns]
	if !ok {
		return nil, false
	}
	var value []byte
	kvWrite, ok := nsRWs.writeMap[key]
	if ok && !kvWrite.IsDelete {
		value = kvWrite.Value
	}
	return value, ok
}

// GetTxReadWriteSet returns the read-write set in the form that can be serialized
func (rws *RWSet) GetTxReadWriteSet() *TxReadWriteSet {
	txRWSet := &TxReadWriteSet{}
	sortedNamespaces := util.GetSortedKeys(rws.rwMap)
	for _, ns := range sortedNamespaces {
		//Get namespace specific read-writes
		nsReadWriteMap := rws.rwMap[ns]

		//add read set
		reads := []*KVRead{}
		sortedReadKeys := util.GetSortedKeys(nsReadWriteMap.readMap)
		for _, key := range sortedReadKeys {
			reads = append(reads, nsReadWriteMap.readMap[key])
		}

		//add write set
		writes := []*KVWrite{}
		sortedWriteKeys := util.GetSortedKeys(nsReadWriteMap.writeMap)
		for _, key := range sortedWriteKeys {
			writes = append(writes, nsReadWriteMap.writeMap[key])
		}

		//add range query info
		rangeQueriesInfo := []*RangeQueryInfo{}
		rangeQueriesMap := nsReadWriteMap.rangeQueriesMap
		for _, key := range nsReadWriteMap.rangeQueriesKeys {
			rangeQueriesInfo = append(rangeQueriesInfo, rangeQueriesMap[key])
		}
		nsRWs := &NsReadWriteSet{NameSpace: ns, Reads: reads, Writes: writes, RangeQueriesInfo: rangeQueriesInfo}
		txRWSet.NsRWs = append(txRWSet.NsRWs, nsRWs)
	}
	return txRWSet
}

func (rws *RWSet) getOrCreateNsRW(ns string) *nsRWs {
	var nsRWs *nsRWs
	var ok bool
	if nsRWs, ok = rws.rwMap[ns]; !ok {
		nsRWs = newNsRWs()
		rws.rwMap[ns] = nsRWs
	}
	return nsRWs
}
