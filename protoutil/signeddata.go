/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
)

// SignedData is used to represent the general triplet required to verify a signature
// This is intended to be generic across crypto schemes, while most crypto schemes will
// include the signing identity and a nonce within the Data, this is left to the crypto
// implementation.
type SignedData struct {
	Data      []byte
	Identity  []byte
	Signature []byte
}

// ConfigUpdateEnvelopeAsSignedData returns the set of signatures for the
// ConfigUpdateEnvelope as SignedData or an error indicating why this was not
// possible.
func ConfigUpdateEnvelopeAsSignedData(ce *common.ConfigUpdateEnvelope) ([]*SignedData, error) {
	if ce == nil {
		return nil, fmt.Errorf("No signatures for nil SignedConfigItem")
	}

	result := make([]*SignedData, len(ce.Signatures))
	for i, configSig := range ce.Signatures {
		sigHeader := &common.SignatureHeader{}
		err := proto.Unmarshal(configSig.SignatureHeader, sigHeader)
		if err != nil {
			return nil, err
		}

		result[i] = &SignedData{
			Data:      bytes.Join([][]byte{configSig.SignatureHeader, ce.ConfigUpdate}, nil),
			Identity:  sigHeader.Creator,
			Signature: configSig.Signature,
		}

	}

	return result, nil
}

// EnvelopeAsSignedData returns the signatures for the Envelope as SignedData
// slice of length 1 or an error indicating why this was not possible.
func EnvelopeAsSignedData(env *common.Envelope) ([]*SignedData, error) {
	if env == nil {
		return nil, fmt.Errorf("No signatures for nil Envelope")
	}

	payload := &common.Payload{}
	err := proto.Unmarshal(env.Payload, payload)
	if err != nil {
		return nil, err
	}

	if payload.Header == nil /* || payload.Header.SignatureHeader == nil */ {
		return nil, fmt.Errorf("Missing Header")
	}

	shdr := &common.SignatureHeader{}
	err = proto.Unmarshal(payload.Header.SignatureHeader, shdr)
	if err != nil {
		return nil, fmt.Errorf("GetSignatureHeaderFromBytes failed, err %s", err)
	}

	return []*SignedData{{
		Data:      env.Payload,
		Identity:  shdr.Creator,
		Signature: env.Signature,
	}}, nil
}

// LogMessageForSerializedIdentity returns a string with seriealized identity information,
// or a string indicating why the serialized identity information cannot be returned.
// Any errors are intentially returned in the return strings so that the function can be used in single-line log messages with minimal clutter.
func LogMessageForSerializedIdentity(serializedIdentity []byte) string {
	id := &msp.SerializedIdentity{}
	err := proto.Unmarshal(serializedIdentity, id)
	if err != nil {
		return fmt.Sprintf("Could not unmarshal serialized identity: %s", err)
	}
	pemBlock, _ := pem.Decode(id.IdBytes)
	if pemBlock == nil {
		// not all identities are certificates so simply log the serialized
		// identity bytes
		return fmt.Sprintf("serialized-identity=%x", serializedIdentity)
	}
	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return fmt.Sprintf("Could not parse certificate: %s", err)
	}
	return fmt.Sprintf("(mspid=%s subject=%s issuer=%s serialnumber=%d)", id.Mspid, cert.Subject, cert.Issuer, cert.SerialNumber)
}

func LogMessageForSerializedIdentities(signedData []*SignedData) (logMsg string) {
	var identityMessages []string
	for _, sd := range signedData {
		identityMessages = append(identityMessages, LogMessageForSerializedIdentity(sd.Identity))
	}
	return strings.Join(identityMessages, ", ")
}
