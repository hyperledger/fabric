/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
	}{
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
			}

			if testCase.proposalMutator != nil {
				proposal = testCase.proposalMutator(proposal)
			}

			_, err := v.VerifyConsenterSig(signature, proposal)

			require.Error(t, err)
			require.Equal(t, testCase.expectedErr, strings.ReplaceAll(err.Error(), "\u00a0", " "))
		})
	}
}

func TestVerifyConsenterSigSubstitutedPayload(t *testing.T) {
	logger := flogging.MustGetLogger("test")

	lastBlock := makeNonConfigBlock(19, 10)
	lastConfigBlock := makeConfigBlock(10)

	cv := &mocks.ConsenterVerifier{}
	cv.On("Evaluate", mock.Anything).Return(nil)

	rtc := smartbft.RuntimeConfig{
		ID2Identities:   map[uint64][]byte{3: {0, 2, 4, 6}},
		LastBlock:       lastBlock,
		LastConfigBlock: lastConfigBlock,
	}
	runtimeConfig := &atomic.Value{}
	runtimeConfig.Store(rtc)

	v := &smartbft.Verifier{
		RuntimeConfig:     runtimeConfig,
		Logger:            logger,
		ConsenterVerifier: cv,
	}

	ss := &mocks.SignerSerializer{}
	ss.On("Sign", mock.Anything).Return([]byte{1, 2, 3}, nil)
	ss.On("Serialize", mock.Anything).Return([]byte{0, 2, 4, 6}, nil)

	s := &smartbft.Signer{
		LastConfigBlockNum: func(_ *cb.Block) uint64 {
			return lastConfigBlock.GetHeader().GetNumber()
		},
		SignerSerializer: ss,
		Logger:           logger,
		ID:               3,
	}

	assembler := &smartbft.Assembler{
		VerificationSeq: func() uint64 { return 12 },
		Logger:          logger,
		RuntimeConfig:   runtimeConfig,
	}

	md := protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{LatestSequence: 20, ViewId: 2})
	proposal := assembler.AssembleProposal(md, [][]byte{nonConfigTx})
	signature := *s.SignProposal(proposal, nil)

	_, err := v.VerifyConsenterSig(signature, proposal)
	require.NoError(t, err, "unmodified proposal must pass consenter signature verification")

	// Keep the signed header and metadata, substitute only the payload.
	tuple := &smartbft.ByteBufferTuple{}
	require.NoError(t, tuple.FromBytes(proposal.Payload))
	blockData := &cb.BlockData{}
	require.NoError(t, proto.Unmarshal(tuple.A, blockData))
	blockData.Data = append(blockData.GetData(), blockData.GetData()[0])
	tuple.A = protoutil.MarshalOrPanic(blockData)
	forgedProposal := types.Proposal{
		Header:               proposal.Header,
		Metadata:             proposal.Metadata,
		Payload:              tuple.ToBytes(),
		VerificationSequence: proposal.VerificationSequence,
	}

	forgedBlock, err := smartbft.ProposalToBlock(forgedProposal)
	require.NoError(t, err)
	actualDataHash, err := protoutil.BlockDataHash(forgedBlock.GetData())
	require.NoError(t, err)
	require.NotEqual(t, forgedBlock.GetHeader().GetDataHash(), actualDataHash)

	_, err = v.VerifyConsenterSig(signature, forgedProposal)
	require.Error(t, err, "substituted payload must not pass consenter signature verification")
	require.Contains(t, err.Error(), "data hash")

	// Mutating the already verified payload's backing array in place must not
	// defeat the binding either: the cached hash key must not alias the payload.
	txOffset := bytes.Index(proposal.Payload, nonConfigTx)
	require.NotEqual(t, -1, txOffset)
	proposal.Payload[txOffset+len(nonConfigTx)/2] ^= 0xff

	_, err = v.VerifyConsenterSig(signature, proposal)
	require.Error(t, err, "in-place mutated payload must not pass consenter signature verification")
	require.Contains(t, err.Error(), "data hash")

	proposal.Payload[txOffset+len(nonConfigTx)/2] ^= 0xff
	_, err = v.VerifyConsenterSig(signature, proposal)
	require.NoError(t, err, "restored payload must pass consenter signature verification")
}

// dataHashCountingVerifier counts block data hash computations and signs proposals
// as consenters 3, 4 and 5.
type dataHashCountingVerifier struct {
	verifier     *smartbft.Verifier
	assembler    *smartbft.Assembler
	computations *atomic.Int32
}

func newDataHashCountingVerifier() *dataHashCountingVerifier {
	logger := flogging.MustGetLogger("test")

	lastBlock := makeNonConfigBlock(19, 10)
	lastConfigBlock := makeConfigBlock(10)

	cv := &mocks.ConsenterVerifier{}
	cv.On("Evaluate", mock.Anything).Return(nil)

	runtimeConfig := &atomic.Value{}
	runtimeConfig.Store(smartbft.RuntimeConfig{
		ID2Identities:   map[uint64][]byte{3: {0, 2, 4, 6}, 4: {0, 2, 4, 8}, 5: {0, 2, 4, 10}},
		LastBlock:       lastBlock,
		LastConfigBlock: lastConfigBlock,
	})

	computations := &atomic.Int32{}
	return &dataHashCountingVerifier{
		verifier: &smartbft.Verifier{
			RuntimeConfig:     runtimeConfig,
			Logger:            logger,
			ConsenterVerifier: cv,
			BlockDataHash: func(blockData *cb.BlockData) ([]byte, error) {
				computations.Add(1)
				return protoutil.BlockDataHash(blockData)
			},
		},
		assembler: &smartbft.Assembler{
			VerificationSeq: func() uint64 { return 12 },
			Logger:          logger,
			RuntimeConfig:   runtimeConfig,
		},
		computations: computations,
	}
}

func (dcv *dataHashCountingVerifier) sign(id uint64, proposal types.Proposal) types.Signature {
	logger := flogging.MustGetLogger("test")

	ss := &mocks.SignerSerializer{}
	ss.On("Sign", mock.Anything).Return([]byte{1, 2, 3}, nil)
	ss.On("Serialize", mock.Anything).Return([]byte{0, 2, 4, byte(2 * id)}, nil)

	s := &smartbft.Signer{
		LastConfigBlockNum: func(_ *cb.Block) uint64 { return 10 },
		SignerSerializer:   ss,
		Logger:             logger,
		ID:                 id,
	}
	return *s.SignProposal(proposal, nil)
}

// verifyQuorum verifies a quorum of consenter signatures over the same proposal,
// as in a commit certificate.
func (dcv *dataHashCountingVerifier) verifyQuorum(t *testing.T, proposal types.Proposal) {
	t.Helper()
	for _, id := range []uint64{3, 4, 5} {
		_, err := dcv.verifier.VerifyConsenterSig(dcv.sign(id, proposal), proposal)
		require.NoError(t, err)
	}
}

func TestVerifyConsenterSigDataHashComputedOncePerProposal(t *testing.T) {
	dcv := newDataHashCountingVerifier()

	md := protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{LatestSequence: 20, ViewId: 2})
	proposal1 := dcv.assembler.AssembleProposal(md, [][]byte{nonConfigTx})
	otherTx := bytes.Clone(nonConfigTx)
	otherTx[len(otherTx)-1] ^= 0xff
	proposal2 := dcv.assembler.AssembleProposal(md, [][]byte{otherTx})

	dcv.verifyQuorum(t, proposal1)
	require.Equal(t, int32(1), dcv.computations.Load(),
		"block data hash must be computed once per proposal, not once per consenter signature")

	// A malformed proposal must be rejected before any hashing and must not touch the cache.
	malformed := types.Proposal{
		Header:               proposal1.Header,
		Metadata:             proposal1.Metadata,
		Payload:              []byte{1, 2, 3, 4},
		VerificationSequence: proposal1.VerificationSequence,
	}
	_, err := dcv.verifier.VerifyConsenterSig(dcv.sign(3, proposal1), malformed)
	require.Error(t, err)
	require.Equal(t, int32(1), dcv.computations.Load(), "malformed proposal must not reach the hash computation")

	// A well-formed payload whose data does not match the signed header must be
	// rejected without being retained by the cache.
	tuple := &smartbft.ByteBufferTuple{}
	require.NoError(t, tuple.FromBytes(proposal1.Payload))
	blockData := &cb.BlockData{}
	require.NoError(t, proto.Unmarshal(tuple.A, blockData))
	blockData.Data = append(blockData.GetData(), blockData.GetData()[0])
	tuple.A = protoutil.MarshalOrPanic(blockData)
	forged := types.Proposal{
		Header:               proposal1.Header,
		Metadata:             proposal1.Metadata,
		Payload:              tuple.ToBytes(),
		VerificationSequence: proposal1.VerificationSequence,
	}
	_, err = dcv.verifier.VerifyConsenterSig(dcv.sign(3, proposal1), forged)
	require.Error(t, err)
	require.Contains(t, err.Error(), "data hash")

	_, err = dcv.verifier.VerifyConsenterSig(dcv.sign(3, proposal1), proposal1)
	require.NoError(t, err, "rejected payload must not poison the cached hash")
	require.Equal(t, int32(2), dcv.computations.Load(), "rejected payload must not be retained")

	dcv.verifyQuorum(t, proposal2)
	require.Equal(t, int32(3), dcv.computations.Load(),
		"each distinct proposal must be hashed exactly once")
}

func TestVerifyConsenterSigDataHashConcurrent(t *testing.T) {
	dcv := newDataHashCountingVerifier()

	md := protoutil.MarshalOrPanic(&smartbftprotos.ViewMetadata{LatestSequence: 20, ViewId: 2})
	proposal := dcv.assembler.AssembleProposal(md, [][]byte{nonConfigTx})

	signatures := []types.Signature{
		dcv.sign(3, proposal),
		dcv.sign(4, proposal),
		dcv.sign(5, proposal),
	}

	// Mirror commit processing, where consenter signatures over the same
	// proposal are verified concurrently.
	const verifications = 30
	var wg sync.WaitGroup
	errs := make(chan error, verifications)
	for i := range verifications {
		wg.Go(func() {
			_, err := dcv.verifier.VerifyConsenterSig(signatures[i%len(signatures)], proposal)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), dcv.computations.Load(),
		"concurrent verifications of the same proposal must share a single hash computation")
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
