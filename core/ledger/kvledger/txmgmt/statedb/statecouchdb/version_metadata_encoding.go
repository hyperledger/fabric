/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/pkg/errors"
)

func encodeVersionAndMetadata(version *version.Height, metadata []byte) (string, error) {
	if version == nil {
		return "", errors.New("nil version not supported")
	}
	msg := &VersionAndMetadata{
		Version:  version.ToBytes(),
		Metadata: metadata,
	}
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(msgBytes), nil
}

func decodeVersionAndMetadata(encodedstr string) (*version.Height, []byte, error) {
	persistedVersionAndMetadata, err := base64.StdEncoding.DecodeString(encodedstr)
	if err != nil {
		return nil, nil, err
	}
	versionAndMetadata := &VersionAndMetadata{}
	if err = proto.Unmarshal(persistedVersionAndMetadata, versionAndMetadata); err != nil {
		return nil, nil, err
	}
	ver, _, err := version.NewHeightFromBytes(versionAndMetadata.Version)
	if err != nil {
		return nil, nil, err
	}
	return ver, versionAndMetadata.Metadata, nil
}
