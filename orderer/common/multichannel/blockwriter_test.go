/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	newchannelconfig "github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	ramledger "github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/orderer/common/blockcutter/mock"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

type mockBlockWriterSupport struct {
	*mockconfigtx.Validator
	crypto.LocalSigner
	blockledger.ReadWriter
	fakeConfig *mock.OrdererConfig
}

func (mbws mockBlockWriterSupport) Update(bundle *newchannelconfig.Bundle) {
	mbws.Validator.SequenceVal++
}

func (mbws mockBlockWriterSupport) CreateBundle(channelID string, config *cb.Config) (*newchannelconfig.Bundle, error) {
	return channelconfig.NewBundle(channelID, config)
}

func (mbws mockBlockWriterSupport) SharedConfig() newchannelconfig.Orderer {
	return mbws.fakeConfig
}

func TestCreateBlock(t *testing.T) {
	seedBlock := cb.NewBlock(7, []byte("lasthash"))
	seedBlock.Data.Data = [][]byte{[]byte("somebytes")}

	bw := &BlockWriter{lastBlock: seedBlock}
	block := bw.CreateNextBlock([]*cb.Envelope{
		{Payload: []byte("some other bytes")},
	})

	assert.Equal(t, seedBlock.Header.Number+1, block.Header.Number)
	assert.Equal(t, block.Data.Hash(), block.Header.DataHash)
	assert.Equal(t, seedBlock.Header.Hash(), block.Header.PreviousHash)
}

func TestBlockSignature(t *testing.T) {
	rlf := ramledger.New(2)
	l, err := rlf.GetOrCreate("mychannel")
	assert.NoError(t, err)
	lastBlock := cb.NewBlock(0, nil)
	l.Append(lastBlock)

	bw := &BlockWriter{
		lastConfigBlockNum: 42,
		support: &mockBlockWriterSupport{
			LocalSigner: mockCrypto(),
			Validator:   &mockconfigtx.Validator{},
			ReadWriter:  l,
		},
		lastBlock: cb.NewBlock(1, lastBlock.Header.Hash()),
	}

	consensusMetadata := []byte("bar")
	bw.commitBlock(consensusMetadata)

	it, seq := l.Iterator(&orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{}})
	assert.Equal(t, uint64(1), seq)
	committedBlock, status := it.Next()
	assert.Equal(t, cb.Status_SUCCESS, status)

	md := utils.GetMetadataFromBlockOrPanic(committedBlock, cb.BlockMetadataIndex_SIGNATURES)

	expectedMetadataValue := utils.MarshalOrPanic(&cb.OrdererBlockMetadata{
		LastConfig:        &cb.LastConfig{Index: 42},
		ConsenterMetadata: utils.MarshalOrPanic(&cb.Metadata{Value: consensusMetadata}),
	})

	assert.Equal(t, expectedMetadataValue, md.Value, "Value contains the consensus metadata and the last config")
	assert.NotNil(t, md.Signatures, "Should have signature")
}

func TestBlockLastConfig(t *testing.T) {
	lastConfigSeq := uint64(6)
	newConfigSeq := lastConfigSeq + 1
	newBlockNum := uint64(9)

	bw := &BlockWriter{
		support: &mockBlockWriterSupport{
			LocalSigner: mockCrypto(),
			Validator: &mockconfigtx.Validator{
				SequenceVal: newConfigSeq,
			},
		},
		lastConfigSeq: lastConfigSeq,
	}

	block := cb.NewBlock(newBlockNum, []byte("foo"))
	bw.addLastConfigSignature(block)

	assert.Equal(t, newBlockNum, bw.lastConfigBlockNum)
	assert.Equal(t, newConfigSeq, bw.lastConfigSeq)

	md := utils.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_LAST_CONFIG)
	assert.NotNil(t, md.Value, "Value not be empty in this case")
	assert.Nil(t, md.Signatures, "Should have no signatures")

	lc := utils.GetLastConfigIndexFromBlockOrPanic(block)
	assert.Equal(t, newBlockNum, lc)
}

func TestWriteConfigBlock(t *testing.T) {
	// TODO, use assert.PanicsWithValue once available
	t.Run("EmptyBlock", func(t *testing.T) {
		assert.Panics(t, func() { (&BlockWriter{}).WriteConfigBlock(&cb.Block{}, nil) })
	})
	t.Run("BadPayload", func(t *testing.T) {
		assert.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						utils.MarshalOrPanic(&cb.Envelope{Payload: []byte("bad")}),
					},
				},
			}, nil)
		})
	})
	t.Run("MissingHeader", func(t *testing.T) {
		assert.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						utils.MarshalOrPanic(&cb.Envelope{
							Payload: utils.MarshalOrPanic(&cb.Payload{}),
						}),
					},
				},
			}, nil)
		})
	})
	t.Run("BadChannelHeader", func(t *testing.T) {
		assert.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						utils.MarshalOrPanic(&cb.Envelope{
							Payload: utils.MarshalOrPanic(&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: []byte("bad"),
								},
							}),
						}),
					},
				},
			}, nil)
		})
	})
	t.Run("BadChannelHeaderType", func(t *testing.T) {
		assert.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						utils.MarshalOrPanic(&cb.Envelope{
							Payload: utils.MarshalOrPanic(&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{}),
								},
							}),
						}),
					},
				},
			}, nil)
		})
	})
}

func TestGoodWriteConfig(t *testing.T) {
	confSys := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlockSys := encoder.New(confSys).GenesisBlock()
	_, l := newRAMLedgerAndFactory(10, genesisconfig.TestChainID, genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			LocalSigner: mockCrypto(),
			ReadWriter:  l,
			Validator:   &mockconfigtx.Validator{ChainIDVal: genesisconfig.TestChainID},
			fakeConfig:  fakeConfig,
		},
	)

	ctx := makeConfigTxFull(genesisconfig.TestChainID, 1)
	block := cb.NewBlock(1, genesisBlockSys.Header.Hash())
	block.Data.Data = [][]byte{utils.MarshalOrPanic(ctx)}
	consenterMetadata := []byte("foo")
	bw.WriteConfigBlock(block, consenterMetadata)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block.Header.Number)
	assert.Equal(t, block.Header, cBlock.Header)
	assert.Equal(t, block.Data, cBlock.Data)

	omd := utils.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_ORDERER)
	assert.Equal(t, consenterMetadata, omd.Value)
}

func TestMigrationWriteConfig(t *testing.T) {
	confSys := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlockSys := encoder.New(confSys).GenesisBlock()
	_, l := newRAMLedgerAndFactory(10, genesisconfig.TestChainID, genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")
	fakeConfig.ConsensusStateReturns(orderer.ConsensusType_STATE_MAINTENANCE)
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			LocalSigner: mockCrypto(),
			ReadWriter:  l,
			Validator:   &mockconfigtx.Validator{ChainIDVal: genesisconfig.TestChainID},
			fakeConfig:  fakeConfig,
		},
	)

	ctx := makeConfigTxMig(genesisconfig.TestChainID, 1)
	block := cb.NewBlock(1, genesisBlockSys.Header.Hash())
	block.Data.Data = [][]byte{utils.MarshalOrPanic(ctx)}
	consenterMetadata := []byte("foo")

	bw.WriteConfigBlock(block, consenterMetadata)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block.Header.Number)
	assert.Equal(t, block.Header, cBlock.Header)
	assert.Equal(t, block.Data, cBlock.Data)

	omd := utils.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_ORDERER)
	assert.Equal(t, []byte(nil), omd.Value)
}

func TestRaceWriteConfig(t *testing.T) {
	confSys := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlockSys := encoder.New(confSys).GenesisBlock()
	_, l := newRAMLedgerAndFactory(10, genesisconfig.TestChainID, genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			LocalSigner: mockCrypto(),
			ReadWriter:  l,
			Validator:   &mockconfigtx.Validator{},
			fakeConfig:  fakeConfig,
		},
	)

	ctx := makeConfigTxFull(genesisconfig.TestChainID, 1)
	block1 := cb.NewBlock(1, genesisBlockSys.Header.Hash())
	block1.Data.Data = [][]byte{utils.MarshalOrPanic(ctx)}
	consenterMetadata1 := []byte("foo")

	ctx = makeConfigTxFull(genesisconfig.TestChainID, 1)
	block2 := cb.NewBlock(2, block1.Header.Hash())
	block2.Data.Data = [][]byte{utils.MarshalOrPanic(ctx)}
	consenterMetadata2 := []byte("bar")

	bw.WriteConfigBlock(block1, consenterMetadata1)
	bw.WriteConfigBlock(block2, consenterMetadata2)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block1.Header.Number)
	assert.Equal(t, block1.Header, cBlock.Header)
	assert.Equal(t, block1.Data, cBlock.Data)
	expectedLastConfigBlockNumber := block1.Header.Number
	testLastConfigBlockNumber(t, block1, expectedLastConfigBlockNumber)

	cBlock = blockledger.GetBlock(l, block2.Header.Number)
	assert.Equal(t, block2.Header, cBlock.Header)
	assert.Equal(t, block2.Data, cBlock.Data)
	expectedLastConfigBlockNumber = block2.Header.Number
	testLastConfigBlockNumber(t, block2, expectedLastConfigBlockNumber)

	omd := utils.GetMetadataFromBlockOrPanic(block1, cb.BlockMetadataIndex_ORDERER)
	assert.Equal(t, consenterMetadata1, omd.Value)
}
