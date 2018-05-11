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

// encode value encodes the versioned value
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

func decodeValue(encodedValue []byte) (*statedb.VersionedValue, error) {
	if oldFormatEncoding(encodedValue) {
		val, ver := decodeValueOldFormat(encodedValue)
		return &statedb.VersionedValue{Version: ver, Value: val, Metadata: nil}, nil
	}
	msg := &msgs.VersionedValueProto{}
	err := proto.Unmarshal(encodedValue[1:], msg)
	if err != nil {
		return nil, err
	}
	ver, _ := version.NewHeightFromBytes(msg.VersionBytes)
	val := msg.Value
	metadata := msg.Metadata
	// protobuf always makes an empty byte array as nil
	if val == nil {
		val = []byte{}
	}
	return &statedb.VersionedValue{Version: ver, Value: val, Metadata: metadata}, nil
}

// encodeValueOldFormat appends the value to the version, allows storage of version and value in binary form.
// With the intorduction of metadata feature, we change the encoding (see function below). However, we retain
// this funtion for test so as to make sure that we can decode old format and support mixed formats present
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
// to use this for decoding the old format data present in the statedb. This function
// should not be used directly or in a tests. The function 'decodeValue' should be used
// for all decodings - which is expected to detect the encoded format and direct the call
// to this function for decoding the values encoded in the old format
func decodeValueOldFormat(encodedValue []byte) ([]byte, *version.Height) {
	height, n := version.NewHeightFromBytes(encodedValue)
	value := encodedValue[n:]
	return value, height
}

func oldFormatEncoding(encodedValue []byte) bool {
	return encodedValue[0] != byte(0)
}
