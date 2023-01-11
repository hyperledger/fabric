/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/asn1"
	"math/big"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
)

// Signature implementation
type Signature struct {
	IdentifierHeader     []byte
	BlockHeader          []byte
	OrdererBlockMetadata []byte
}

// Unmarshal the signature
func (sig *Signature) Unmarshal(bytes []byte) error {
	_, err := asn1.Unmarshal(bytes, sig)
	return err
}

// Marshal the signature
func (sig *Signature) Marshal() []byte {
	bytes, err := asn1.Marshal(*sig)
	if err != nil {
		panic(err)
	}
	return bytes
}

// AsBytes returns the message to sign
func (sig Signature) AsBytes() []byte {
	msg2Sign := util.ConcatenateBytes(sig.OrdererBlockMetadata, sig.IdentifierHeader, sig.BlockHeader)
	return msg2Sign
}

// ProposalToBlock marshals the proposal the the block
func ProposalToBlock(proposal types.Proposal) (*cb.Block, error) {
	// initialize block with empty fields
	block := &cb.Block{
		Data:     &cb.BlockData{},
		Metadata: &cb.BlockMetadata{},
	}

	if len(proposal.Header) == 0 {
		return nil, errors.New("proposal header cannot be nil")
	}

	hdr := &asn1Header{}

	if _, err := asn1.Unmarshal(proposal.Header, hdr); err != nil {
		return nil, errors.Wrap(err, "bad header")
	}

	block.Header = &cb.BlockHeader{
		Number:       hdr.Number.Uint64(),
		PreviousHash: hdr.PreviousHash,
		DataHash:     hdr.DataHash,
	}

	if len(proposal.Payload) == 0 {
		return nil, errors.New("proposal payload cannot be nil")
	}

	tuple := &ByteBufferTuple{}
	if err := tuple.FromBytes(proposal.Payload); err != nil {
		return nil, errors.Wrap(err, "bad payload and metadata tuple")
	}

	if err := proto.Unmarshal(tuple.A, block.Data); err != nil {
		return nil, errors.Wrap(err, "bad payload")
	}

	if err := proto.Unmarshal(tuple.B, block.Metadata); err != nil {
		return nil, errors.Wrap(err, "bad metadata")
	}
	return block, nil
}

type asn1Header struct {
	Number       *big.Int
	PreviousHash []byte
	DataHash     []byte
}
