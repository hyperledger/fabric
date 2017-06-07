/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestNonceRandomness(t *testing.T) {
	n1, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	n2, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(n1, n2) {
		t.Fatalf("Expected nonces to be different, got %x and %x", n1, n2)
	}
}

func TestNonceLength(t *testing.T) {
	n, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	actual := len(n)
	expected := crypto.NonceSize
	if actual != expected {
		t.Fatalf("Expected nonce to be of size %d, got %d instead", expected, actual)
	}

}

func TestUnmarshalPayload(t *testing.T) {
	var payload *cb.Payload
	good, _ := proto.Marshal(&cb.Payload{
		Data: []byte("payload"),
	})
	payload, err := UnmarshalPayload(good)
	assert.NoError(t, err, "Unexpected error unmarshaling payload")
	assert.NotNil(t, payload, "Payload should not be nil")
	payload = UnmarshalPayloadOrPanic(good)
	assert.NotNil(t, payload, "Payload should not be nil")

	bad := []byte("bad payload")
	assert.Panics(t, func() {
		_ = UnmarshalPayloadOrPanic(bad)
	}, "Expected panic unmarshaling malformed payload")

}

func TestUnmarshalEnvelope(t *testing.T) {
	var env *cb.Envelope
	good, _ := proto.Marshal(&cb.Envelope{})
	env, err := UnmarshalEnvelope(good)
	assert.NoError(t, err, "Unexpected error unmarshaling envelope")
	assert.NotNil(t, env, "Envelope should not be nil")
	env = UnmarshalEnvelopeOrPanic(good)
	assert.NotNil(t, env, "Envelope should not be nil")

	bad := []byte("bad envelope")
	assert.Panics(t, func() {
		_ = UnmarshalEnvelopeOrPanic(bad)
	}, "Expected panic unmarshaling malformed envelope")

}

func TestUnmarshalEnvelopeOfType(t *testing.T) {
	env := &cb.Envelope{}

	env.Payload = []byte("bad payload")
	_, err := UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	assert.Error(t, err, "Expected error unmarshaling malformed envelope")

	payload, _ := proto.Marshal(&cb.Payload{
		Header: nil,
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	assert.Error(t, err, "Expected error with missing payload header")

	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: []byte("bad header"),
		},
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	assert.Error(t, err, "Expected error for malformed channel header")

	chdr, _ := proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CHAINCODE_PACKAGE),
	})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	assert.Error(t, err, "Expected error for wrong channel header type")

	chdr, _ = proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CONFIG),
	})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
		Data: []byte("bad data"),
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, &cb.ConfigEnvelope{})
	assert.Error(t, err, "Expected error for malformed payload data")

	chdr, _ = proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CONFIG),
	})
	configEnv, _ := proto.Marshal(&cb.ConfigEnvelope{})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
		Data: configEnv,
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, &cb.ConfigEnvelope{})
	assert.NoError(t, err, "Unexpected error unmarshaling envelope")

}

func TestExtractEnvelopeNilData(t *testing.T) {
	block := &cb.Block{}
	_, err := ExtractEnvelope(block, 0)
	assert.Error(t, err, "Nil data")
}

func TestExtractEnvelopeWrongIndex(t *testing.T) {
	block := testBlock()
	if _, err := ExtractEnvelope(block, len(block.GetData().Data)); err == nil {
		t.Fatal("Expected envelope extraction to fail (wrong index)")
	}
}

func TestExtractEnvelopeWrongIndexOrPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Expected envelope extraction to panic (wrong index)")
		}
	}()

	block := testBlock()
	ExtractEnvelopeOrPanic(block, len(block.GetData().Data))
}

func TestExtractEnvelope(t *testing.T) {
	if envelope, err := ExtractEnvelope(testBlock(), 0); err != nil {
		t.Fatalf("Expected envelop extraction to succeed: %s", err)
	} else if !proto.Equal(envelope, testEnvelope()) {
		t.Fatal("Expected extracted envelope to match test envelope")
	}
}

func TestExtractEnvelopeOrPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Expected envelope extraction to succeed")
		}
	}()

	if !proto.Equal(ExtractEnvelopeOrPanic(testBlock(), 0), testEnvelope()) {
		t.Fatal("Expected extracted envelope to match test envelope")
	}
}

func TestExtractPayload(t *testing.T) {
	if payload, err := ExtractPayload(testEnvelope()); err != nil {
		t.Fatalf("Expected payload extraction to succeed: %s", err)
	} else if !proto.Equal(payload, testPayload()) {
		t.Fatal("Expected extracted payload to match test payload")
	}
}

func TestExtractPayloadOrPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Expected payload extraction to succeed")
		}
	}()

	if !proto.Equal(ExtractPayloadOrPanic(testEnvelope()), testPayload()) {
		t.Fatal("Expected extracted payload to match test payload")
	}
}

func TestUnmarshalChaincodeID(t *testing.T) {
	ccname := "mychaincode"
	ccversion := "myversion"
	ccidbytes, _ := proto.Marshal(&pb.ChaincodeID{
		Name:    ccname,
		Version: ccversion,
	})
	ccid, err := UnmarshalChaincodeID(ccidbytes)
	assert.Equal(t, ccname, ccid.Name, "Expected ccid names to match")
	assert.Equal(t, ccversion, ccid.Version, "Expected ccid versions to match")

	_, err = UnmarshalChaincodeID([]byte("bad chaincodeID"))
	assert.Error(t, err, "Expected error marshaling malformed chaincode ID")
}

func TestNewSignatureHeaderOrPanic(t *testing.T) {
	var sigHeader *cb.SignatureHeader

	sigHeader = NewSignatureHeaderOrPanic(goodSigner)
	assert.NotNil(t, sigHeader, "Signature header should not be nil")

	assert.Panics(t, func() {
		_ = NewSignatureHeaderOrPanic(nil)
	}, "Expected panic with nil signer")

	assert.Panics(t, func() {
		_ = NewSignatureHeaderOrPanic(badSigner)
	}, "Expected panic with signature header error")

}

func TestSignOrPanic(t *testing.T) {
	msg := []byte("sign me")
	sig := SignOrPanic(goodSigner, msg)
	// mock signer returns message to be signed
	assert.Equal(t, msg, sig, "Signature does not match expected value")

	assert.Panics(t, func() {
		_ = SignOrPanic(nil, []byte("sign me"))
	}, "Expected panic with nil signer")

	assert.Panics(t, func() {
		_ = SignOrPanic(badSigner, []byte("sign me"))
	}, "Expected panic with sign error")
}

// Helper functions

func testPayload() *cb.Payload {
	return &cb.Payload{
		Header: MakePayloadHeader(
			MakeChannelHeader(cb.HeaderType_MESSAGE, int32(1), "test", 0),
			MakeSignatureHeader([]byte("creator"), []byte("nonce"))),
		Data: []byte("test"),
	}
}

func testEnvelope() *cb.Envelope {
	// No need to set the signature
	return &cb.Envelope{Payload: MarshalOrPanic(testPayload())}
}

func testBlock() *cb.Block {
	// No need to set the block's Header, or Metadata
	return &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{MarshalOrPanic(testEnvelope())},
		},
	}
}

// mock
var badSigner = &mockLocalSigner{
	returnError: true,
}

var goodSigner = &mockLocalSigner{
	returnError: false,
}

type mockLocalSigner struct {
	returnError bool
}

func (m *mockLocalSigner) NewSignatureHeader() (*cb.SignatureHeader, error) {
	if m.returnError {
		return nil, errors.New("signature header error")
	}
	return &cb.SignatureHeader{}, nil
}

func (m *mockLocalSigner) Sign(message []byte) ([]byte, error) {
	if m.returnError {
		return nil, errors.New("sign error")
	}
	return message, nil
}
