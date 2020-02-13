/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto/rand"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
)

// SignConfigUpdate signs the given configuration update with a
// specific signer.
func SignConfigUpdate(configUpdate *common.ConfigUpdate, signer *Signer) (*common.ConfigSignature, error) {
	signatureHeader, err := signer.CreateSignatureHeader()
	if err != nil {
		return nil, err
	}

	configSignature := &common.ConfigSignature{
		SignatureHeader: MarshalOrPanic(signatureHeader),
	}
	configUpdateBytes := MarshalOrPanic(configUpdate)
	configSignature.Signature, err = signer.Sign(rand.Reader, concatenateBytes(configSignature.SignatureHeader, configUpdateBytes))
	if err != nil {
		return nil, err
	}

	return configSignature, nil
}

// concatenateBytes combines multiple arrays of bytes, for signatures or digests
// over multiple fields.
func concatenateBytes(data ...[]byte) []byte {
	bytes := []byte{}
	for _, d := range data {
		for _, b := range d {
			bytes = append(bytes, b)
		}
	}
	return bytes
}

// MarshalOrPanic serializes a protobuf message and panics if this
// operation fails.
func MarshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return data
}
