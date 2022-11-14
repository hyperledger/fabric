/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"github.com/SmartBFT-Go/consensus/pkg/types"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/protoutil"
)

//go:generate mockery -dir . -name SignerSerializer -case underscore -output ./mocks/

// SignerSerializer signs messages and serializes identities
type SignerSerializer interface {
	crypto.Signer
	crypto.IdentitySerializer
}

// Signer implementation
type Signer struct {
	ID                 uint64
	SignerSerializer   SignerSerializer
	Logger             Logger
	LastConfigBlockNum func(*cb.Block) uint64
}

// Sign signs the message
func (s *Signer) Sign(msg []byte) []byte {
	signature, err := s.SignerSerializer.Sign(msg)
	if err != nil {
		s.Logger.Panicf("Failed signing message: %v", err)
	}
	return signature
}

// SignProposal signs the proposal
func (s *Signer) SignProposal(proposal types.Proposal, auxiliaryInput []byte) *types.Signature {
	block, err := ProposalToBlock(proposal)
	if err != nil {
		s.Logger.Panicf("Tried to sign bad proposal: %v", err)
	}

	nonce := randomNonceOrPanic()

	sig := Signature{
		AuxiliaryInput:  auxiliaryInput,
		Nonce:           nonce,
		BlockHeader:     protoutil.BlockHeaderBytes(block.Header),
		SignatureHeader: protoutil.MarshalOrPanic(s.newSignatureHeaderOrPanic(nonce)),
		OrdererBlockMetadata: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig:        &cb.LastConfig{Index: uint64(s.LastConfigBlockNum(block))},
			ConsenterMetadata: proposal.Metadata,
		}),
	}

	signature := protoutil.SignOrPanic(s.SignerSerializer, sig.AsBytes())

	// Nil out the signature header after creating the signature
	sig.SignatureHeader = nil

	return &types.Signature{
		ID:    s.ID,
		Value: signature,
		Msg:   sig.Marshal(),
	}
}

// NewSignatureHeader creates a SignatureHeader with the correct signing identity and a valid nonce
func (s *Signer) newSignatureHeaderOrPanic(nonce []byte) *cb.SignatureHeader {
	creator, err := s.SignerSerializer.Serialize()
	if err != nil {
		panic(err)
	}

	return &cb.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}
}

func randomNonceOrPanic() []byte {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		panic(err)
	}
	return nonce
}
