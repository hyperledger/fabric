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

package rwsetutil

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

var logger = flogging.MustGetLogger("rwsetutil")

type nsRWs struct {
	readMap          map[string]*kvrwset.KVRead //for mvcc validation
	writeMap         map[string]*kvrwset.KVWrite
	rangeQueriesMap  map[rangeQueryKey]*kvrwset.RangeQueryInfo //for phantom read validation
	rangeQueriesKeys []rangeQueryKey
}

func newNsRWs() *nsRWs {
	return &nsRWs{make(map[string]*kvrwset.KVRead),
		make(map[string]*kvrwset.KVWrite),
		make(map[rangeQueryKey]*kvrwset.RangeQueryInfo), nil}
}

type rangeQueryKey struct {
	startKey     string
	endKey       string
	itrExhausted bool
}

// RWSetBuilder helps building the read-write set
type RWSetBuilder struct {
	rwMap map[string]*nsRWs
}

// NewRWSetBuilder constructs a new instance of RWSetBuilder
func NewRWSetBuilder() *RWSetBuilder {
	return &RWSetBuilder{make(map[string]*nsRWs)}
}

// AddToReadSet adds a key and corresponding version to the read-set
func (rws *RWSetBuilder) AddToReadSet(ns string, key string, version *version.Height) {
	nsRWs := rws.getOrCreateNsRW(ns)
	nsRWs.readMap[key] = NewKVRead(key, version)
}

// AddToWriteSet adds a key and value to the write-set
func (rws *RWSetBuilder) AddToWriteSet(ns string, key string, value []byte) {
	nsRWs := rws.getOrCreateNsRW(ns)
	nsRWs.writeMap[key] = newKVWrite(key, value)
}

// AddToRangeQuerySet adds a range query info for performing phantom read validation
func (rws *RWSetBuilder) AddToRangeQuerySet(ns string, rqi *kvrwset.RangeQueryInfo) {
	nsRWs := rws.getOrCreateNsRW(ns)
	key := rangeQueryKey{rqi.StartKey, rqi.EndKey, rqi.ItrExhausted}
	_, ok := nsRWs.rangeQueriesMap[key]
	if !ok {
		nsRWs.rangeQueriesMap[key] = rqi
		nsRWs.rangeQueriesKeys = append(nsRWs.rangeQueriesKeys, key)
	}
}

// GetTxReadWriteSet returns the read-write set in the form that can be serialized
func (rws *RWSetBuilder) GetTxReadWriteSet() *TxRwSet {
	txRWSet := &TxRwSet{}
	sortedNamespaces := util.GetSortedKeys(rws.rwMap)
	for _, ns := range sortedNamespaces {
		//Get namespace specific read-writes
		nsReadWriteMap := rws.rwMap[ns]

		//add read set
		var reads []*kvrwset.KVRead
		sortedReadKeys := util.GetSortedKeys(nsReadWriteMap.readMap)
		for _, key := range sortedReadKeys {
			reads = append(reads, nsReadWriteMap.readMap[key])
		}

		//add write set
		var writes []*kvrwset.KVWrite
		sortedWriteKeys := util.GetSortedKeys(nsReadWriteMap.writeMap)
		for _, key := range sortedWriteKeys {
			writes = append(writes, nsReadWriteMap.writeMap[key])
		}

		//add range query info
		var rangeQueriesInfo []*kvrwset.RangeQueryInfo
		rangeQueriesMap := nsReadWriteMap.rangeQueriesMap
		for _, key := range nsReadWriteMap.rangeQueriesKeys {
			rangeQueriesInfo = append(rangeQueriesInfo, rangeQueriesMap[key])
		}
		kvRWs := &kvrwset.KVRWSet{Reads: reads, Writes: writes, RangeQueriesInfo: rangeQueriesInfo}
		nsRWs := &NsRwSet{ns, kvRWs}
		txRWSet.NsRwSets = append(txRWSet.NsRwSets, nsRWs)
	}
	return txRWSet
}

func (rws *RWSetBuilder) getOrCreateNsRW(ns string) *nsRWs {
	var nsRWs *nsRWs
	var ok bool
	if nsRWs, ok = rws.rwMap[ns]; !ok {
		nsRWs = newNsRWs()
		rws.rwMap[ns] = nsRWs
	}
	return nsRWs
}
