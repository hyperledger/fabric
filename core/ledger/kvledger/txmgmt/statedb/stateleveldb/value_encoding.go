/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
)

// encodeValue encodes the value, version, and metadata
func encodeValue(v *state.VersionedValue) ([]byte, error) {
	return proto.Marshal(
		&DBValue{
			Version:  v.Version.ToBytes(),
			Value:    v.Value,
			Metadata: v.Metadata,
		},
	)
}

// decodeValue decodes the statedb value bytes
func decodeValue(encodedValue []byte) (*state.VersionedValue, error) {
	dbValue := &DBValue{}
	err := proto.Unmarshal(encodedValue, dbValue)
	if err != nil {
		return nil, err
	}
	ver, _, err := version.NewHeightFromBytes(dbValue.Version)
	if err != nil {
		return nil, err
	}
	val := dbValue.Value
	metadata := dbValue.Metadata
	// protobuf always makes an empty byte array as nil
	if val == nil {
		val = []byte{}
	}
	return &state.VersionedValue{Version: ver, Value: val, Metadata: metadata}, nil
}
