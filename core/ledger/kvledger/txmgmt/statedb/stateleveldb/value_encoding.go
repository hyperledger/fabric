/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

// encodeValue encodes the value, version, and metadata
func encodeValue(v *statedb.VersionedValue) ([]byte, error) {
	return proto.Marshal(
		&msgs.VersionedValueProto{
			VersionBytes: v.Version.ToBytes(),
			Value:        v.Value,
			Metadata:     v.Metadata,
		},
	)
}

// decodeValue decodes the statedb value bytes
func decodeValue(encodedValue []byte) (*statedb.VersionedValue, error) {
	msg := &msgs.VersionedValueProto{}
	err := proto.Unmarshal(encodedValue, msg)
	if err != nil {
		return nil, err
	}
	ver, _, err := version.NewHeightFromBytes(msg.VersionBytes)
	if err != nil {
		return nil, err
	}
	val := msg.Value
	metadata := msg.Metadata
	// protobuf always makes an empty byte array as nil
	if val == nil {
		val = []byte{}
	}
	return &statedb.VersionedValue{Version: ver, Value: val, Metadata: metadata}, nil
}
