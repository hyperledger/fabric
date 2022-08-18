/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"crypto/sha256"
	"encoding/asn1"
	"math"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/hyperledger/fabric/protoutil/mocks"
	"github.com/stretchr/testify/require"
)

var testChannelID = "myuniquetestchainid"

func TestNewBlock(t *testing.T) {
	var block *cb.Block
	require.Nil(t, block.GetHeader())
	require.Nil(t, block.GetData())
	require.Nil(t, block.GetMetadata())

	data := &cb.BlockData{
		Data: [][]byte{{0, 1, 2}},
	}
	block = protoutil.NewBlock(uint64(0), []byte("datahash"))
	require.Equal(t, []byte("datahash"), block.Header.PreviousHash, "Incorrect previous hash")
	require.NotNil(t, block.GetData())
	require.NotNil(t, block.GetMetadata())
	block.GetHeader().DataHash = protoutil.BlockDataHash(data)

	asn1Bytes, err := asn1.Marshal(struct {
		Number       int64
		PreviousHash []byte
		DataHash     []byte
	}{
		Number:       0,
		DataHash:     protoutil.BlockDataHash(data),
		PreviousHash: []byte("datahash"),
	})
	headerHash := sha256.Sum256(asn1Bytes)
	require.NoError(t, err)
	require.Equal(t, asn1Bytes, protoutil.BlockHeaderBytes(block.Header), "Incorrect marshaled blockheader bytes")
	require.Equal(t, headerHash[:], protoutil.BlockHeaderHash(block.Header), "Incorrect blockheader hash")
}

func TestGoodBlockHeaderBytes(t *testing.T) {
	goodBlockHeader := &cb.BlockHeader{
		Number:       1,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = protoutil.BlockHeaderBytes(goodBlockHeader) // Should not panic

	goodBlockHeaderMaxNumber := &cb.BlockHeader{
		Number:       math.MaxUint64,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = protoutil.BlockHeaderBytes(goodBlockHeaderMaxNumber) // Should not panic
}

func TestGetChannelIDFromBlockBytes(t *testing.T) {
	gb, err := configtxtest.MakeGenesisBlock(testChannelID)
	require.NoError(t, err, "Failed to create test configuration block")
	bytes, err := proto.Marshal(gb)
	require.NoError(t, err)
	cid, err := protoutil.GetChannelIDFromBlockBytes(bytes)
	require.NoError(t, err)
	require.Equal(t, testChannelID, cid, "Failed to return expected chain ID")

	// bad block bytes
	_, err = protoutil.GetChannelIDFromBlockBytes([]byte("bad block"))
	require.Error(t, err, "Expected error with malformed block bytes")
}

func TestGetChannelIDFromBlock(t *testing.T) {
	var err error
	var gb *cb.Block
	var cid string

	// nil block
	_, err = protoutil.GetChannelIDFromBlock(gb)
	require.Error(t, err, "Expected error getting channel id from nil block")

	gb, err = configtxtest.MakeGenesisBlock(testChannelID)
	require.NoError(t, err, "Failed to create test configuration block")

	cid, err = protoutil.GetChannelIDFromBlock(gb)
	require.NoError(t, err, "Failed to get chain ID from block")
	require.Equal(t, testChannelID, cid, "Failed to return expected chain ID")

	// missing data
	badBlock := gb
	badBlock.Data = nil
	_, err = protoutil.GetChannelIDFromBlock(badBlock)
	require.Error(t, err, "Expected error with missing block data")

	// no envelope
	badBlock = &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{[]byte("bad envelope")},
		},
	}
	_, err = protoutil.GetChannelIDFromBlock(badBlock)
	require.Error(t, err, "Expected error with no envelope in data")

	// bad payload
	env, _ := proto.Marshal(&cb.Envelope{
		Payload: []byte("bad payload"),
	})
	badBlock = &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{env},
		},
	}
	_, err = protoutil.GetChannelIDFromBlock(badBlock)
	require.Error(t, err, "Expected error - malformed payload")

	// bad channel header
	payload, _ := proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: []byte("bad header"),
		},
	})
	env, _ = proto.Marshal(&cb.Envelope{
		Payload: payload,
	})
	badBlock = &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{env},
		},
	}
	_, err = protoutil.GetChannelIDFromBlock(badBlock)
	require.Error(t, err, "Expected error with malformed channel header")

	// nil payload header
	payload, _ = proto.Marshal(&cb.Payload{})
	env, _ = proto.Marshal(&cb.Envelope{
		Payload: payload,
	})
	badBlock = &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{env},
		},
	}
	_, err = protoutil.GetChannelIDFromBlock(badBlock)
	require.Error(t, err, "Expected error when payload header is nil")
}

func TestGetBlockFromBlockBytes(t *testing.T) {
	testChainID := "myuniquetestchainid"
	gb, err := configtxtest.MakeGenesisBlock(testChainID)
	require.NoError(t, err, "Failed to create test configuration block")
	blockBytes, err := protoutil.Marshal(gb)
	require.NoError(t, err, "Failed to marshal block")
	_, err = protoutil.UnmarshalBlock(blockBytes)
	require.NoError(t, err, "to get block from block bytes")

	// bad block bytes
	_, err = protoutil.UnmarshalBlock([]byte("bad block"))
	require.Error(t, err, "Expected error for malformed block bytes")
}

func TestGetMetadataFromBlock(t *testing.T) {
	t.Run("new block", func(t *testing.T) {
		block := protoutil.NewBlock(0, nil)
		md, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
		require.NoError(t, err, "Unexpected error extracting metadata from new block")
		require.Nil(t, md.Value, "Expected metadata field value to be nil")
		require.Equal(t, 0, len(md.Value), "Expected length of metadata field value to be 0")
		md = protoutil.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_ORDERER)
		require.NotNil(t, md, "Expected to get metadata from block")
	})
	t.Run("no metadata", func(t *testing.T) {
		block := protoutil.NewBlock(0, nil)
		block.Metadata = nil
		_, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
		require.Error(t, err, "Expected error with nil metadata")
		require.Contains(t, err.Error(), "no metadata in block")
	})
	t.Run("no metadata at index", func(t *testing.T) {
		block := protoutil.NewBlock(0, nil)
		block.Metadata.Metadata = [][]byte{{1, 2, 3}}
		_, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_LAST_CONFIG)
		require.Error(t, err, "Expected error with nil metadata")
		require.Contains(t, err.Error(), "no metadata at index")
	})
	t.Run("malformed metadata", func(t *testing.T) {
		block := protoutil.NewBlock(0, nil)
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = []byte("bad metadata")
		_, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
		require.Error(t, err, "Expected error with malformed metadata")
		require.Contains(t, err.Error(), "error unmarshalling metadata at index [ORDERER]")
		require.Panics(t, func() {
			_ = protoutil.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_ORDERER)
		}, "Expected panic with malformed metadata")
	})
}

func TestGetConsenterMetadataFromBlock(t *testing.T) {
	cases := []struct {
		name       string
		value      []byte
		signatures []byte
		orderer    []byte
		pass       bool
	}{
		{
			name:       "empty",
			value:      nil,
			signatures: nil,
			orderer:    nil,
			pass:       true,
		},
		{
			name:  "signature only",
			value: []byte("hello"),
			signatures: protoutil.MarshalOrPanic(&cb.Metadata{
				Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
					ConsenterMetadata: protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("hello")}),
				}),
			}),
			orderer: nil,
			pass:    true,
		},
		{
			name:       "orderer only",
			value:      []byte("hello"),
			signatures: nil,
			orderer:    protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("hello")}),
			pass:       true,
		},
		{
			name:  "both signatures and orderer",
			value: []byte("hello"),
			signatures: protoutil.MarshalOrPanic(&cb.Metadata{
				Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
					ConsenterMetadata: protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("hello")}),
				}),
			}),
			orderer: protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("hello")}),
			pass:    true,
		},
		{
			name:       "malformed OrdererBlockMetadata",
			signatures: protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("malformed")}),
			orderer:    nil,
			pass:       false,
		},
	}

	for _, test := range cases {
		block := protoutil.NewBlock(0, nil)
		block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = test.signatures
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = test.orderer
		result, err := protoutil.GetConsenterMetadataFromBlock(block)

		if test.pass {
			require.NoError(t, err)
			require.Equal(t, result.Value, test.value)
		} else {
			require.Error(t, err)
		}
	}
}

func TestInitBlockMeta(t *testing.T) {
	// block with no metadata
	block := &cb.Block{}
	protoutil.InitBlockMetadata(block)
	// should have 3 entries
	require.Equal(t, 5, len(block.Metadata.Metadata), "Expected block to have 5 metadata entries")

	// block with a single entry
	block = &cb.Block{
		Metadata: &cb.BlockMetadata{},
	}
	block.Metadata.Metadata = append(block.Metadata.Metadata, []byte{})
	protoutil.InitBlockMetadata(block)
	// should have 3 entries
	require.Equal(t, 5, len(block.Metadata.Metadata), "Expected block to have 5 metadata entries")
}

func TestCopyBlockMetadata(t *testing.T) {
	srcBlock := protoutil.NewBlock(0, nil)
	dstBlock := &cb.Block{}

	metadata, _ := proto.Marshal(&cb.Metadata{
		Value: []byte("orderer metadata"),
	})
	srcBlock.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = metadata
	protoutil.CopyBlockMetadata(srcBlock, dstBlock)

	// check that the copy worked
	require.Equal(t, len(srcBlock.Metadata.Metadata), len(dstBlock.Metadata.Metadata),
		"Expected target block to have same number of metadata entries after copy")
	require.Equal(t, metadata, dstBlock.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER],
		"Unexpected metadata from target block")
}

func TestGetLastConfigIndexFromBlock(t *testing.T) {
	index := uint64(2)
	block := protoutil.NewBlock(0, nil)

	t.Run("block with last config metadata in signatures field", func(t *testing.T) {
		block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
			Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
				LastConfig: &cb.LastConfig{Index: 2},
			}),
		})
		result, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.NoError(t, err, "Unexpected error returning last config index")
		require.Equal(t, index, result, "Unexpected last config index returned from block")
		result = protoutil.GetLastConfigIndexFromBlockOrPanic(block)
		require.Equal(t, index, result, "Unexpected last config index returned from block")
	})

	t.Run("block with malformed signatures", func(t *testing.T) {
		block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = []byte("apple")
		_, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to retrieve metadata: error unmarshalling metadata at index [SIGNATURES]")
	})

	t.Run("block with malformed orderer block metadata", func(t *testing.T) {
		block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{Value: []byte("banana")})
		_, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to unmarshal orderer block metadata")
	})

	// TODO: FAB-15864 remove the tests below when we stop supporting upgrade from
	//       pre-1.4.1 orderer
	t.Run("block with deprecated (pre-1.4.1) last config", func(t *testing.T) {
		block = protoutil.NewBlock(0, nil)
		block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{
			Value: protoutil.MarshalOrPanic(&cb.LastConfig{
				Index: index,
			}),
		})
		result, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.NoError(t, err, "Unexpected error returning last config index")
		require.Equal(t, index, result, "Unexpected last config index returned from block")
		result = protoutil.GetLastConfigIndexFromBlockOrPanic(block)
		require.Equal(t, index, result, "Unexpected last config index returned from block")
	})

	t.Run("malformed metadata", func(t *testing.T) {
		block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = []byte("bad metadata")
		_, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to retrieve metadata: error unmarshalling metadata at index [LAST_CONFIG]")
	})

	t.Run("malformed last config", func(t *testing.T) {
		block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{
			Value: []byte("bad last config"),
		})
		_, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.Error(t, err, "Expected error with malformed last config metadata")
		require.Contains(t, err.Error(), "error unmarshalling LastConfig")
		require.Panics(t, func() {
			_ = protoutil.GetLastConfigIndexFromBlockOrPanic(block)
		}, "Expected panic with malformed last config metadata")
	})
}

func TestBlockSignatureVerifierEmptyMetadata(t *testing.T) {
	policies := mocks.Policy{}

	verify := protoutil.BlockSignatureVerifier(true, nil, &policies)

	header := &cb.BlockHeader{}
	md := &cb.BlockMetadata{}

	err := verify(header, md)
	require.ErrorContains(t, err, "no signatures in block metadata")
}

func TestBlockSignatureVerifierByIdentifier(t *testing.T) {
	consenters := []*cb.Consenter{
		{
			Id:       1,
			Host:     "host1",
			Port:     8001,
			MspId:    "msp1",
			Identity: []byte("identity1"),
		},
		{
			Id:       2,
			Host:     "host2",
			Port:     8002,
			MspId:    "msp2",
			Identity: []byte("identity2"),
		},
		{
			Id:       3,
			Host:     "host3",
			Port:     8003,
			MspId:    "msp3",
			Identity: []byte("identity3"),
		},
	}

	policies := mocks.Policy{}

	verify := protoutil.BlockSignatureVerifier(true, consenters, &policies)

	header := &cb.BlockHeader{}
	md := &cb.BlockMetadata{
		Metadata: [][]byte{
			protoutil.MarshalOrPanic(&cb.Metadata{Signatures: []*cb.MetadataSignature{
				{
					Signature:        []byte{},
					IdentifierHeader: protoutil.MarshalOrPanic(&cb.IdentifierHeader{Identifier: 1}),
				},
				{
					Signature:        []byte{},
					IdentifierHeader: protoutil.MarshalOrPanic(&cb.IdentifierHeader{Identifier: 3}),
				},
			}}),
		},
	}

	err := verify(header, md)
	require.NoError(t, err)
	signatureSet := policies.EvaluateSignedDataArgsForCall(0)
	require.Len(t, signatureSet, 2)
	require.Equal(t, protoutil.MarshalOrPanic(&msp.SerializedIdentity{Mspid: "msp1", IdBytes: []byte("identity1")}), signatureSet[0].Identity)
	require.Equal(t, protoutil.MarshalOrPanic(&msp.SerializedIdentity{Mspid: "msp3", IdBytes: []byte("identity3")}), signatureSet[1].Identity)
}

func TestBlockSignatureVerifierByCreator(t *testing.T) {
	consenters := []*cb.Consenter{
		{
			Id:       1,
			Host:     "host1",
			Port:     8001,
			MspId:    "msp1",
			Identity: []byte("identity1"),
		},
		{
			Id:       2,
			Host:     "host2",
			Port:     8002,
			MspId:    "msp2",
			Identity: []byte("identity2"),
		},
		{
			Id:       3,
			Host:     "host3",
			Port:     8003,
			MspId:    "msp3",
			Identity: []byte("identity3"),
		},
	}

	policies := mocks.Policy{}

	verify := protoutil.BlockSignatureVerifier(true, consenters, &policies)

	header := &cb.BlockHeader{}
	md := &cb.BlockMetadata{
		Metadata: [][]byte{
			protoutil.MarshalOrPanic(&cb.Metadata{Signatures: []*cb.MetadataSignature{
				{
					Signature:       []byte{},
					SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{Creator: []byte("creator1")}),
				},
			}}),
		},
	}

	err := verify(header, md)
	require.NoError(t, err)
	signatureSet := policies.EvaluateSignedDataArgsForCall(0)
	require.Len(t, signatureSet, 1)
	require.Equal(t, []byte("creator1"), signatureSet[0].Identity)
}
