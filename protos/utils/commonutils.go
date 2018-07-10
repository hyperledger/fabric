/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/common/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

// MarshalOrPanic serializes a protobuf message and panics if this
// operation fails
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
	nonce, err := CreateNonce()
	if err != nil {
		panic(err)
	}
	return nonce
}

// CreateNonce generates a nonce using the common/crypto package.
func CreateNonce() ([]byte, error) {
	nonce, err := crypto.GetRandomNonce()
	return nonce, errors.WithMessage(err, "error generating random nonce")
}

// UnmarshalPayloadOrPanic unmarshals bytes to a Payload structure or panics
// on error
func UnmarshalPayloadOrPanic(encoded []byte) *cb.Payload {
	payload, err := UnmarshalPayload(encoded)
	if err != nil {
		panic(err)
	}
	return payload
}

// UnmarshalPayload unmarshals bytes to a Payload structure
func UnmarshalPayload(encoded []byte) (*cb.Payload, error) {
	payload := &cb.Payload{}
	err := proto.Unmarshal(encoded, payload)
	return payload, errors.Wrap(err, "error unmarshaling Payload")
}

// UnmarshalEnvelopeOrPanic unmarshals bytes to an Envelope structure or panics
// on error
func UnmarshalEnvelopeOrPanic(encoded []byte) *cb.Envelope {
	envelope, err := UnmarshalEnvelope(encoded)
	if err != nil {
		panic(err)
	}
	return envelope
}

// UnmarshalEnvelope unmarshals bytes to an Envelope structure
func UnmarshalEnvelope(encoded []byte) (*cb.Envelope, error) {
	envelope := &cb.Envelope{}
	err := proto.Unmarshal(encoded, envelope)
	return envelope, errors.Wrap(err, "error unmarshaling Envelope")
}

// UnmarshalBlockOrPanic unmarshals bytes to an Block structure or panics
// on error
func UnmarshalBlockOrPanic(encoded []byte) *cb.Block {
	block, err := UnmarshalBlock(encoded)
	if err != nil {
		panic(err)
	}
	return block
}

// UnmarshalBlock unmarshals bytes to an Block structure
func UnmarshalBlock(encoded []byte) (*cb.Block, error) {
	block := &cb.Block{}
	err := proto.Unmarshal(encoded, block)
	return block, errors.Wrap(err, "error unmarshaling Block")
}

// UnmarshalEnvelopeOfType unmarshals an envelope of the specified type,
// including unmarshaling the payload data
func UnmarshalEnvelopeOfType(envelope *cb.Envelope, headerType cb.HeaderType, message proto.Message) (*cb.ChannelHeader, error) {
	payload, err := UnmarshalPayload(envelope.Payload)
	if err != nil {
		return nil, err
	}

	if payload.Header == nil {
		return nil, errors.New("envelope must have a Header")
	}

	chdr, err := UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, err
	}

	if chdr.Type != int32(headerType) {
		return nil, errors.Errorf("invalid type %s, expected %s", cb.HeaderType(chdr.Type), headerType)
	}

	err = proto.Unmarshal(payload.Data, message)
	err = errors.Wrapf(err, "error unmarshaling message for type %s", headerType)
	return chdr, err
}

// ExtractEnvelopeOrPanic retrieves the requested envelope from a given block
// and unmarshals it -- it panics if either of these operations fail
func ExtractEnvelopeOrPanic(block *cb.Block, index int) *cb.Envelope {
	envelope, err := ExtractEnvelope(block, index)
	if err != nil {
		panic(err)
	}
	return envelope
}

// ExtractEnvelope retrieves the requested envelope from a given block and
// unmarshals it
func ExtractEnvelope(block *cb.Block, index int) (*cb.Envelope, error) {
	if block.Data == nil {
		return nil, errors.New("block data is nil")
	}

	envelopeCount := len(block.Data.Data)
	if index < 0 || index >= envelopeCount {
		return nil, errors.New("envelope index out of bounds")
	}
	marshaledEnvelope := block.Data.Data[index]
	envelope, err := GetEnvelopeFromBlock(marshaledEnvelope)
	err = errors.WithMessage(err, fmt.Sprintf("block data does not carry an envelope at index %d", index))
	return envelope, err
}

// ExtractPayloadOrPanic retrieves the payload of a given envelope and
// unmarshals it -- it panics if either of these operations fail
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
	err := proto.Unmarshal(envelope.Payload, payload)
	err = errors.Wrap(err, "no payload in envelope")
	return payload, err
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

// SetTxID generates a transaction id based on the provided signature header
// and sets the TxId field in the channel header
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
		panic(errors.New("invalid signer. cannot be nil"))
	}

	signatureHeader, err := signer.NewSignatureHeader()
	if err != nil {
		panic(fmt.Errorf("failed generating a new SignatureHeader: %s", err))
	}
	return signatureHeader
}

// SignOrPanic signs a message and panics on error.
func SignOrPanic(signer crypto.LocalSigner, msg []byte) []byte {
	if signer == nil {
		panic(errors.New("invalid signer. cannot be nil"))
	}

	sigma, err := signer.Sign(msg)
	if err != nil {
		panic(fmt.Errorf("failed generating signature: %s", err))
	}
	return sigma
}

// UnmarshalChannelHeader returns a ChannelHeader from bytes
func UnmarshalChannelHeader(bytes []byte) (*cb.ChannelHeader, error) {
	chdr := &cb.ChannelHeader{}
	err := proto.Unmarshal(bytes, chdr)
	return chdr, errors.Wrap(err, "error unmarshaling ChannelHeader")
}

// UnmarshalChannelHeaderOrPanic unmarshals bytes to a ChannelHeader or panics
// on error
func UnmarshalChannelHeaderOrPanic(bytes []byte) *cb.ChannelHeader {
	chdr, err := UnmarshalChannelHeader(bytes)
	if err != nil {
		panic(err)
	}
	return chdr
}

// UnmarshalChaincodeID returns a ChaincodeID from bytes
func UnmarshalChaincodeID(bytes []byte) (*pb.ChaincodeID, error) {
	ccid := &pb.ChaincodeID{}
	err := proto.Unmarshal(bytes, ccid)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling ChaincodeID")
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
		return nil, err
	}

	if envPayload.Header == nil {
		return nil, errors.New("header not set")
	}

	if envPayload.Header.ChannelHeader == nil {
		return nil, errors.New("channel header not set")
	}

	chdr, err := UnmarshalChannelHeader(envPayload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshaling channel header")
	}

	return chdr, nil
}

// ChannelID returns the Channel ID for a given *cb.Envelope.
func ChannelID(env *cb.Envelope) (string, error) {
	chdr, err := ChannelHeader(env)
	if err != nil {
		return "", errors.WithMessage(err, "error retrieving channel header")
	}

	return chdr.ChannelId, nil
}
