/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
)

//go:generate mockery --dir . --name SignerSerializer --case underscore --with-expecter=true --output mocks

// SignerSerializer signs messages and serializes identities
type SignerSerializer interface {
	identity.SignerSerializer
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
func (s *Signer) SignProposal(proposal types.Proposal, _ []byte) *types.Signature {
	block, err := ProposalToBlock(proposal)
	if err != nil {
		s.Logger.Panicf("Tried to sign bad proposal: %v", err)
	}

	nonce := randomNonceOrPanic()

	sig := &Signature{
		BlockHeader:      protoutil.BlockHeaderBytes(block.Header),
		IdentifierHeader: protoutil.MarshalOrPanic(s.newIdentifierHeaderOrPanic(nonce)),
		OrdererBlockMetadata: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig:        &cb.LastConfig{Index: s.LastConfigBlockNum(block)},
			ConsenterMetadata: proposal.Metadata,
		}),
	}

	signature := protoutil.SignOrPanic(s.SignerSerializer, sig.AsBytes())

	return &types.Signature{
		ID:    s.ID,
		Value: signature,
		Msg:   sig.Marshal(),
	}
}

// newIdentifierHeaderOrPanic creates an IdentifierHeader with the signer's identifier and a valid nonce
func (s *Signer) newIdentifierHeaderOrPanic(nonce []byte) *cb.IdentifierHeader {
	return &cb.IdentifierHeader{
		Identifier: uint32(s.ID),
		Nonce:      nonce,
	}
}

func randomNonceOrPanic() []byte {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		panic(err)
	}
	return nonce
}
