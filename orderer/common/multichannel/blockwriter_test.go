/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	newchannelconfig "github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter/mock"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/configtx_validator.go --fake-name ConfigTXValidator . configtxValidator

type configtxValidator interface {
	configtx.Validator
}

type mockBlockWriterSupport struct {
	*mocks.ConfigTXValidator
	identity.SignerSerializer
	blockledger.ReadWriter
	fakeConfig *mock.OrdererConfig
	bccsp      bccsp.BCCSP
}

func (mbws mockBlockWriterSupport) Update(bundle *newchannelconfig.Bundle) {}

func (mbws mockBlockWriterSupport) CreateBundle(channelID string, config *cb.Config) (*newchannelconfig.Bundle, error) {
	return channelconfig.NewBundle(channelID, config, mbws.bccsp)
}

func (mbws mockBlockWriterSupport) SharedConfig() newchannelconfig.Orderer {
	return mbws.fakeConfig
}

func TestCreateBlock(t *testing.T) {
	seedBlock := protoutil.NewBlock(7, []byte("lasthash"))
	seedBlock.Data.Data = [][]byte{[]byte("somebytes")}

	bw := &BlockWriter{lastBlock: seedBlock}
	block := bw.CreateNextBlock([]*cb.Envelope{
		{Payload: []byte("some other bytes")},
	})

	require.Equal(t, seedBlock.Header.Number+1, block.Header.Number)
	require.Equal(t, protoutil.BlockDataHash(block.Data), block.Header.DataHash)
	require.Equal(t, protoutil.BlockHeaderHash(seedBlock.Header), block.Header.PreviousHash)
}

func TestBlockSignature(t *testing.T) {
	dir, err := ioutil.TempDir("", "file-ledger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	rlf, err := fileledger.New(dir, &disabled.Provider{})
	require.NoError(t, err)

	l, err := rlf.GetOrCreate("mychannel")
	require.NoError(t, err)
	lastBlock := protoutil.NewBlock(0, nil)
	l.Append(lastBlock)

	bw := &BlockWriter{
		lastConfigBlockNum: 42,
		support: &mockBlockWriterSupport{
			SignerSerializer:  mockCrypto(),
			ConfigTXValidator: &mocks.ConfigTXValidator{},
			ReadWriter:        l,
		},
		lastBlock: protoutil.NewBlock(1, protoutil.BlockHeaderHash(lastBlock.Header)),
	}

	consensusMetadata := []byte("bar")
	bw.commitBlock(consensusMetadata)

	it, seq := l.Iterator(&orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{}})
	require.Equal(t, uint64(1), seq)
	committedBlock, status := it.Next()
	require.Equal(t, cb.Status_SUCCESS, status)

	md := protoutil.GetMetadataFromBlockOrPanic(committedBlock, cb.BlockMetadataIndex_SIGNATURES)

	expectedMetadataValue := protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
		LastConfig:        &cb.LastConfig{Index: 42},
		ConsenterMetadata: protoutil.MarshalOrPanic(&cb.Metadata{Value: consensusMetadata}),
	})

	require.Equal(t, expectedMetadataValue, md.Value, "Value contains the consensus metadata and the last config")
	require.NotNil(t, md.Signatures, "Should have signature")
}

func TestBlockLastConfig(t *testing.T) {
	lastConfigSeq := uint64(6)
	newConfigSeq := lastConfigSeq + 1
	newBlockNum := uint64(9)

	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.SequenceReturns(newConfigSeq)
	bw := &BlockWriter{
		support: &mockBlockWriterSupport{
			SignerSerializer:  mockCrypto(),
			ConfigTXValidator: mockValidator,
		},
		lastConfigSeq: lastConfigSeq,
	}

	block := protoutil.NewBlock(newBlockNum, []byte("foo"))
	bw.addLastConfig(block)

	require.Equal(t, newBlockNum, bw.lastConfigBlockNum)
	require.Equal(t, newConfigSeq, bw.lastConfigSeq)

	md := protoutil.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_LAST_CONFIG)
	require.NotNil(t, md.Value, "Value not be empty in this case")
	require.Nil(t, md.Signatures, "Should not have signature")

	lc := protoutil.GetLastConfigIndexFromBlockOrPanic(block)
	require.Equal(t, newBlockNum, lc)
}

func TestWriteConfigBlock(t *testing.T) {
	// TODO, use assert.PanicsWithValue once available
	t.Run("EmptyBlock", func(t *testing.T) {
		require.Panics(t, func() { (&BlockWriter{}).WriteConfigBlock(&cb.Block{}, nil) })
	})
	t.Run("BadPayload", func(t *testing.T) {
		require.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						protoutil.MarshalOrPanic(&cb.Envelope{Payload: []byte("bad")}),
					},
				},
			}, nil)
		})
	})
	t.Run("MissingHeader", func(t *testing.T) {
		require.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						protoutil.MarshalOrPanic(&cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{}),
						}),
					},
				},
			}, nil)
		})
	})
	t.Run("BadChannelHeader", func(t *testing.T) {
		require.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						protoutil.MarshalOrPanic(&cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{
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
		require.Panics(t, func() {
			(&BlockWriter{}).WriteConfigBlock(&cb.Block{
				Data: &cb.BlockData{
					Data: [][]byte{
						protoutil.MarshalOrPanic(&cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{}),
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
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	tmpdir, err := ioutil.TempDir("", "file-ledger")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	_, l := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ChannelIDReturns("testchannelid")
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			SignerSerializer:  mockCrypto(),
			ReadWriter:        l,
			ConfigTXValidator: mockValidator,
			fakeConfig:        fakeConfig,
			bccsp:             cryptoProvider,
		},
	)

	ctx := makeConfigTxFull("testchannelid", 1)
	block := protoutil.NewBlock(1, protoutil.BlockHeaderHash(genesisBlockSys.Header))
	block.Data.Data = [][]byte{protoutil.MarshalOrPanic(ctx)}
	consenterMetadata := []byte("foo")
	bw.WriteConfigBlock(block, consenterMetadata)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block.Header.Number)
	require.Equal(t, block.Header, cBlock.Header)
	require.Equal(t, block.Data, cBlock.Data)

	omd, err := protoutil.GetConsenterMetadataFromBlock(block)
	require.NoError(t, err)
	require.Equal(t, consenterMetadata, omd.Value)
}

func TestMigrationWriteConfig(t *testing.T) {
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	tmpdir, err := ioutil.TempDir("", "file-ledger")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	_, l := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")
	fakeConfig.ConsensusStateReturns(orderer.ConsensusType_STATE_MAINTENANCE)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ChannelIDReturns("testchannelid")
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			SignerSerializer:  mockCrypto(),
			ReadWriter:        l,
			ConfigTXValidator: mockValidator,
			fakeConfig:        fakeConfig,
			bccsp:             cryptoProvider,
		},
	)

	ctx := makeConfigTxMig("testchannelid", 1)
	block := protoutil.NewBlock(1, protoutil.BlockHeaderHash(genesisBlockSys.Header))
	block.Data.Data = [][]byte{protoutil.MarshalOrPanic(ctx)}
	consenterMetadata := []byte("foo")

	bw.WriteConfigBlock(block, consenterMetadata)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block.Header.Number)
	require.Equal(t, block.Header, cBlock.Header)
	require.Equal(t, block.Data, cBlock.Data)

	omd := protoutil.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_ORDERER)
	require.Equal(t, []byte(nil), omd.Value)
}

func TestRaceWriteConfig(t *testing.T) {
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	tmpdir, err := ioutil.TempDir("", "file-ledger")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	_, l := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

	fakeConfig := &mock.OrdererConfig{}
	fakeConfig.ConsensusTypeReturns("solo")

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	mockValidator := &mocks.ConfigTXValidator{}
	bw := newBlockWriter(genesisBlockSys, nil,
		&mockBlockWriterSupport{
			SignerSerializer:  mockCrypto(),
			ReadWriter:        l,
			ConfigTXValidator: mockValidator,
			fakeConfig:        fakeConfig,
			bccsp:             cryptoProvider,
		},
	)

	ctx := makeConfigTxFull("testchannelid", 1)
	block1 := protoutil.NewBlock(1, protoutil.BlockHeaderHash(genesisBlockSys.Header))
	block1.Data.Data = [][]byte{protoutil.MarshalOrPanic(ctx)}
	consenterMetadata1 := []byte("foo")
	mockValidator.SequenceReturnsOnCall(1, 1)

	ctx = makeConfigTxFull("testchannelid", 1)
	block2 := protoutil.NewBlock(2, protoutil.BlockHeaderHash(block1.Header))
	block2.Data.Data = [][]byte{protoutil.MarshalOrPanic(ctx)}
	consenterMetadata2 := []byte("bar")
	mockValidator.SequenceReturnsOnCall(2, 2)

	bw.WriteConfigBlock(block1, consenterMetadata1)
	bw.WriteConfigBlock(block2, consenterMetadata2)

	// Wait for the commit to complete
	bw.committingBlock.Lock()
	bw.committingBlock.Unlock()

	cBlock := blockledger.GetBlock(l, block1.Header.Number)
	require.Equal(t, block1.Header, cBlock.Header)
	require.Equal(t, block1.Data, cBlock.Data)
	expectedLastConfigBlockNumber := block1.Header.Number
	testLastConfigBlockNumber(t, block1, expectedLastConfigBlockNumber)

	cBlock = blockledger.GetBlock(l, block2.Header.Number)
	require.Equal(t, block2.Header, cBlock.Header)
	require.Equal(t, block2.Data, cBlock.Data)
	expectedLastConfigBlockNumber = block2.Header.Number
	testLastConfigBlockNumber(t, block2, expectedLastConfigBlockNumber)

	omd, err := protoutil.GetConsenterMetadataFromBlock(block1)
	require.NoError(t, err)
	require.Equal(t, consenterMetadata1, omd.Value)
}

func testLastConfigBlockNumber(t *testing.T, block *cb.Block, expectedBlockNumber uint64) {
	metadata := &cb.Metadata{}
	err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], metadata)
	require.NoError(t, err, "Block should carry SIGNATURES metadata item")
	obm := &cb.OrdererBlockMetadata{}
	err = proto.Unmarshal(metadata.Value, obm)
	require.NoError(t, err, "Block SIGNATURES should carry OrdererBlockMetadata")
	require.Equal(t, expectedBlockNumber, obm.LastConfig.Index, "SIGNATURES value should point to last config block")

	metadata = &cb.Metadata{}
	err = proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG], metadata)
	require.NoError(t, err, "Block should carry LAST_CONFIG metadata item")
	lastConfig := &cb.LastConfig{}
	err = proto.Unmarshal(metadata.Value, lastConfig)
	require.NoError(t, err, "LAST_CONFIG metadata item should carry last config value")
	require.Equal(t, expectedBlockNumber, lastConfig.Index, "LAST_CONFIG value should point to last config block")
}
