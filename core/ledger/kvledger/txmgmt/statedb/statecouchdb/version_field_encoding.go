/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/pkg/errors"
)

func encodeVersionAndMetadata(version *version.Height, metadata []byte) (string, error) {
	if version == nil {
		return "", errors.New("nil version not supported")
	}
	msg := &msgs.VersionFieldProto{
		VersionBytes: version.ToBytes(),
		Metadata:     metadata,
	}
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(msgBytes), nil
}

func decodeVersionAndMetadata(encodedstr string) (*version.Height, []byte, error) {
	versionFieldBytes, err := base64.StdEncoding.DecodeString(encodedstr)
	if err != nil {
		return nil, nil, err
	}
	versionFieldMsg := &msgs.VersionFieldProto{}
	if err = proto.Unmarshal(versionFieldBytes, versionFieldMsg); err != nil {
		return nil, nil, err
	}
	ver, _, err := version.NewHeightFromBytes(versionFieldMsg.VersionBytes)
	if err != nil {
		return nil, nil, err
	}
	return ver, versionFieldMsg.Metadata, nil
}
