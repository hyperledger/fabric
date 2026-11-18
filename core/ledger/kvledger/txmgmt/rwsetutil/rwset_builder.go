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
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("rwsetutil")

// RWSetBuilder helps building the read-write set
type RWSetBuilder struct {
	pubRwBuilderMap map[string]*nsPubRwBuilder
	pvtRwBuilderMap map[string]*nsPvtRwBuilder
	// maxWriteSetBytes bounds the cumulative size of the write set so that a
	// single simulation cannot exhaust the endorser's memory
	maxWriteSetBytes int64
	writeSetBytes    int64
}

type nsPubRwBuilder struct {
	namespace         string
	readMap           map[string]*kvrwset.KVRead // for mvcc validation
	writeMap          map[string]*kvrwset.KVWrite
	metadataWriteMap  map[string]*kvrwset.KVMetadataWrite
	rangeQueriesMap   map[rangeQueryKey]*kvrwset.RangeQueryInfo // for phantom read validation
	rangeQueriesKeys  []rangeQueryKey
	collHashRwBuilder map[string]*collHashRwBuilder
}

type collHashRwBuilder struct {
	collName         string
	readMap          map[string]*kvrwset.KVReadHash
	writeMap         map[string]*kvrwset.KVWriteHash
	metadataWriteMap map[string]*kvrwset.KVMetadataWriteHash
	pvtDataHash      []byte
}

type nsPvtRwBuilder struct {
	namespace         string
	collPvtRwBuilders map[string]*collPvtRwBuilder
}

type collPvtRwBuilder struct {
	collectionName   string
	writeMap         map[string]*kvrwset.KVWrite
	metadataWriteMap map[string]*kvrwset.KVMetadataWrite
}

type rangeQueryKey struct {
	startKey     string
	endKey       string
	itrExhausted bool
}

// NewRWSetBuilder constructs a new instance of RWSetBuilder with no limit on
// the write set size
func NewRWSetBuilder() *RWSetBuilder {
	return NewRWSetBuilderWithMaxWriteSetSize(0)
}

// NewRWSetBuilderWithMaxWriteSetSize constructs a new instance of RWSetBuilder
// that rejects a write once the cumulative size of the write set keys and
// values would exceed maxWriteSetBytes; a non-positive maxWriteSetBytes
// disables the limit
func NewRWSetBuilderWithMaxWriteSetSize(maxWriteSetBytes int64) *RWSetBuilder {
	return &RWSetBuilder{
		pubRwBuilderMap:  make(map[string]*nsPubRwBuilder),
		pvtRwBuilderMap:  make(map[string]*nsPvtRwBuilder),
		maxWriteSetBytes: maxWriteSetBytes,
	}
}

// accountWriteSetSize meters the net growth of the write set, crediting
// overwritten entries, and rejects the write before any state changes once
// the maximum would be exceeded
func (b *RWSetBuilder) accountWriteSetSize(newBytes, oldBytes int64) error {
	if b.maxWriteSetBytes <= 0 {
		return nil
	}
	updated := b.writeSetBytes - oldBytes + newBytes
	if updated > b.maxWriteSetBytes {
		return errors.Errorf(
			"transaction write set size %d exceeds the maximum allowed size %d",
			updated, b.maxWriteSetBytes,
		)
	}
	b.writeSetBytes = updated
	return nil
}

// AddToReadSet adds a key and corresponding version to the read-set
func (b *RWSetBuilder) AddToReadSet(ns string, key string, version *version.Height) {
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	nsPubRwBuilder.readMap[key] = NewKVRead(key, version)
}

// AddToWriteSet adds a key and value to the write-set
func (b *RWSetBuilder) AddToWriteSet(ns string, key string, value []byte) error {
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	var oldBytes int64
	if existing := nsPubRwBuilder.writeMap[key]; existing != nil {
		oldBytes = int64(len(key) + len(existing.GetValue()))
	}
	if err := b.accountWriteSetSize(int64(len(key)+len(value)), oldBytes); err != nil {
		return err
	}
	nsPubRwBuilder.writeMap[key] = newKVWrite(key, value)
	return nil
}

// AddToMetadataWriteSet adds a metadata to a key in the write-set
// A nil/empty-map for 'metadata' parameter indicates the delete of the metadata
func (b *RWSetBuilder) AddToMetadataWriteSet(ns, key string, metadata map[string][]byte) error {
	metadataWrite := mapToMetadataWrite(key, metadata)
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	var oldBytes int64
	if existing := nsPubRwBuilder.metadataWriteMap[key]; existing != nil {
		oldBytes = metadataWriteSize(existing)
	}
	if err := b.accountWriteSetSize(metadataWriteSize(metadataWrite), oldBytes); err != nil {
		return err
	}
	nsPubRwBuilder.metadataWriteMap[key] = metadataWrite
	return nil
}

// AddToRangeQuerySet adds a range query info for performing phantom read validation
func (b *RWSetBuilder) AddToRangeQuerySet(ns string, rqi *kvrwset.RangeQueryInfo) {
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	key := rangeQueryKey{rqi.GetStartKey(), rqi.GetEndKey(), rqi.GetItrExhausted()}
	_, ok := nsPubRwBuilder.rangeQueriesMap[key]
	if !ok {
		nsPubRwBuilder.rangeQueriesMap[key] = rqi
		nsPubRwBuilder.rangeQueriesKeys = append(nsPubRwBuilder.rangeQueriesKeys, key)
	}
}

// AddToHashedReadSet adds a key and corresponding version to the hashed read-set
func (b *RWSetBuilder) AddToHashedReadSet(ns string, coll string, key string, version *version.Height) {
	kvReadHash := newPvtKVReadHash(key, version)
	b.getOrCreateCollHashedRwBuilder(ns, coll).readMap[key] = kvReadHash
}

// AddToPvtAndHashedWriteSet adds a key and value to the private and hashed write-set
func (b *RWSetBuilder) AddToPvtAndHashedWriteSet(ns string, coll string, key string, value []byte) error {
	kvWrite, kvWriteHash := newPvtKVWriteAndHash(key, value)
	pvtRwBuilder := b.getOrCreateCollPvtRwBuilder(ns, coll)
	hashRwBuilder := b.getOrCreateCollHashedRwBuilder(ns, coll)
	var oldBytes int64
	if existing := pvtRwBuilder.writeMap[key]; existing != nil {
		oldBytes += int64(len(existing.GetKey()) + len(existing.GetValue()))
	}
	if existing := hashRwBuilder.writeMap[key]; existing != nil {
		oldBytes += int64(len(existing.GetKeyHash()) + len(existing.GetValueHash()))
	}
	newBytes := int64(len(kvWrite.GetKey())+len(kvWrite.GetValue())) +
		int64(len(kvWriteHash.GetKeyHash())+len(kvWriteHash.GetValueHash()))
	if err := b.accountWriteSetSize(newBytes, oldBytes); err != nil {
		return err
	}
	pvtRwBuilder.writeMap[key] = kvWrite
	hashRwBuilder.writeMap[key] = kvWriteHash
	return nil
}

// AddToPvtAndHashedWriteSetForPurge adds a purge key to the hashed write-set
func (b *RWSetBuilder) AddToPvtAndHashedWriteSetForPurge(ns string, coll string, key string) error {
	kvWrite, kvWriteHash := newPvtKVWriteAndHash(key, nil)
	kvWriteHash.IsPurge = true
	pvtRwBuilder := b.getOrCreateCollPvtRwBuilder(ns, coll)
	hashRwBuilder := b.getOrCreateCollHashedRwBuilder(ns, coll)
	var oldBytes int64
	if existing := pvtRwBuilder.writeMap[key]; existing != nil {
		oldBytes += int64(len(existing.GetKey()) + len(existing.GetValue()))
	}
	if existing := hashRwBuilder.writeMap[key]; existing != nil {
		oldBytes += int64(len(existing.GetKeyHash()) + len(existing.GetValueHash()))
	}
	newBytes := int64(len(kvWrite.GetKey())+len(kvWrite.GetValue())) +
		int64(len(kvWriteHash.GetKeyHash())+len(kvWriteHash.GetValueHash()))
	if err := b.accountWriteSetSize(newBytes, oldBytes); err != nil {
		return err
	}
	pvtRwBuilder.writeMap[key] = kvWrite
	hashRwBuilder.writeMap[key] = kvWriteHash
	return nil
}

// AddToHashedMetadataWriteSet adds a metadata to a key in the hashed write-set
func (b *RWSetBuilder) AddToHashedMetadataWriteSet(ns, coll, key string, metadata map[string][]byte) error {
	// pvt write set just need the key; not the entire metadata. The metadata is stored only
	// by the hashed key. Pvt write-set need to know the key for handling a special case where only
	// metadata is updated so, the version of the key present in the pvt data should be incremented
	metadataWrite := &kvrwset.KVMetadataWrite{Key: key, Entries: nil}
	metadataWriteHash := mapToMetadataWriteHash(key, metadata)
	pvtRwBuilder := b.getOrCreateCollPvtRwBuilder(ns, coll)
	hashRwBuilder := b.getOrCreateCollHashedRwBuilder(ns, coll)
	var oldBytes int64
	if existing := pvtRwBuilder.metadataWriteMap[key]; existing != nil {
		oldBytes += metadataWriteSize(existing)
	}
	if existing := hashRwBuilder.metadataWriteMap[key]; existing != nil {
		oldBytes += metadataWriteHashSize(existing)
	}
	if err := b.accountWriteSetSize(metadataWriteSize(metadataWrite)+metadataWriteHashSize(metadataWriteHash), oldBytes); err != nil {
		return err
	}
	pvtRwBuilder.metadataWriteMap[key] = metadataWrite
	hashRwBuilder.metadataWriteMap[key] = metadataWriteHash
	return nil
}

// GetTxSimulationResults returns the proto bytes of public rwset
// (public data + hashes of private data) and the private rwset for the transaction
func (b *RWSetBuilder) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	pvtData := b.getTxPvtReadWriteSet()
	var err error

	var pubDataProto *rwset.TxReadWriteSet
	var pvtDataProto *rwset.TxPvtReadWriteSet

	// Populate the collection-level hashes into pub rwset and compute the proto bytes for pvt rwset
	if pvtData != nil {
		if pvtDataProto, err = pvtData.ToProtoMsg(); err != nil {
			return nil, err
		}
		for _, ns := range pvtDataProto.GetNsPvtRwset() {
			for _, coll := range ns.GetCollectionPvtRwset() {
				b.setPvtCollectionHash(ns.GetNamespace(), coll.GetCollectionName(), coll.GetRwset())
			}
		}
	}
	// Compute the proto bytes for pub rwset
	pubSet := b.GetTxReadWriteSet()
	if pubSet != nil {
		if pubDataProto, err = pubSet.toProtoMsg(); err != nil {
			return nil, err
		}
	}
	return &ledger.TxSimulationResults{
		PubSimulationResults: pubDataProto,
		PvtSimulationResults: pvtDataProto,
	}, nil
}

func (b *RWSetBuilder) setPvtCollectionHash(ns string, coll string, pvtDataProto []byte) {
	collHashedBuilder := b.getOrCreateCollHashedRwBuilder(ns, coll)
	collHashedBuilder.pvtDataHash = util.ComputeHash(pvtDataProto)
}

// GetTxReadWriteSet returns the read-write set
// TODO make this function private once txmgr starts using new function `GetTxSimulationResults` introduced here
func (b *RWSetBuilder) GetTxReadWriteSet() *TxRwSet {
	sortedNsPubBuilders := []*nsPubRwBuilder{}
	util.GetValuesBySortedKeys(&b.pubRwBuilderMap, &sortedNsPubBuilders)

	nsPubRwSets := make([]*NsRwSet, 0, len(sortedNsPubBuilders))
	for _, nsPubRwBuilder := range sortedNsPubBuilders {
		nsPubRwSets = append(nsPubRwSets, nsPubRwBuilder.build())
	}
	return &TxRwSet{NsRwSets: nsPubRwSets}
}

// getTxPvtReadWriteSet returns the private read-write set
func (b *RWSetBuilder) getTxPvtReadWriteSet() *TxPvtRwSet {
	sortedNsPvtBuilders := []*nsPvtRwBuilder{}
	util.GetValuesBySortedKeys(&b.pvtRwBuilderMap, &sortedNsPvtBuilders)

	nsPvtRwSets := make([]*NsPvtRwSet, 0, len(sortedNsPvtBuilders))
	for _, nsPvtRwBuilder := range sortedNsPvtBuilders {
		nsPvtRwSets = append(nsPvtRwSets, nsPvtRwBuilder.build())
	}
	if len(nsPvtRwSets) == 0 {
		return nil
	}
	return &TxPvtRwSet{NsPvtRwSet: nsPvtRwSets}
}

func (b *nsPubRwBuilder) build() *NsRwSet {
	var readSet []*kvrwset.KVRead
	var writeSet []*kvrwset.KVWrite
	var metadataWriteSet []*kvrwset.KVMetadataWrite
	// add read set
	util.GetValuesBySortedKeys(&b.readMap, &readSet)
	// add write set
	util.GetValuesBySortedKeys(&b.writeMap, &writeSet)
	util.GetValuesBySortedKeys(&b.metadataWriteMap, &metadataWriteSet)

	rangeQueriesInfo := make([]*kvrwset.RangeQueryInfo, 0, len(b.rangeQueriesKeys))
	// add range query info
	for _, key := range b.rangeQueriesKeys {
		rangeQueriesInfo = append(rangeQueriesInfo, b.rangeQueriesMap[key])
	}
	// add hashed rws for private collections
	sortedCollBuilders := []*collHashRwBuilder{}
	util.GetValuesBySortedKeys(&b.collHashRwBuilder, &sortedCollBuilders)

	collHashedRwSet := make([]*CollHashedRwSet, 0, len(sortedCollBuilders))
	for _, collBuilder := range sortedCollBuilders {
		collHashedRwSet = append(collHashedRwSet, collBuilder.build())
	}

	return &NsRwSet{
		NameSpace: b.namespace,
		KvRwSet: &kvrwset.KVRWSet{
			Reads:            readSet,
			Writes:           writeSet,
			MetadataWrites:   metadataWriteSet,
			RangeQueriesInfo: rangeQueriesInfo,
		},
		CollHashedRwSets: collHashedRwSet,
	}
}

func (b *nsPvtRwBuilder) build() *NsPvtRwSet {
	sortedCollBuilders := []*collPvtRwBuilder{}
	util.GetValuesBySortedKeys(&b.collPvtRwBuilders, &sortedCollBuilders)

	collPvtRwSets := make([]*CollPvtRwSet, 0, len(sortedCollBuilders))
	for _, collBuilder := range sortedCollBuilders {
		collPvtRwSets = append(collPvtRwSets, collBuilder.build())
	}
	return &NsPvtRwSet{NameSpace: b.namespace, CollPvtRwSets: collPvtRwSets}
}

func (b *collHashRwBuilder) build() *CollHashedRwSet {
	var readSet []*kvrwset.KVReadHash
	var writeSet []*kvrwset.KVWriteHash
	var metadataWriteSet []*kvrwset.KVMetadataWriteHash

	util.GetValuesBySortedKeys(&b.readMap, &readSet)
	util.GetValuesBySortedKeys(&b.writeMap, &writeSet)
	util.GetValuesBySortedKeys(&b.metadataWriteMap, &metadataWriteSet)
	return &CollHashedRwSet{
		CollectionName: b.collName,
		HashedRwSet: &kvrwset.HashedRWSet{
			HashedReads:    readSet,
			HashedWrites:   writeSet,
			MetadataWrites: metadataWriteSet,
		},
		PvtRwSetHash: b.pvtDataHash,
	}
}

func (b *collPvtRwBuilder) build() *CollPvtRwSet {
	var writeSet []*kvrwset.KVWrite
	var metadataWriteSet []*kvrwset.KVMetadataWrite
	util.GetValuesBySortedKeys(&b.writeMap, &writeSet)
	util.GetValuesBySortedKeys(&b.metadataWriteMap, &metadataWriteSet)
	return &CollPvtRwSet{
		CollectionName: b.collectionName,
		KvRwSet: &kvrwset.KVRWSet{
			Writes:         writeSet,
			MetadataWrites: metadataWriteSet,
		},
	}
}

func (b *RWSetBuilder) getOrCreateNsPubRwBuilder(ns string) *nsPubRwBuilder {
	nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]
	if !ok {
		nsPubRwBuilder = newNsPubRwBuilder(ns)
		b.pubRwBuilderMap[ns] = nsPubRwBuilder
	}
	return nsPubRwBuilder
}

func (b *RWSetBuilder) getOrCreateNsPvtRwBuilder(ns string) *nsPvtRwBuilder {
	nsPvtRwBuilder, ok := b.pvtRwBuilderMap[ns]
	if !ok {
		nsPvtRwBuilder = newNsPvtRwBuilder(ns)
		b.pvtRwBuilderMap[ns] = nsPvtRwBuilder
	}
	return nsPvtRwBuilder
}

func (b *RWSetBuilder) getOrCreateCollHashedRwBuilder(ns string, coll string) *collHashRwBuilder {
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	collHashRwBuilder, ok := nsPubRwBuilder.collHashRwBuilder[coll]
	if !ok {
		collHashRwBuilder = newCollHashRwBuilder(coll)
		nsPubRwBuilder.collHashRwBuilder[coll] = collHashRwBuilder
	}
	return collHashRwBuilder
}

func (b *RWSetBuilder) getOrCreateCollPvtRwBuilder(ns string, coll string) *collPvtRwBuilder {
	nsPvtRwBuilder := b.getOrCreateNsPvtRwBuilder(ns)
	collPvtRwBuilder, ok := nsPvtRwBuilder.collPvtRwBuilders[coll]
	if !ok {
		collPvtRwBuilder = newCollPvtRwBuilder(coll)
		nsPvtRwBuilder.collPvtRwBuilders[coll] = collPvtRwBuilder
	}
	return collPvtRwBuilder
}

func newNsPubRwBuilder(namespace string) *nsPubRwBuilder {
	return &nsPubRwBuilder{
		namespace,
		make(map[string]*kvrwset.KVRead),
		make(map[string]*kvrwset.KVWrite),
		make(map[string]*kvrwset.KVMetadataWrite),
		make(map[rangeQueryKey]*kvrwset.RangeQueryInfo),
		nil,
		make(map[string]*collHashRwBuilder),
	}
}

func newNsPvtRwBuilder(namespace string) *nsPvtRwBuilder {
	return &nsPvtRwBuilder{namespace, make(map[string]*collPvtRwBuilder)}
}

func newCollHashRwBuilder(collName string) *collHashRwBuilder {
	return &collHashRwBuilder{
		collName,
		make(map[string]*kvrwset.KVReadHash),
		make(map[string]*kvrwset.KVWriteHash),
		make(map[string]*kvrwset.KVMetadataWriteHash),
		nil,
	}
}

func newCollPvtRwBuilder(collName string) *collPvtRwBuilder {
	return &collPvtRwBuilder{
		collName,
		make(map[string]*kvrwset.KVWrite),
		make(map[string]*kvrwset.KVMetadataWrite),
	}
}

func mapToMetadataWrite(key string, m map[string][]byte) *kvrwset.KVMetadataWrite {
	proto := &kvrwset.KVMetadataWrite{Key: key}
	names := util.GetSortedKeys(m)
	for _, name := range names {
		proto.Entries = append(
			proto.Entries,
			&kvrwset.KVMetadataEntry{Name: name, Value: m[name]},
		)
	}
	return proto
}

func mapToMetadataWriteHash(key string, m map[string][]byte) *kvrwset.KVMetadataWriteHash {
	proto := &kvrwset.KVMetadataWriteHash{KeyHash: util.ComputeStringHash(key)}
	names := util.GetSortedKeys(m)
	for _, name := range names {
		proto.Entries = append(
			proto.Entries,
			&kvrwset.KVMetadataEntry{Name: name, Value: m[name]},
		)
	}
	return proto
}

func metadataWriteSize(metadataWrite *kvrwset.KVMetadataWrite) int64 {
	size := int64(len(metadataWrite.GetKey()))
	for _, entry := range metadataWrite.GetEntries() {
		size += int64(len(entry.GetName()) + len(entry.GetValue()))
	}
	return size
}

func metadataWriteHashSize(metadataWriteHash *kvrwset.KVMetadataWriteHash) int64 {
	size := int64(len(metadataWriteHash.GetKeyHash()))
	for _, entry := range metadataWriteHash.GetEntries() {
		size += int64(len(entry.GetName()) + len(entry.GetValue()))
	}
	return size
}
