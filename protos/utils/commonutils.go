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
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/common/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
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

// CreateNonceOrPanic generates a nonce using the common/crypto package
// and panics if this operation fails.
func CreateNonceOrPanic() []byte {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		panic(fmt.Errorf("Cannot generate random nonce: %s", err))
	}
	return nonce
}

// CreateNonce generates a nonce using the common/crypto package.
func CreateNonce() ([]byte, error) {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, fmt.Errorf("Cannot generate random nonce: %s", err)
	}
	return nonce, nil
}

// UnmarshalPayloadOrPanic unmarshals bytes to a Payload structure or panics on error
func UnmarshalPayloadOrPanic(encoded []byte) *cb.Payload {
	payload, err := UnmarshalPayload(encoded)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling data to payload: %s", err))
	}
	return payload
}

// UnmarshalPayload unmarshals bytes to a Payload structure
func UnmarshalPayload(encoded []byte) (*cb.Payload, error) {
	payload := &cb.Payload{}
	err := proto.Unmarshal(encoded, payload)
	if err != nil {
		return nil, err
	}
	return payload, err
}

// UnmarshalEnvelopeOrPanic unmarshals bytes to an Envelope structure or panics on error
func UnmarshalEnvelopeOrPanic(encoded []byte) *cb.Envelope {
	envelope, err := UnmarshalEnvelope(encoded)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling data to envelope: %s", err))
	}
	return envelope
}

// UnmarshalEnvelope unmarshals bytes to an Envelope structure
func UnmarshalEnvelope(encoded []byte) (*cb.Envelope, error) {
	envelope := &cb.Envelope{}
	err := proto.Unmarshal(encoded, envelope)
	if err != nil {
		return nil, err
	}
	return envelope, err
}

// UnmarshalBlockOrPanic unmarshals bytes to an Block structure or panics on error
func UnmarshalBlockOrPanic(encoded []byte) *cb.Block {
	block, err := UnmarshalBlock(encoded)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling data to block: %s", err))
	}
	return block
}

// UnmarshalBlock unmarshals bytes to an Block structure
func UnmarshalBlock(encoded []byte) (*cb.Block, error) {
	block := &cb.Block{}
	err := proto.Unmarshal(encoded, block)
	if err != nil {
		return nil, err
	}
	return block, err
}

// UnmarshalEnvelopeOfType unmarshals an envelope of the specified type, including
// the unmarshaling the payload data
func UnmarshalEnvelopeOfType(envelope *cb.Envelope, headerType cb.HeaderType, message proto.Message) (*cb.ChannelHeader, error) {
	return UnmarshalEnvelopeOfTypes(envelope, []cb.HeaderType{headerType}, message)
}

// UnmarshalEnvelopeOfTypes unmarshals an envelope of the one of the specified types, including
// the unmarshaling the payload data
func UnmarshalEnvelopeOfTypes(envelope *cb.Envelope, expectedHeaderTypes []cb.HeaderType, message proto.Message) (*cb.ChannelHeader, error) {
	payload, err := UnmarshalPayload(envelope.Payload)
	if err != nil {
		return nil, err
	}

	if payload.Header == nil {
		return nil, fmt.Errorf("Envelope must have a Header")
	}

	chdr, err := UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, fmt.Errorf("Invalid ChannelHeader")
	}

	headerTypeMatched := false
	for i := 0; i < len(expectedHeaderTypes); i++ {
		if chdr.Type == int32(expectedHeaderTypes[i]) {
			headerTypeMatched = true
			break
		}
	}
	if !headerTypeMatched {
		return nil, fmt.Errorf("Not a tx of type %v", expectedHeaderTypes)
	}

	if err = proto.Unmarshal(payload.Data, message); err != nil {
		return nil, fmt.Errorf("Error unmarshaling message for type %v: %s", expectedHeaderTypes, err)
	}

	return chdr, nil
}

// ExtractEnvelopeOrPanic retrieves the requested envelope from a given block and unmarshals it -- it panics if either of these operation fail.
func ExtractEnvelopeOrPanic(block *cb.Block, index int) *cb.Envelope {
	envelope, err := ExtractEnvelope(block, index)
	if err != nil {
		panic(err)
	}
	return envelope
}

// ExtractEnvelope retrieves the requested envelope from a given block and unmarshals it.
func ExtractEnvelope(block *cb.Block, index int) (*cb.Envelope, error) {
	if block.Data == nil {
		return nil, fmt.Errorf("No data in block")
	}

	envelopeCount := len(block.Data.Data)
	if index < 0 || index >= envelopeCount {
		return nil, fmt.Errorf("Envelope index out of bounds")
	}
	marshaledEnvelope := block.Data.Data[index]
	envelope, err := GetEnvelopeFromBlock(marshaledEnvelope)
	if err != nil {
		return nil, fmt.Errorf("Block data does not carry an envelope at index %d: %s", index, err)
	}
	return envelope, nil
}

// ExtractPayloadOrPanic retrieves the payload of a given envelope and unmarshals it -- it panics if either of these operations fail.
func ExtractPayloadOrPanic(envelope *cb.Envelope) *cb.Payload {
	payload, err := ExtractPayload(envelope)
	if err != nil {
		panic(err)
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

// MakeChannelHeader creates a ChannelHeader.
func MakeChannelHeader(headerType cb.HeaderType, version int32, chainID string, epoch uint64) *cb.ChannelHeader {
	return &cb.ChannelHeader{
		Type:    int32(headerType),
		Version: version,
		Timestamp: &timestamp.Timestamp{
			Seconds: time.Now().Unix(),
			Nanos:   0,
		},
		ChannelId: chainID,
		Epoch:     epoch,
	}
}

// MakeSignatureHeader creates a SignatureHeader.
func MakeSignatureHeader(serializedCreatorCertChain []byte, nonce []byte) *cb.SignatureHeader {
	return &cb.SignatureHeader{
		Creator: serializedCreatorCertChain,
		Nonce:   nonce,
	}
}

func SetTxID(channelHeader *cb.ChannelHeader, signatureHeader *cb.SignatureHeader) error {
	txid, err := ComputeProposalTxID(
		signatureHeader.Nonce,
		signatureHeader.Creator,
	)
	if err != nil {
		return err
	}
	channelHeader.TxId = txid
	return nil
}

// MakePayloadHeader creates a Payload Header.
func MakePayloadHeader(ch *cb.ChannelHeader, sh *cb.SignatureHeader) *cb.Header {
	return &cb.Header{
		ChannelHeader:   MarshalOrPanic(ch),
		SignatureHeader: MarshalOrPanic(sh),
	}
}

// NewSignatureHeaderOrPanic returns a signature header and panics on error.
func NewSignatureHeaderOrPanic(signer crypto.LocalSigner) *cb.SignatureHeader {
	if signer == nil {
		panic(errors.New("Invalid signer. Must be different from nil."))
	}

	signatureHeader, err := signer.NewSignatureHeader()
	if err != nil {
		panic(fmt.Errorf("Failed generating a new SignatureHeader [%s]", err))
	}
	return signatureHeader
}

// SignOrPanic signs a message and panics on error.
func SignOrPanic(signer crypto.LocalSigner, msg []byte) []byte {
	if signer == nil {
		panic(errors.New("Invalid signer. Must be different from nil."))
	}

	sigma, err := signer.Sign(msg)
	if err != nil {
		panic(fmt.Errorf("Failed generting signature [%s]", err))
	}
	return sigma
}

// UnmarshalChannelHeader returns a ChannelHeader from bytes
func UnmarshalChannelHeader(bytes []byte) (*cb.ChannelHeader, error) {
	chdr := &cb.ChannelHeader{}
	err := proto.Unmarshal(bytes, chdr)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalChannelHeader failed, err %s", err)
	}

	return chdr, nil
}

// UnmarshalChannelHeaderOrPanic unmarshals bytes to a ChannelHeader or panics on error
func UnmarshalChannelHeaderOrPanic(bytes []byte) *cb.ChannelHeader {
	chdr := &cb.ChannelHeader{}
	err := proto.Unmarshal(bytes, chdr)
	if err != nil {
		panic(fmt.Errorf("UnmarshalChannelHeader failed, err %s", err))
	}
	return chdr
}

// UnmarshalChaincodeID returns a ChaincodeID from bytes
func UnmarshalChaincodeID(bytes []byte) (*pb.ChaincodeID, error) {
	ccid := &pb.ChaincodeID{}
	err := proto.Unmarshal(bytes, ccid)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalChaincodeID failed, err %s", err)
	}

	return ccid, nil
}

// IsConfigBlock validates whenever given block contains configuration
// update transaction
func IsConfigBlock(block *cb.Block) bool {
	envelope, err := ExtractEnvelope(block, 0)
	if err != nil {
		return false
	}

	payload, err := GetPayload(envelope)
	if err != nil {
		return false
	}

	if payload.Header == nil {
		return false
	}

	hdr, err := UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return false
	}

	return cb.HeaderType(hdr.Type) == cb.HeaderType_CONFIG
}

// ChannelHeader returns the *cb.ChannelHeader for a given *cb.Envelope.
func ChannelHeader(env *cb.Envelope) (*cb.ChannelHeader, error) {
	envPayload, err := UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("payload unmarshaling error: %s", err)
	}

	if envPayload.Header == nil {
		return nil, fmt.Errorf("no header was set")
	}

	if envPayload.Header.ChannelHeader == nil {
		return nil, fmt.Errorf("no channel header was set")
	}

	chdr, err := UnmarshalChannelHeader(envPayload.Header.ChannelHeader)
	if err != nil {
		return nil, fmt.Errorf("channel header unmarshaling error: %s", err)
	}

	return chdr, nil
}

// ChannelID returns the Channel ID for a given *cb.Envelope.
func ChannelID(env *cb.Envelope) (string, error) {
	chdr, err := ChannelHeader(env)
	if err != nil {
		return "", fmt.Errorf("channel header unmarshaling error: %s", err)
	}

	return chdr.ChannelId, nil
}
