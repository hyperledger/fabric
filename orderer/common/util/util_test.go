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

package util

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
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
	expected := primitives.NonceSize
	if actual != expected {
		t.Fatalf("Expected nonce to be of size %d, got %d instead", expected, actual)
	}

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

// Helper functions

func testPayload() *cb.Payload {
	return &cb.Payload{
		Header: MakePayloadHeader(MakeChainHeader(cb.HeaderType_MESSAGE, int32(1), "test", 0), nil),
		Data:   []byte("test"),
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
