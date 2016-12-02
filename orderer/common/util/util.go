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
	"fmt"
	"time"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// MarshalOrPanic serializes a protobuf message and panics if this operation fails.
func MarshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return data
}

// Marshal serializes a protobuf message.
func Marshal(pb proto.Message) ([]byte, error) {
	return proto.Marshal(pb)
}

// CreateNonceOrPanic generates a nonce using the crypto/primitives package
// and panics if this operation fails.
func CreateNonceOrPanic() []byte {
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		panic(fmt.Errorf("Cannot generate random nonce: %s", err))
	}
	return nonce
}

// CreateNonce generates a nonce using the crypto/primitives package.
func CreateNonce() ([]byte, error) {
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		return nil, fmt.Errorf("Cannot generate random nonce: %s", err)
	}
	return nonce, nil
}

// ExtractEnvelopeOrPanic retrieves the requested envelope from a given block and unmarshals it -- it panics if either of these operation fail.
func ExtractEnvelopeOrPanic(block *cb.Block, index int) *cb.Envelope {
	envelopeCount := len(block.Data.Data)
	if index < 0 || index >= envelopeCount {
		panic("Envelope index out of bounds")
	}
	marshaledEnvelope := block.Data.Data[index]
	envelope := &cb.Envelope{}
	if err := proto.Unmarshal(marshaledEnvelope, envelope); err != nil {
		panic(fmt.Errorf("Block data does not carry an envelope at index %d: %s", index, err))
	}
	return envelope
}

// ExtractEnvelope retrieves the requested envelope from a given block and unmarshals it.
func ExtractEnvelope(block *cb.Block, index int) (*cb.Envelope, error) {
	envelopeCount := len(block.Data.Data)
	if index < 0 || index >= envelopeCount {
		return nil, fmt.Errorf("Envelope index out of bounds")
	}
	marshaledEnvelope := block.Data.Data[index]
	envelope := &cb.Envelope{}
	if err := proto.Unmarshal(marshaledEnvelope, envelope); err != nil {
		return nil, fmt.Errorf("Block data does not carry an envelope at index %d: %s", index, err)
	}
	return envelope, nil
}

// ExtractPayloadOrPanic retrieves the payload of a given envelope and unmarshals it -- it panics if either of these operations fail.
func ExtractPayloadOrPanic(envelope *cb.Envelope) *cb.Payload {
	payload := &cb.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		panic(fmt.Errorf("Envelope does not carry a Payload: %s", err))
	}
	return payload
}

// ExtractPayload retrieves the payload of a given envelope and unmarshals it.
func ExtractPayload(envelope *cb.Envelope) (*cb.Payload, error) {
	payload := &cb.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return nil, fmt.Errorf("Envelope does not carry a Payload: %s", err)
	}
	return payload, nil
}

// MakeChainHeader creates a ChainHeader.
func MakeChainHeader(headerType cb.HeaderType, version int32, chainID string, epoch uint64) *cb.ChainHeader {
	return &cb.ChainHeader{
		Type:    int32(headerType),
		Version: version,
		Timestamp: &timestamp.Timestamp{
			Seconds: time.Now().Unix(),
			Nanos:   0,
		},
		ChainID: chainID,
		Epoch:   epoch,
	}
}

// MakeSignatureHeader creates a SignatureHeader.
func MakeSignatureHeader(serializedCreatorCertChain []byte, nonce []byte) *cb.SignatureHeader {
	return &cb.SignatureHeader{
		Creator: serializedCreatorCertChain,
		Nonce:   nonce,
	}
}

// MakePayloadHeader creates a Payload Header.
func MakePayloadHeader(ch *cb.ChainHeader, sh *cb.SignatureHeader) *cb.Header {
	return &cb.Header{
		ChainHeader:     ch,
		SignatureHeader: sh,
	}
}

// MakeConfigurationItem makes a ConfigurationItem.
func MakeConfigurationItem(ch *cb.ChainHeader, configItemType cb.ConfigurationItem_ConfigurationType, lastModified uint64, modPolicyID string, key string, value []byte) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Header:             ch,
		Type:               configItemType,
		LastModified:       lastModified,
		ModificationPolicy: modPolicyID,
		Key:                key,
		Value:              value,
	}
}

// MakeConfigurationEnvelope makes a ConfigurationEnvelope.
func MakeConfigurationEnvelope(items ...*cb.SignedConfigurationItem) *cb.ConfigurationEnvelope {
	return &cb.ConfigurationEnvelope{Items: items}
}

// MakePolicyOrPanic creates a Policy proto message out of a SignaturePolicyEnvelope, and panics if this operation fails.
// NOTE Expand this as more policy types as supported.
func MakePolicyOrPanic(policyEnvelope interface{}) *cb.Policy {
	switch pe := policyEnvelope.(type) {
	case *cb.SignaturePolicyEnvelope:
		return &cb.Policy{
			Type: &cb.Policy_SignaturePolicy{
				SignaturePolicy: pe,
			},
		}
	default:
		panic("Unknown policy envelope type given")
	}
}

// MakePolicy creates a Policy proto message out of a SignaturePolicyEnvelope.
// NOTE Expand this as more policy types as supported.
func MakePolicy(policyEnvelope interface{}) (*cb.Policy, error) {
	switch pe := policyEnvelope.(type) {
	case *cb.SignaturePolicyEnvelope:
		return &cb.Policy{
			Type: &cb.Policy_SignaturePolicy{
				SignaturePolicy: pe,
			},
		}, nil
	default:
		return nil, fmt.Errorf("Unknown policy envelope type given")
	}
}
