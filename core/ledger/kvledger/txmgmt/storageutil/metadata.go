/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package storageutil

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

// SerializeMetadata serializes metadata entries for stroing in statedb
func SerializeMetadata(metadataEntries []*kvrwset.KVMetadataEntry) ([]byte, error) {
	metadata := &kvrwset.KVMetadataWrite{Entries: metadataEntries}
	return proto.Marshal(metadata)
}

// DeserializeMetadata deserializes metadata bytes from statedb
func DeserializeMetadata(metadataBytes []byte) (map[string][]byte, error) {
	if metadataBytes == nil {
		return nil, nil
	}
	metadata := &kvrwset.KVMetadataWrite{}
	if err := proto.Unmarshal(metadataBytes, metadata); err != nil {
		return nil, err
	}
	m := make(map[string][]byte, len(metadata.Entries))
	for _, metadataEntry := range metadata.Entries {
		m[metadataEntry.Name] = metadataEntry.Value
	}
	return m, nil
}
