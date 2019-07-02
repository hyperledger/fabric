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

// encode value encodes the versioned value. starting in v1.3 the encoding begins with a nil
// byte and includes metadata.
func encodeValue(v *statedb.VersionedValue) ([]byte, error) {
	vvMsg := &msgs.VersionedValueProto{
		VersionBytes: v.Version.ToBytes(),
		Value:        v.Value,
		Metadata:     v.Metadata,
	}
	encodedValue, err := proto.Marshal(vvMsg)
	if err != nil {
		return nil, err
	}
	encodedValue = append([]byte{0}, encodedValue...)
	return encodedValue, nil
}

// decodeValue decodes the statedb value bytes using either the old (pre-v1.3) encoding
// or the new (v1.3 and later) encoding that supports metadata.
func decodeValue(encodedValue []byte) (*statedb.VersionedValue, error) {
	if oldFormatEncoding(encodedValue) {
		val, ver, err := decodeValueOldFormat(encodedValue)
		if err != nil {
			return nil, err
		}
		return &statedb.VersionedValue{Version: ver, Value: val, Metadata: nil}, nil
	}
	msg := &msgs.VersionedValueProto{}
	err := proto.Unmarshal(encodedValue[1:], msg)
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

// encodeValueOldFormat appends the value to the version, allows storage of version and value in binary form.
// With the introduction of metadata feature in v1.3, we change the encoding (see function below). However, we retain
// this function for test so as to make sure that we can decode old format and support mixed formats present
// in a statedb. This function should be used only in tests to generate the encoding in old format
func encodeValueOldFormat(value []byte, version *version.Height) []byte {
	encodedValue := version.ToBytes()
	if value != nil {
		encodedValue = append(encodedValue, value...)
	}
	return encodedValue
}

// decodeValueOldFormat separates the version and value from a binary value
// See comments in the function `encodeValueOldFormat`. We retain this function as is
// to use this for decoding the old format (pre-v1.3) data present in the statedb. This function
// should not be used directly or in a tests. The function 'decodeValue' should be used
// for all decodings - which is expected to detect the encoded format and direct the call
// to this function for decoding the values encoded in the old format
func decodeValueOldFormat(encodedValue []byte) ([]byte, *version.Height, error) {
	height, n, err := version.NewHeightFromBytes(encodedValue)
	if err != nil {
		return nil, nil, err
	}
	value := encodedValue[n:]
	return value, height, nil
}

// oldFormatEncoding checks whether the value is encoded using the old (pre-v1.3) format
// or new format (v1.3 and later for encoding metadata).
func oldFormatEncoding(encodedValue []byte) bool {
	return encodedValue[0] != byte(0) ||
		(encodedValue[0]|encodedValue[1]) == byte(0) // this check covers a corner case
	// where the old formatted value happens to start with a nil byte. In this corner case,
	// the channel config happen to be persisted for the tuple <block 0, tran 0>. So, this
	// is assumed that block 0 contains a single transaction (i.e., tran 0)
}
