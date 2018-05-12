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
func DeserializeMetadata(metadataBytes []byte) ([]*kvrwset.KVMetadataEntry, error) {
	metadata := &kvrwset.KVMetadataWrite{}
	if err := proto.Unmarshal(metadataBytes, metadata); err != nil {
		return nil, err
	}
	return metadata.Entries, nil
}
