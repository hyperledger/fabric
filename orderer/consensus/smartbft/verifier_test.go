/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/SmartBFT/pkg/types"
	"github.com/hyperledger/SmartBFT/smartbftprotos"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var hashOfZero = hex.EncodeToString(sha256.New().Sum(nil))

func TestNodeIdentitiesByID(t *testing.T) {
	m := make(smartbft.NodeIdentitiesByID)
	for id := range uint64(4) {
		m[id] = protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			IdBytes: fmt.Appendf(nil, "%d", id),
			Mspid:   "OrdererOrg",
		})

		sID := &msp.SerializedIdentity{}
		err := proto.Unmarshal(m[id], sID)
		require.NoError(t, err)

		id2, ok := m.IdentityToID(m[id])
		require.True(t, ok)
		require.Equal(t, id, id2)
	}

	_, ok := m.IdentityToID(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
		IdBytes: fmt.Appendf(nil, "%d", 4),
		Mspid:   "OrdererOrg",
	}))

	require.False(t, ok)

	_, ok = m.IdentityToID([]byte{1, 2, 3})
	require.False(t, ok)
}

func TestVerifySignature(t *testing.T) {
	logger := flogging.MustGetLogger("test")

	cv := &mocks.ConsenterVerifier{}
	cv.On("Evaluate", mock.Anything).Return(errors.New("bad signature"))

	v := &smartbft.Verifier{
		Logger:            logger,
		ConsenterVerifier: cv,
		RuntimeConfig:     &atomic.Value{},
	}

	rtc := smartbft.RuntimeConfig{
		ID2Identities: map[uint64][]byte{3: {0, 2, 4, 6}},
	}
	v.RuntimeConfig.Store(rtc)

	t.Run("identity doesn't exist", func(t *testing.T) {
		err := v.VerifySignature(types.Signature{
			ID: 2,
		})
		require.EqualError(t, err, "node with id of 2 doesn't exist")
	})

	t.Run("signature doesn't verify", func(t *testing.T) {
		err := v.VerifySignature(types.Signature{
			ID: 3,
		})
		require.EqualError(t, err, "bad signature")
	})
}

func TestVerifyConsenterSig(t *testing.T) {
	logger := flogging.MustGetLogger("test")

	lastBlock := makeNonConfigBlock(19, 10)
	lastConfigBlock := makeConfigBlock(10)

	ac := &mocks.AccessController{}
	ac.On("Evaluate", mock.Anything).Return(nil)

	ledger := &mocks.Ledger{}
	ledger.On("Height").Return(uint64(20))
	ledger.On("Block", uint64(19)).Return(lastBlock)
	ledger.On("Block", uint64(10)).Return(lastConfigBlock)

	sequencer := &mocks.Sequencer{}
	sequencer.On("Sequence").Return(uint64(12))

	reqInspector := &smartbft.RequestInspector{
		ValidateIdentityStructure: func(_ *msp.SerializedIdentity) error {
			return nil
		},
		Logger: logger,
	}

	cv := &mocks.ConsenterVerifier{}
	cv.On("Evaluate", mock.Anything).Return(errors.New("bad signature"))

	lastHash := hex.EncodeToString(protoutil.BlockHeaderHash(lastBlock.GetHeader()))

	for _, testCase := range []struct {
		description                 string
		verificationSequence        uint64
		lastBlock                   *cb.Block
		id2Identity                 map[uint64][]byte
		signatureMutator            func(types.Signature) types.Signature
		lastConfigBlockNum          uint64
		bftMetadataMutator          func([]byte) []byte
		proposalMutator             func(proposal types.Proposal) types.Proposal
		ordererBlockMetadataMutator func(metadata *cb.OrdererBlockMetadata)
		expectedErr                 string
		expectedValidationTime      bool
	}{
		{
			description:            "timestamp is passed to the consenter verifier",
			expectedErr:            "bad signature",
			expectedValidationTime: true,
			lastBlock:              lastBlock,
			lastConfigBlockNum:     lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:            map[uint64][]byte{3: {0, 2, 4, 6}},
		},
		{
			description:        "malformed timestamp nonce is rejected",
			expectedErr:        "malformed SmartBFT consenter signature timestamp: malformed Pactway block signature nonce",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				return signatureWithIdentifierNonce(signature, []byte("PWBTSIG1"))
			},
		},
		{
			description:        "invalid timestamp nanoseconds are rejected",
			expectedErr:        "malformed SmartBFT consenter signature timestamp: invalid nanoseconds in Pactway block signature nonce: 1000000000",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				nonce := protoutil.BlockSignatureNonceWithTimestamp([]byte("nonce"), time.Now())
				binary.BigEndian.PutUint32(nonce[16:20], 1_000_000_000)
				return signatureWithIdentifierNonce(signature, nonce)
			},
		},
		{
			description:        "No consenter in mapping",
			expectedErr:        "node with id of 3 doesn't exist",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
		},
		{
			description: "Bad signature format",
			expectedErr: "malformed signature format: asn1: structure error: tags don't match (16 vs " +
				"{class:0 tag:1 length:2 isCompound:false}) {optional:false explicit:false application:" +
				"false private:false defaultValue:<nil> tag:<nil> stringType:0 timeType:0 set:false omitEmpty:false} Signature @2",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				return types.Signature{
					ID:    signature.ID,
					Value: signature.Value,
					Msg:   []byte{1, 2, 3},
				}
			},
		},
		{
			description:        "metadata doesn't match proposal",
			expectedErr:        "consenter metadata in OrdererBlockMetadata doesn't match proposal",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				sig := smartbft.Signature{}
				_ = sig.Unmarshal(signature.Msg)
				sig.OrdererBlockMetadata = nil
				return types.Signature{
					ID:    signature.ID,
					Value: signature.Value,
					Msg:   sig.Marshal(),
				}
			},
		},
		{
			description:        "block header doesn't match proposal",
			expectedErr:        "mismatched block header",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				sig := smartbft.Signature{}
				_ = sig.Unmarshal(signature.Msg)
				sig.BlockHeader = nil
				return types.Signature{
					ID:    signature.ID,
					Value: signature.Value,
					Msg:   sig.Marshal(),
				}
			},
		},
		{
			description:        "nonce different than what was used for signing",
			expectedErr:        "bad signature",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				sig := smartbft.Signature{}
				_ = sig.Unmarshal(signature.Msg)
				// sig.Nonce = nil
				return types.Signature{
					ID:    signature.ID,
					Value: signature.Value,
					Msg:   sig.Marshal(),
				}
			},
		},
		{
			description:        "orderer block metadata is malformed",
			expectedErr:        "malformed orderer metadata in signature: proto: cannot parse invalid wire-format data",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			signatureMutator: func(signature types.Signature) types.Signature {
				sig := smartbft.Signature{}
				_ = sig.Unmarshal(signature.Msg)
				sig.OrdererBlockMetadata = []byte{1, 2, 3}
				return types.Signature{
					ID:    signature.ID,
					Value: signature.Value,
					Msg:   sig.Marshal(),
				}
			},
		},
		{
			description:        "signature doesn't verify",
			expectedErr:        "bad signature",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
		},
		{
			description: "malformed proposal",
			expectedErr: "bad payload and metadata tuple: asn1: structure error: " +
				"tags don't match (16 vs {class:0 tag:1 length:2 isCompound:false}) " +
				"{optional:false explicit:false application:false private:false " +
				"defaultValue:<nil> tag:<nil> stringType:0 timeType:0 set:false " +
				"omitEmpty:false} ByteBufferTuple @2",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				proposal.Payload = []byte{1, 2, 3}
				return proposal
			},
		},
		{
			description:        "empty proposal payload",
			expectedErr:        "proposal payload cannot be nil",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				proposal.Payload = nil
				return proposal
			},
		},
		{
			description:        "metadata too short",
			expectedErr:        "block metadata is of size 4 but should be of size 5",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				block, _ := smartbft.ProposalToBlock(proposal)
				block.Metadata.Metadata = make([][]byte, len(cb.BlockMetadataIndex_name)-1)
				bbt := &smartbft.ByteBufferTuple{}
				_ = bbt.FromBytes(proposal.Payload)
				bbt.B = protoutil.MarshalOrPanic(block.GetMetadata())
				proposal.Payload = bbt.ToBytes()
				return proposal
			},
		},
		{
			description:        "malformed signature metadata",
			expectedErr:        "malformed signature metadata: proto: cannot parse invalid wire-format data",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				block, _ := smartbft.ProposalToBlock(proposal)
				block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = []byte{1, 2, 3}
				bbt := &smartbft.ByteBufferTuple{}
				_ = bbt.FromBytes(proposal.Payload)
				bbt.B = protoutil.MarshalOrPanic(block.GetMetadata())
				proposal.Payload = bbt.ToBytes()
				return proposal
			},
		},
		{
			description:        "malformed OrdererBlockMetadata",
			expectedErr:        "malformed orderer metadata in block: proto: cannot parse invalid wire-format data",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				block, _ := smartbft.ProposalToBlock(proposal)
				md := &cb.Metadata{}
				_ = proto.Unmarshal(block.GetMetadata().GetMetadata()[cb.BlockMetadataIndex_SIGNATURES], md)
				md.Value = []byte{1, 2, 3}
				block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(md)
				bbt := &smartbft.ByteBufferTuple{}
				_ = bbt.FromBytes(proposal.Payload)
				bbt.B = protoutil.MarshalOrPanic(block.GetMetadata())
				proposal.Payload = bbt.ToBytes()
				return proposal
			},
		},
		{
			description:        "mismatched OrdererBlockMetadata",
			expectedErr:        "signature's OrdererBlockMetadata and OrdererBlockMetadata extracted from block do not match",
			lastBlock:          lastBlock,
			lastConfigBlockNum: lastConfigBlock.GetHeader().GetNumber(),
			id2Identity:        map[uint64][]byte{3: {0, 2, 4, 6}},
			proposalMutator: func(proposal types.Proposal) types.Proposal {
				block, _ := smartbft.ProposalToBlock(proposal)
				md := &cb.Metadata{}
				_ = proto.Unmarshal(block.GetMetadata().GetMetadata()[cb.BlockMetadataIndex_SIGNATURES], md)
				obm := &cb.OrdererBlockMetadata{}
				_ = proto.Unmarshal(md.GetValue(), obm)
				obm.LastConfig.Index++
				md.Value = protoutil.MarshalOrPanic(obm)
				block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(md)
				bbt := &smartbft.ByteBufferTuple{}
				_ = bbt.FromBytes(proposal.Payload)
				bbt.B = protoutil.MarshalOrPanic(block.GetMetadata())
				proposal.Payload = bbt.ToBytes()
				return proposal
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			ss := &mocks.SignerSerializer{}
			ss.On("Sign", mock.Anything).Return([]byte{1, 2, 3}, nil).Once()
			ss.On("Serialize", mock.Anything).Return([]byte{0, 2, 4, 6}, nil)

			s := &smartbft.Signer{
				LastConfigBlockNum: func(_ *cb.Block) uint64 {
					return lastConfigBlock.GetHeader().GetNumber()
				},
				SignerSerializer: ss,
				Logger:           flogging.MustGetLogger("test"),
				ID:               3,
			}

			rtc := smartbft.RuntimeConfig{
				ID2Identities:          testCase.id2Identity,
				LastBlock:              testCase.lastBlock,
				LastConfigBlock:        lastConfigBlock,
				LastCommittedBlockHash: lastHash,
			}
			runtimeConfig := &atomic.Value{}
			runtimeConfig.Store(rtc)

			assembler := &smartbft.Assembler{
				VerificationSeq: func() uint64 {
					return testCase.verificationSequence
				},
				Logger:        logger,
				RuntimeConfig: runtimeConfig,
			}

			md := protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{
				LatestSequence: 1,
				ViewId:         2,
			})

			proposal := assembler.AssembleProposal(md, [][]byte{nonConfigTx})

			signature := *s.SignProposal(proposal, nil)
			if testCase.signatureMutator != nil {
				signature = testCase.signatureMutator(signature)
			}

			v := &smartbft.Verifier{
				RuntimeConfig:         runtimeConfig,
				Logger:                logger,
				Ledger:                ledger,
				VerificationSequencer: sequencer,
				AccessController:      ac,
				ConsenterVerifier:     cv,
				ReqInspector:          reqInspector,
				ValidateAdmission:     func(*cb.Envelope) error { return nil },
			}

			if testCase.proposalMutator != nil {
				proposal = testCase.proposalMutator(proposal)
			}

			_, err := v.VerifyConsenterSig(signature, proposal)

			require.Error(t, err)
			require.Equal(t, testCase.expectedErr, strings.ReplaceAll(err.Error(), "\u00a0", " "))
			_, liveErr := v.VerifyLiveConsenterSig(signature, proposal)
			require.Error(t, liveErr)
			require.Equal(t, testCase.expectedErr, strings.ReplaceAll(liveErr.Error(), "\u00a0", " "))
			if testCase.expectedValidationTime {
				signedData := cv.Calls[len(cv.Calls)-1].Arguments.Get(0).([]*protoutil.SignedData)
				require.Len(t, signedData, 1)
				require.NotNil(t, signedData[0].ValidationTime)
				require.WithinDuration(t, time.Now(), *signedData[0].ValidationTime, time.Minute)
			}
		})
	}
}

func signatureWithIdentifierNonce(signature types.Signature, nonce []byte) types.Signature {
	sig := &smartbft.Signature{}
	if err := sig.Unmarshal(signature.Msg); err != nil {
		panic(err)
	}
	identifierHeader, err := protoutil.UnmarshalIdentifierHeader(sig.IdentifierHeader)
	if err != nil {
		panic(err)
	}
	identifierHeader.Nonce = nonce
	sig.IdentifierHeader = protoutil.MarshalOrPanic(identifierHeader)
	signature.Msg = sig.Marshal()
	return signature
}

func TestVerifyLiveConsenterSig(t *testing.T) {
	lastBlock := makeNonConfigBlock(19, 10)
	lastConfigBlock := makeConfigBlock(10)
	runtimeConfig := &atomic.Value{}
	runtimeConfig.Store(smartbft.RuntimeConfig{
		ID2Identities:   map[uint64][]byte{3: []byte("consenter")},
		LastBlock:       lastBlock,
		LastConfigBlock: lastConfigBlock,
	})
	assembler := &smartbft.Assembler{
		VerificationSeq: func() uint64 { return 12 },
		Logger:          flogging.MustGetLogger("test"),
		RuntimeConfig:   runtimeConfig,
	}
	proposal := assembler.AssembleProposal(protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{
		LatestSequence: 1, ViewId: 2,
	}), [][]byte{nonConfigTx})
	ss := &mocks.SignerSerializer{}
	ss.On("Sign", mock.Anything).Return([]byte("signature"), nil)
	signer := &smartbft.Signer{
		LastConfigBlockNum: func(*cb.Block) uint64 { return 10 },
		SignerSerializer:   ss,
		Logger:             flogging.MustGetLogger("test"),
		ID:                 3,
	}
	signingTime := time.Now().Add(-2 * time.Hour)
	signature := signatureWithIdentifierNonce(*signer.SignProposal(proposal, nil),
		protoutil.BlockSignatureNonceWithTimestamp([]byte("nonce"), signingTime))

	for _, tc := range []struct {
		name          string
		historicalErr error
		currentErr    error
	}{
		{name: "valid at both times"},
		{name: "backdated vote from expired consenter", currentErr: errors.New("certificate has expired")},
		{name: "timestamp outside certificate validity", historicalErr: errors.New("invalid signing time")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cv := &mocks.ConsenterVerifier{}
			historical := mock.MatchedBy(func(data []*protoutil.SignedData) bool {
				return len(data) == 1 && data[0].ValidationTime != nil && data[0].ValidationTime.Equal(signingTime)
			})
			current := mock.MatchedBy(func(data []*protoutil.SignedData) bool {
				return len(data) == 1 && data[0].ValidationTime == nil
			})
			cv.On("Evaluate", historical).Return(tc.historicalErr).Twice()
			if tc.historicalErr == nil {
				cv.On("Evaluate", current).Return(tc.currentErr).Once()
			}
			verifier := &smartbft.Verifier{
				RuntimeConfig: runtimeConfig, Logger: flogging.MustGetLogger("test"), ConsenterVerifier: cv,
			}
			_, err := verifier.VerifyConsenterSig(signature, proposal)
			require.Equal(t, tc.historicalErr, err, "historical verification must not use the current clock")
			_, err = verifier.VerifyLiveConsenterSig(signature, proposal)
			expectedErr := tc.historicalErr
			if expectedErr == nil {
				expectedErr = tc.currentErr
			}
			require.Equal(t, expectedErr, err)
			cv.AssertExpectations(t)
		})
	}
}

func TestVerifyProposal(t *testing.T) {
	logger := flogging.MustGetLogger("test")
	lastBlock := makeNonConfigBlock(19, 10)
	notLastBlock := makeNonConfigBlock(18, 10)
	lastConfigBlock := makeConfigBlock(10)

	ledger := &mocks.Ledger{}
	ledger.On("Height").Return(uint64(20))
	ledger.On("Block", uint64(19)).Return(lastBlock)
	ledger.On("Block", uint64(10)).Return(lastConfigBlock)

	sequencer := &mocks.Sequencer{}
	sequencer.On("Sequence").Return(uint64(12))

	ac := &mocks.AccessController{}
	ac.On("Evaluate", mock.Anything).Return(nil)

	cv := &mocks.ConsenterVerifier{}

	reqInspector := &smartbft.RequestInspector{
		ValidateIdentityStructure: func(_ *msp.SerializedIdentity) error {
			return nil
		},
		Logger: logger,
	}

	lastHash := hex.EncodeToString(protoutil.BlockHeaderHash(lastBlock.GetHeader()))

	for _, testCase := range []struct {
		description                 string
		verificationSequence        uint64
		lastBlock                   *cb.Block
		lastConfigBlock             *cb.Block
		bftMetadataMutator          func([]byte) []byte
		ordererBlockMetadataMutator func(metadata *cb.OrdererBlockMetadata)
		verifierChainID             string
		expectedErr                 string
	}{
		{
			description:                 "green path",
			verificationSequence:        12,
			lastBlock:                   lastBlock,
			lastConfigBlock:             lastConfigBlock,
			bftMetadataMutator:          noopMutator,
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			verifierChainID:             "test-chain",
		},
		{
			description:                 "wrong chain ID",
			verificationSequence:        12,
			lastBlock:                   lastBlock,
			lastConfigBlock:             lastConfigBlock,
			bftMetadataMutator:          noopMutator,
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			verifierChainID:             "not-test-chain",
			expectedErr:                 "request is for channel test-chain but expected channel not-test-chain",
		},
		{
			description:                 "wrong verification sequence 1",
			verificationSequence:        11,
			lastBlock:                   lastBlock,
			lastConfigBlock:             lastConfigBlock,
			bftMetadataMutator:          noopMutator,
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			expectedErr:                 "expected verification sequence 12, but proposal has 11",
			verifierChainID:             "test-chain",
		},
		{
			description:                 "wrong verification sequence 2",
			verificationSequence:        12,
			lastBlock:                   lastBlock,
			lastConfigBlock:             protoutil.NewBlock(666, nil),
			bftMetadataMutator:          noopMutator,
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			expectedErr:                 "last config in block orderer metadata points to 666 but our persisted last config is 10",
			verifierChainID:             "test-chain",
		},
		{
			description:                 "wrong verification sequence 3",
			verificationSequence:        12,
			lastBlock:                   notLastBlock,
			lastConfigBlock:             lastConfigBlock,
			bftMetadataMutator:          noopMutator,
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			expectedErr: fmt.Sprintf("previous header hash is %s but expected %s",
				hex.EncodeToString(protoutil.BlockHeaderHash(notLastBlock.GetHeader())),
				hex.EncodeToString(protoutil.BlockHeaderHash(lastBlock.GetHeader()))),
			verifierChainID: "test-chain",
		},
		{
			description:          "corrupt metadata",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator: func([]byte) []byte {
				return []byte{1, 2, 3}
			},
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			expectedErr:                 "failed unmarshaling smartbft metadata from proposal: proto: cannot parse invalid wire-format data",
			verifierChainID:             "test-chain",
		},
		{
			description:          "corrupt metadata",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator: func([]byte) []byte {
				return protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{LatestSequence: 100, ViewId: 2})
			},
			ordererBlockMetadataMutator: noopOrdererBlockMetadataMutator,
			expectedErr:                 "expected metadata in block to be [view_id:2 latest_sequence:100] but got [view_id:2 latest_sequence:1]",
		},
		{
			description:          "No last config",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator:   noopMutator,
			ordererBlockMetadataMutator: func(metadata *cb.OrdererBlockMetadata) {
				metadata.LastConfig = nil
			},
			expectedErr: "last config is nil",
		},
		{
			description:          "Mismatched last config",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator:   noopMutator,
			ordererBlockMetadataMutator: func(metadata *cb.OrdererBlockMetadata) {
				metadata.LastConfig.Index = 666
			},
			expectedErr: "last config in block orderer metadata points to 666 but our persisted last config is 10",
		},
		{
			description:          "Corrupt inner BFT metadata",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator:   noopMutator,
			ordererBlockMetadataMutator: func(metadata *cb.OrdererBlockMetadata) {
				metadata.ConsenterMetadata = []byte{1, 2, 3}
			},
			expectedErr: "failed unmarshaling smartbft metadata from block: proto: cannot parse invalid wire-format data",
		},
		{
			description:          "Mismatching inner BFT metadata",
			verificationSequence: 12,
			lastBlock:            lastBlock,
			lastConfigBlock:      lastConfigBlock,
			bftMetadataMutator:   noopMutator,
			ordererBlockMetadataMutator: func(metadata *cb.OrdererBlockMetadata) {
				metadata.ConsenterMetadata = protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{LatestSequence: 666})
			},
			expectedErr: "expected metadata in block to be [view_id:2 latest_sequence:1] but got [view_id:0 latest_sequence:666]",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			runtimeConfig := &atomic.Value{}
			rtc := smartbft.RuntimeConfig{
				LastCommittedBlockHash: lastHash,
				LastBlock:              testCase.lastBlock,
				LastConfigBlock:        testCase.lastConfigBlock,
			}

			runtimeConfig.Store(rtc)

			assembler := &smartbft.Assembler{
				RuntimeConfig: &atomic.Value{},
				VerificationSeq: func() uint64 {
					return testCase.verificationSequence
				},
				Logger: logger,
			}
			assembler.RuntimeConfig.Store(smartbft.RuntimeConfig{
				LastConfigBlock: testCase.lastConfigBlock,
				LastBlock:       testCase.lastBlock,
			})

			md := protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{
				LatestSequence: 1,
				ViewId:         2,
			})

			proposal := assembler.AssembleProposal(md, [][]byte{nonConfigTx})

			// Maybe mutate the BFT metadata
			proposal.Metadata = testCase.bftMetadataMutator(proposal.Metadata)

			// Unwrap the OrdererBlockMetadata
			tuple := &smartbft.ByteBufferTuple{}
			_ = tuple.FromBytes(proposal.Payload)
			blockMD := &cb.BlockMetadata{}
			require.NoError(t, proto.Unmarshal(tuple.B, blockMD))

			sigMD := &cb.Metadata{}
			require.NoError(t, proto.Unmarshal(blockMD.GetMetadata()[cb.BlockMetadataIndex_SIGNATURES], sigMD))

			ordererMetadataFromSignature := &cb.OrdererBlockMetadata{}
			require.NoError(t, proto.Unmarshal(sigMD.GetValue(), ordererMetadataFromSignature))

			// Mutate the OrdererBlockMetadata
			testCase.ordererBlockMetadataMutator(ordererMetadataFromSignature)

			// And fold it back into the block
			sigMD.Value = protoutil.MarshalOrPanic(ordererMetadataFromSignature)
			blockMD.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(sigMD)
			tuple.B = protoutil.MarshalOrPanic(blockMD)
			proposal.Payload = tuple.ToBytes()

			runtimeConfig = &atomic.Value{}
			rtc.LastConfigBlock = lastConfigBlock
			runtimeConfig.Store(rtc)
			v := &smartbft.Verifier{
				Channel:               testCase.verifierChainID,
				RuntimeConfig:         runtimeConfig,
				Logger:                logger,
				Ledger:                ledger,
				VerificationSequencer: sequencer,
				AccessController:      ac,
				ConsenterVerifier:     cv,
				ReqInspector:          reqInspector,
				ValidateAdmission:     func(*cb.Envelope) error { return nil },
			}

			if testCase.description == "green path" {
				v.ValidateAdmission = nil
				_, err := v.VerifyRequest(nonConfigTx)
				require.ErrorContains(t, err, "transaction admission validator is not configured")
				v.ValidateAdmission = func(*cb.Envelope) error {
					return errors.New("expired endorsement")
				}
				_, err = v.VerifyRequest(nonConfigTx)
				require.ErrorContains(t, err, "transaction admission rejected: expired endorsement")
				_, err = v.VerifyProposal(proposal)
				require.ErrorContains(t, err, "transaction admission rejected: expired endorsement")
				v.ValidateAdmission = func(*cb.Envelope) error { return nil }
			}

			reqInfo, err := v.VerifyProposal(proposal)

			if testCase.expectedErr == "" {
				require.NoError(t, err)
				require.NotNil(t, reqInfo)
				require.Len(t, reqInfo, 1)
				require.Equal(t, hashOfZero, reqInfo[0].ClientID)
				require.Equal(t, hashOfZero, reqInfo[0].ID)
				return
			}

			require.Error(t, err)
			require.Equal(t, testCase.expectedErr, strings.ReplaceAll(err.Error(), "\u00a0", " "))
			require.Nil(t, reqInfo)
		})
	}
}

func noopMutator(b []byte) []byte {
	return b
}

func noopOrdererBlockMetadataMutator(_ *cb.OrdererBlockMetadata) {
}
