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
	"google.golang.org/protobuf/proto"
)

var logger = flogging.MustGetLogger("rwsetutil")

// Per-entry metering overheads: len(key)+len(value) alone misses map buckets,
// proto headers, slice growth, and the per-key WritesetMetadata entry the
// transaction manager adds for every written key (~300B total measured for a
// public write with an 8-byte key). The constants err high so the metered
// size stays a conservative bound on retained memory. Range query results are
// charged per retained pending result and released when the results helper
// compacts them into the merkle summary, so the charge tracks peak retention,
// not the sum of a scan.
const (
	writeEntryOverhead      = 512
	readEntryOverhead       = 384
	rangeQueryEntryOverhead = 512
	rangeQueryReadOverhead  = 128
)

// RWSetBuilder helps building the read-write set
type RWSetBuilder struct {
	pubRwBuilderMap map[string]*nsPubRwBuilder
	pvtRwBuilderMap map[string]*nsPvtRwBuilder
	// maxRWSetBytes bounds the cumulative size of the read-write set so that a
	// single simulation cannot exhaust the endorser's memory
	maxRWSetBytes int64
	rwSetBytes    int64
	// limitErr records the first limit breach; once set, all further additions
	// and GetTxSimulationResults fail so an oversized simulation can never
	// produce a partial, valid-looking read-write set
	limitErr error
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
// the read-write set size
func NewRWSetBuilder() *RWSetBuilder {
	return NewRWSetBuilderWithMaxRWSetSize(0)
}

// NewRWSetBuilderWithMaxRWSetSize constructs a new instance of RWSetBuilder
// that rejects an addition once the cumulative size of the read-write set
// would exceed maxRWSetBytes; a non-positive maxRWSetBytes disables the limit
func NewRWSetBuilderWithMaxRWSetSize(maxRWSetBytes int64) *RWSetBuilder {
	return &RWSetBuilder{
		pubRwBuilderMap: make(map[string]*nsPubRwBuilder),
		pvtRwBuilderMap: make(map[string]*nsPvtRwBuilder),
		maxRWSetBytes:   maxRWSetBytes,
	}
}

// AddRangeQueryKeys meters a range query's bounds at iterator creation; an
// open iterator references the bounds until the simulation ends, so the charge
// cannot wait for the finalized query info
func (b *RWSetBuilder) AddRangeQueryKeys(startKey, endKey string) error {
	return b.accountSize(rangeQueryEntryOverhead+int64(len(startKey)+len(endKey)), 0)
}

// chargeRangeQueryResult meters one read retained by a range query results
// helper; the charge is released via creditBytes when the helper compacts the
// results into the merkle summary
func (b *RWSetBuilder) chargeRangeQueryResult(key string) error {
	return b.accountSize(rangeQueryReadOverhead+int64(len(key)), 0)
}

// creditBytes releases metered bytes that are no longer retained
func (b *RWSetBuilder) creditBytes(credited int64) error {
	return b.accountSize(0, credited)
}

// AccountMetadata meters state metadata that the transaction manager retains
// per written key for state-based endorsement; the metadata content ships with
// the simulation results and is not covered by the write entry size
func (b *RWSetBuilder) AccountMetadata(metadata, oldMetadata map[string][]byte) error {
	return b.accountSize(metadataMapSize(metadata), metadataMapSize(oldMetadata))
}

// accountSize meters the net growth of the read-write set, crediting
// overwritten entries, and rejects the addition before any state changes once
// the maximum would be exceeded
func (b *RWSetBuilder) accountSize(newBytes, oldBytes int64) error {
	if b.limitErr != nil {
		return b.limitErr
	}
	if b.maxRWSetBytes <= 0 {
		return nil
	}
	updated := b.rwSetBytes - oldBytes + newBytes
	if updated > b.maxRWSetBytes {
		b.limitErr = errors.Errorf(
			"transaction read-write set size %d exceeds the maximum allowed size %d",
			updated, b.maxRWSetBytes,
		)
		// a breach is a likely bulk-write/read abuse; surface it to the operator
		logger.Warningf("rejecting oversized transaction simulation: %s", b.limitErr)
		return b.limitErr
	}
	b.rwSetBytes = updated
	return nil
}

// AddToReadSet adds a key and corresponding version to the read-set
func (b *RWSetBuilder) AddToReadSet(ns string, key string, version *version.Height) error {
	var charge int64
	if existing, ok := b.pubRwBuilderMap[ns]; !ok || existing.readMap[key] == nil {
		// a re-read of an already tracked key only replaces the version
		charge = readEntryOverhead + int64(len(key))
	}
	if err := b.accountSize(charge, 0); err != nil {
		return err
	}
	b.getOrCreateNsPubRwBuilder(ns).readMap[key] = NewKVRead(key, version)
	return nil
}

// AddToWriteSet adds a key and value to the write-set
func (b *RWSetBuilder) AddToWriteSet(ns string, key string, value []byte) error {
	var oldBytes int64
	if nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]; ok {
		if existing := nsPubRwBuilder.writeMap[key]; existing != nil {
			oldBytes = writeEntryOverhead + int64(len(key)+len(existing.GetValue()))
		}
	}
	if err := b.accountSize(writeEntryOverhead+int64(len(key)+len(value)), oldBytes); err != nil {
		return err
	}
	b.getOrCreateNsPubRwBuilder(ns).writeMap[key] = newKVWrite(key, value)
	return nil
}

// AddToMetadataWriteSet adds a metadata to a key in the write-set
// A nil/empty-map for 'metadata' parameter indicates the delete of the metadata
func (b *RWSetBuilder) AddToMetadataWriteSet(ns, key string, metadata map[string][]byte) error {
	metadataWrite := mapToMetadataWrite(key, metadata)
	var oldBytes int64
	if nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]; ok {
		if existing := nsPubRwBuilder.metadataWriteMap[key]; existing != nil {
			oldBytes = writeEntryOverhead + metadataWriteSize(existing)
		}
	}
	if err := b.accountSize(writeEntryOverhead+metadataWriteSize(metadataWrite), oldBytes); err != nil {
		return err
	}
	b.getOrCreateNsPubRwBuilder(ns).metadataWriteMap[key] = metadataWrite
	return nil
}

// AddToRangeQuerySet adds a range query info for performing phantom read validation
func (b *RWSetBuilder) AddToRangeQuerySet(ns string, rqi *kvrwset.RangeQueryInfo) error {
	key := rangeQueryKey{rqi.GetStartKey(), rqi.GetEndKey(), rqi.GetItrExhausted()}
	var existing *nsPubRwBuilder
	if nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]; ok {
		existing = nsPubRwBuilder
	}
	var charge int64
	if existing == nil || existing.rangeQueriesMap[key] == nil {
		// the bounds and raw reads were metered at iterator creation and per
		// result; only a merkle summary (query results above the hashing
		// threshold) is first retained here
		charge = int64(proto.Size(rqi.GetReadsMerkleHashes()))
	}
	if err := b.accountSize(charge, 0); err != nil {
		return err
	}
	nsPubRwBuilder := b.getOrCreateNsPubRwBuilder(ns)
	if _, ok := nsPubRwBuilder.rangeQueriesMap[key]; !ok {
		nsPubRwBuilder.rangeQueriesMap[key] = rqi
		nsPubRwBuilder.rangeQueriesKeys = append(nsPubRwBuilder.rangeQueriesKeys, key)
	}
	return nil
}

// AddToHashedReadSet adds a key and corresponding version to the hashed read-set
func (b *RWSetBuilder) AddToHashedReadSet(ns string, coll string, key string, version *version.Height) error {
	kvReadHash := newPvtKVReadHash(key, version)
	var charge int64
	var existing *collHashRwBuilder
	if nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]; ok {
		existing = nsPubRwBuilder.collHashRwBuilder[coll]
	}
	if existing == nil || existing.readMap[key] == nil {
		// the read map is keyed by the original key, so the key is retained too
		charge = readEntryOverhead + int64(len(key)+len(kvReadHash.GetKeyHash()))
	}
	if err := b.accountSize(charge, 0); err != nil {
		return err
	}
	b.getOrCreateCollHashedRwBuilder(ns, coll).readMap[key] = kvReadHash
	return nil
}

// AddToPvtAndHashedWriteSet adds a key and value to the private and hashed write-set
func (b *RWSetBuilder) AddToPvtAndHashedWriteSet(ns string, coll string, key string, value []byte) error {
	kvWrite, kvWriteHash := newPvtKVWriteAndHash(key, value)
	if err := b.accountSize(writeEntryOverhead+pvtWriteSize(kvWrite)+pvtWriteHashSize(kvWriteHash)+int64(len(key)), b.pvtWriteEntrySize(ns, coll, key)); err != nil {
		return err
	}
	b.getOrCreateCollPvtRwBuilder(ns, coll).writeMap[key] = kvWrite
	b.getOrCreateCollHashedRwBuilder(ns, coll).writeMap[key] = kvWriteHash
	return nil
}

// AddToPvtAndHashedWriteSetForPurge adds a purge key to the hashed write-set
func (b *RWSetBuilder) AddToPvtAndHashedWriteSetForPurge(ns string, coll string, key string) error {
	kvWrite, kvWriteHash := newPvtKVWriteAndHash(key, nil)
	kvWriteHash.IsPurge = true
	if err := b.accountSize(writeEntryOverhead+pvtWriteSize(kvWrite)+pvtWriteHashSize(kvWriteHash)+int64(len(key)), b.pvtWriteEntrySize(ns, coll, key)); err != nil {
		return err
	}
	b.getOrCreateCollPvtRwBuilder(ns, coll).writeMap[key] = kvWrite
	b.getOrCreateCollHashedRwBuilder(ns, coll).writeMap[key] = kvWriteHash
	return nil
}

// pvtWriteEntrySize returns the metered size of the private and hashed write
// entries for a key, or 0 when the key is not tracked yet
func (b *RWSetBuilder) pvtWriteEntrySize(ns, coll, key string) int64 {
	var size int64
	existed := false
	if existing := b.findCollPvtRwBuilder(ns, coll); existing != nil && existing.writeMap[key] != nil {
		size += pvtWriteSize(existing.writeMap[key])
		existed = true
	}
	if existing := b.findCollHashRwBuilder(ns, coll); existing != nil && existing.writeMap[key] != nil {
		// the hashed write map is keyed by the original key
		size += pvtWriteHashSize(existing.writeMap[key]) + int64(len(key))
		existed = true
	}
	if existed {
		size += writeEntryOverhead
	}
	return size
}

// AddToHashedMetadataWriteSet adds a metadata to a key in the hashed write-set
func (b *RWSetBuilder) AddToHashedMetadataWriteSet(ns, coll, key string, metadata map[string][]byte) error {
	// pvt write set just need the key; not the entire metadata. The metadata is stored only
	// by the hashed key. Pvt write-set need to know the key for handling a special case where only
	// metadata is updated so, the version of the key present in the pvt data should be incremented
	metadataWrite := &kvrwset.KVMetadataWrite{Key: key, Entries: nil}
	metadataWriteHash := mapToMetadataWriteHash(key, metadata)
	var oldBytes int64
	existed := false
	if existing := b.findCollPvtRwBuilder(ns, coll); existing != nil && existing.metadataWriteMap[key] != nil {
		oldBytes += metadataWriteSize(existing.metadataWriteMap[key])
		existed = true
	}
	if existing := b.findCollHashRwBuilder(ns, coll); existing != nil && existing.metadataWriteMap[key] != nil {
		// the hashed metadata write map is keyed by the original key
		oldBytes += metadataWriteHashSize(existing.metadataWriteMap[key]) + int64(len(key))
		existed = true
	}
	if existed {
		oldBytes += writeEntryOverhead
	}
	newBytes := writeEntryOverhead + metadataWriteSize(metadataWrite) + metadataWriteHashSize(metadataWriteHash) + int64(len(key))
	if err := b.accountSize(newBytes, oldBytes); err != nil {
		return err
	}
	b.getOrCreateCollPvtRwBuilder(ns, coll).metadataWriteMap[key] = metadataWrite
	b.getOrCreateCollHashedRwBuilder(ns, coll).metadataWriteMap[key] = metadataWriteHash
	return nil
}

// GetTxSimulationResults returns the proto bytes of public rwset
// (public data + hashes of private data) and the private rwset for the transaction
func (b *RWSetBuilder) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	if b.limitErr != nil {
		return nil, b.limitErr
	}
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

// GetTxReadWriteSet returns the read-write set, or nil once the size limit
// has been breached so that no partial read-write set can be obtained
// TODO make this function private once txmgr starts using new function `GetTxSimulationResults` introduced here
func (b *RWSetBuilder) GetTxReadWriteSet() *TxRwSet {
	if b.limitErr != nil {
		// never hand out a partial read-write set after a limit breach
		return nil
	}
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

// findCollPvtRwBuilder returns the builder for a collection without creating it
func (b *RWSetBuilder) findCollPvtRwBuilder(ns, coll string) *collPvtRwBuilder {
	if nsPvtRwBuilder, ok := b.pvtRwBuilderMap[ns]; ok {
		return nsPvtRwBuilder.collPvtRwBuilders[coll]
	}
	return nil
}

// findCollHashRwBuilder returns the builder for a collection without creating it
func (b *RWSetBuilder) findCollHashRwBuilder(ns, coll string) *collHashRwBuilder {
	if nsPubRwBuilder, ok := b.pubRwBuilderMap[ns]; ok {
		return nsPubRwBuilder.collHashRwBuilder[coll]
	}
	return nil
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

func metadataMapSize(metadata map[string][]byte) int64 {
	var size int64
	for name, value := range metadata {
		size += int64(len(name) + len(value))
	}
	return size
}

func pvtWriteSize(kvWrite *kvrwset.KVWrite) int64 {
	if kvWrite == nil {
		return 0
	}
	return int64(len(kvWrite.GetKey()) + len(kvWrite.GetValue()))
}

func pvtWriteHashSize(kvWriteHash *kvrwset.KVWriteHash) int64 {
	if kvWriteHash == nil {
		return 0
	}
	return int64(len(kvWriteHash.GetKeyHash()) + len(kvWriteHash.GetValueHash()))
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
