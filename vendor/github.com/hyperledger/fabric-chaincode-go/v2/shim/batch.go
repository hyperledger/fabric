// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

type batchRecordType int

const (
	dataKeyType batchRecordType = iota
	metadataKeyType
)

type batchKey struct {
	Collection string
	Key        string
	Type       batchRecordType
}

type writeBatch struct {
	writes map[batchKey]*peer.WriteRecord
}

func newWriteBatch() *writeBatch {
	return &writeBatch{
		writes: make(map[batchKey]*peer.WriteRecord),
	}
}

func (b *writeBatch) Writes() []*peer.WriteRecord {
	if b == nil {
		return nil
	}

	results := make([]*peer.WriteRecord, 0, len(b.writes))
	for _, value := range b.writes {
		results = append(results, value)
	}

	return results
}

func (b *writeBatch) PutState(collection string, key string, value []byte) {
	b.writeData(&peer.WriteRecord{
		Key:        key,
		Value:      value,
		Collection: collection,
		Type:       peer.WriteRecord_PUT_STATE,
	})
}

func (b *writeBatch) PutStateMetadataEntry(collection string, key string, metakey string, metadata []byte) {
	b.writeMetadata(&peer.WriteRecord{
		Key:        key,
		Collection: collection,
		Metadata:   &peer.StateMetadata{Metakey: metakey, Value: metadata},
		Type:       peer.WriteRecord_PUT_STATE_METADATA,
	})
}

func (b *writeBatch) DelState(collection string, key string) {
	b.writeData(&peer.WriteRecord{
		Key:        key,
		Collection: collection,
		Type:       peer.WriteRecord_DEL_STATE,
	})
}

func (b *writeBatch) PurgeState(collection string, key string) {
	b.writeData(&peer.WriteRecord{
		Key:        key,
		Collection: collection,
		Type:       peer.WriteRecord_PURGE_PRIVATE_DATA,
	})
}

func (b *writeBatch) writeData(record *peer.WriteRecord) {
	key := batchKey{
		Collection: record.Collection,
		Key:        record.Key,
		Type:       dataKeyType,
	}
	b.writes[key] = record
}

func (b *writeBatch) writeMetadata(record *peer.WriteRecord) {
	key := batchKey{
		Collection: record.Collection,
		Key:        record.Key,
		Type:       metadataKeyType,
	}
	b.writes[key] = record
}
