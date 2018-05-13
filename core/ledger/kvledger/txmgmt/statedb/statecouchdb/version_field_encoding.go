/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	proto "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func encodeVersionAndMetadata(version *version.Height, metadata []byte) (string, error) {
	msg := &msgs.VersionFieldProto{
		VersionBytes: version.ToBytes(),
		Metadata:     metadata,
	}
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	msgBase64 := base64.StdEncoding.EncodeToString(msgBytes)
	encodedVersionField := append([]byte{byte(0)}, []byte(msgBase64)...)
	return string(encodedVersionField), nil
}

func decodeVersionAndMetadata(encodedstr string) (*version.Height, []byte, error) {
	if oldFormatEncoding(encodedstr) {
		return decodeVersionOldFormat(encodedstr), nil, nil
	}
	versionFieldBytes, err := base64.StdEncoding.DecodeString(encodedstr[1:])
	if err != nil {
		return nil, nil, err
	}
	versionFieldMsg := &msgs.VersionFieldProto{}
	if err = proto.Unmarshal(versionFieldBytes, versionFieldMsg); err != nil {
		return nil, nil, err
	}
	ver, _ := version.NewHeightFromBytes(versionFieldMsg.VersionBytes)
	return ver, versionFieldMsg.Metadata, nil
}

// encodeVersionOldFormat return string representation of version
// With the intorduction of metadata feature, we change the encoding (see function below). However, we retain
// this funtion for test so as to make sure that we can decode old format and support mixed formats present
// in a statedb. This function should be used only in tests to generate the encoding in old format
func encodeVersionOldFormat(version *version.Height) string {
	return fmt.Sprintf("%v:%v", version.BlockNum, version.TxNum)
}

// decodeVersionOldFormat separates the version and value from encoded string
// See comments in the function `encodeVersionOldFormat`. We retain this function as is
// to use this for decoding the old format data present in the statedb. This function
// should not be used directly or in a tests. The function 'decodeVersionAndMetadata' should be used
// for all decodings - which is expected to detect the encoded format and direct the call
// to this function for decoding the versions encoded in the old format
func decodeVersionOldFormat(encodedVersion string) *version.Height {
	versionArray := strings.Split(fmt.Sprintf("%s", encodedVersion), ":")
	// convert the blockNum from String to unsigned int
	blockNum, _ := strconv.ParseUint(versionArray[0], 10, 64)
	// convert the txNum from String to unsigned int
	txNum, _ := strconv.ParseUint(versionArray[1], 10, 64)
	return version.NewHeight(blockNum, txNum)
}

func oldFormatEncoding(encodedstr string) bool {
	return []byte(encodedstr)[0] != byte(0)
}
