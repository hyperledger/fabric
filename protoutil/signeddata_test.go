/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
)

// More duplicate utility which should go away, but the utils are a bit of a mess right now with import cycles
func marshalOrPanic(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic("Error marshaling")
	}
	return data
}

func TestNilConfigEnvelopeAsSignedData(t *testing.T) {
	var ce *common.ConfigUpdateEnvelope
	_, err := protoutil.ConfigUpdateEnvelopeAsSignedData(ce)
	if err == nil {
		t.Fatalf("Should have errored trying to convert a nil signed config item to signed data")
	}
}

func TestConfigEnvelopeAsSignedData(t *testing.T) {
	configBytes := []byte("Foo")
	signatures := [][]byte{[]byte("Signature1"), []byte("Signature2")}
	identities := [][]byte{[]byte("Identity1"), []byte("Identity2")}

	configSignatures := make([]*common.ConfigSignature, len(signatures))
	for i := range configSignatures {
		configSignatures[i] = &common.ConfigSignature{
			SignatureHeader: marshalOrPanic(&common.SignatureHeader{
				Creator: identities[i],
			}),
			Signature: signatures[i],
		}
	}

	ce := &common.ConfigUpdateEnvelope{
		ConfigUpdate: configBytes,
		Signatures:   configSignatures,
	}

	signedData, err := protoutil.ConfigUpdateEnvelopeAsSignedData(ce)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	for i, sigData := range signedData {
		if !bytes.Equal(sigData.Identity, identities[i]) {
			t.Errorf("Expected identity to match at index %d", i)
		}
		if !bytes.Equal(sigData.Data, append(configSignatures[i].SignatureHeader, configBytes...)) {
			t.Errorf("Expected signature over concatenation of config item bytes and signature header")
		}
		if !bytes.Equal(sigData.Signature, signatures[i]) {
			t.Errorf("Expected signature to match at index %d", i)
		}
	}
}

func TestNilEnvelopeAsSignedData(t *testing.T) {
	var env *common.Envelope
	_, err := protoutil.EnvelopeAsSignedData(env)
	if err == nil {
		t.Fatalf("Should have errored trying to convert a nil envelope")
	}
}

func TestEnvelopeAsSignedData(t *testing.T) {
	identity := []byte("Foo")
	sig := []byte("Bar")

	shdrbytes, err := proto.Marshal(&common.SignatureHeader{Creator: identity})
	if err != nil {
		t.Fatalf("%s", err)
	}

	env := &common.Envelope{
		Payload: marshalOrPanic(&common.Payload{
			Header: &common.Header{
				SignatureHeader: shdrbytes,
			},
		}),
		Signature: sig,
	}

	signedData, err := protoutil.EnvelopeAsSignedData(env)
	if err != nil {
		t.Fatalf("Unexpected error converting envelope to SignedData: %s", err)
	}

	if len(signedData) != 1 {
		t.Fatalf("Expected 1 entry of signed data, but got %d", len(signedData))
	}

	if !bytes.Equal(signedData[0].Identity, identity) {
		t.Errorf("Wrong identity bytes")
	}
	if !bytes.Equal(signedData[0].Data, env.Payload) {
		t.Errorf("Wrong data bytes")
	}
	if !bytes.Equal(signedData[0].Signature, sig) {
		t.Errorf("Wrong data bytes")
	}
}
