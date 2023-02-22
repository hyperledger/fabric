/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"sync/atomic"
	"testing"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var (
	configTx    = makeTx(int32(common.HeaderType_CONFIG))
	nonConfigTx = makeTx(int32(common.HeaderType_ENDORSER_TRANSACTION))
)

func TestAssembler(t *testing.T) {
	lastBlock := makeNonConfigBlock(19, 10)
	lastHash := protoutil.BlockHeaderHash(lastBlock.Header)
	lastConfigBlock := makeConfigBlock(10)

	ledger := &mocks.Ledger{}
	ledger.On("Height").Return(uint64(20))
	ledger.On("Block", uint64(19)).Return(lastBlock)
	ledger.On("Block", uint64(10)).Return(lastConfigBlock)

	for _, testCase := range []struct {
		name             string
		panicVal         string
		requests         [][]byte
		expectedProposal types.Proposal
	}{
		{
			name:     "Must contain at least one request",
			panicVal: "Programming error, no requests in proposal",
		},
		{
			name: "Must not contain an invalid request",
			panicVal: "Programming error, received bad envelope but should have" +
				" validated it: error unmarshalling Envelope: proto: cannot parse invalid wire-format data",
			requests: [][]byte{{1, 2, 3}},
		},
		{
			name:             "Config transaction is first in the batch",
			requests:         [][]byte{configTx, nonConfigTx},
			expectedProposal: proposalFromRequests(10, 20, 20, lastHash, []byte("metadata"), configTx),
		},
		{
			name:             "Config transaction is in the middle of the batch",
			requests:         [][]byte{nonConfigTx, configTx, nonConfigTx},
			expectedProposal: proposalFromRequests(10, 20, 10, lastHash, []byte("metadata"), nonConfigTx),
		},
		{
			name:             "Config transaction is at the end of the batch",
			requests:         [][]byte{nonConfigTx, nonConfigTx, configTx},
			expectedProposal: proposalFromRequests(10, 20, 10, lastHash, []byte("metadata"), nonConfigTx, nonConfigTx),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			logger := flogging.MustGetLogger("test")

			assembler := &smartbft.Assembler{
				VerificationSeq: func() uint64 {
					return 10
				},
				Logger:        logger,
				RuntimeConfig: &atomic.Value{},
			}

			rtc := smartbft.RuntimeConfig{
				LastBlock:       smartbft.LastBlockFromLedgerOrPanic(ledger, logger),
				LastConfigBlock: smartbft.LastConfigBlockFromLedgerOrPanic(ledger, logger),
			}
			assembler.RuntimeConfig.Store(rtc)

			if testCase.panicVal != "" {

				require.Panics(t, func() {
					assembler.AssembleProposal([]byte{1, 2, 3}, testCase.requests)
				})
				return
			}

			prop := assembler.AssembleProposal([]byte("metadata"), testCase.requests)
			require.Equal(t, testCase.expectedProposal, prop)
		})
	}
}

func makeTx(headerType int32) []byte {
	return protoutil.MarshalOrPanic(&common.Envelope{
		Payload: protoutil.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
					Type:      headerType,
					ChannelId: "test-chain",
				}),
			},
		}),
	})
}

func makeNonConfigBlock(seq, lastConfigSeq uint64) *common.Block {
	return &common.Block{
		Header: &common.BlockHeader{
			Number: seq,
		},
		Data: &common.BlockData{
			Data: [][]byte{nonConfigTx},
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{
				{},
				protoutil.MarshalOrPanic(&common.Metadata{
					Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: lastConfigSeq}),
				}),
			},
		},
	}
}

func makeConfigBlock(seq uint64) *common.Block {
	return &common.Block{
		Header: &common.BlockHeader{
			Number: seq,
		},
		Data: &common.BlockData{
			Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						Config: &common.Config{
							Sequence: 10,
						},
					}),
				}),
			})},
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{
				{},
				protoutil.MarshalOrPanic(&common.Metadata{
					Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 666}),
				}),
			},
		},
	}
}

func proposalFromRequests(verificationSeq, seq, lastConfigSeq uint64, lastBlockHash, metadata []byte, requests ...[]byte) types.Proposal {
	block := protoutil.NewBlock(seq, nil)
	block.Data = &common.BlockData{Data: requests}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	block.Header.PreviousHash = lastBlockHash
	block.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: lastConfigSeq}),
	})

	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
			ConsenterMetadata: metadata,
			LastConfig: &common.LastConfig{
				Index: lastConfigSeq,
			},
		}),
	})

	tuple := &smartbft.ByteBufferTuple{
		A: protoutil.MarshalOrPanic(block.Data),
		B: protoutil.MarshalOrPanic(block.Metadata),
	}

	return types.Proposal{
		Header:               protoutil.BlockHeaderBytes(block.Header),
		Payload:              tuple.ToBytes(),
		Metadata:             metadata,
		VerificationSequence: int64(verificationSeq),
	}
}
