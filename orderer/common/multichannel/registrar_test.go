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
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/resources.go --fake-name Resources . resources

type resources interface {
	channelconfig.Resources
}

//go:generate counterfeiter -o mocks/orderer_config.go --fake-name OrdererConfig . ordererConfig

type ordererConfig interface {
	channelconfig.Orderer
}

//go:generate counterfeiter -o mocks/orderer_capabilities.go --fake-name OrdererCapabilities . ordererCapabilities

type ordererCapabilities interface {
	channelconfig.OrdererCapabilities
}

//go:generate counterfeiter -o mocks/channel_config.go --fake-name ChannelConfig . channelConfig

type channelConfig interface {
	channelconfig.Channel
}

//go:generate counterfeiter -o mocks/channel_capabilities.go --fake-name ChannelCapabilities . channelCapabilities

type channelCapabilities interface {
	channelconfig.ChannelCapabilities
}

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

func mockCrypto() *mocks.SignerSerializer {
	return &mocks.SignerSerializer{}
}

func newLedgerAndFactory(dir string, chainID string, genesisBlockSys *cb.Block) (blockledger.Factory, blockledger.ReadWriter) {
	rlf, err := fileledger.New(dir, &disabled.Provider{})
	if err != nil {
		panic(err)
	}

	rl, err := rlf.GetOrCreate(chainID)
	if err != nil {
		panic(err)
	}

	if genesisBlockSys != nil {
		err = rl.Append(genesisBlockSys)
		if err != nil {
			panic(err)
		}
	}
	return rlf, rl
}

func testMessageOrderAndRetrieval(maxMessageCount uint32, chainID string, chainSupport *ChainSupport, lr blockledger.ReadWriter, t *testing.T) {
	messages := make([]*cb.Envelope, maxMessageCount)
	for i := uint32(0); i < maxMessageCount; i++ {
		messages[i] = makeNormalTx(chainID, int(i))
	}
	for _, message := range messages {
		chainSupport.Order(message, 0)
	}
	it, _ := lr.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	defer it.Close()
	block, status := it.Next()
	assert.Equal(t, cb.Status_SUCCESS, status, "Could not retrieve block")
	for i := uint32(0); i < maxMessageCount; i++ {
		assert.True(t, proto.Equal(messages[i], protoutil.ExtractEnvelopeOrPanic(block, int(i))), "Block contents wrong at index %d", i)
	}
}

func TestConfigTx(t *testing.T) {
	//system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	// Tests for a normal chain which contains 3 config transactions and other normal transactions to make
	// sure the right one returned
	t.Run("GetConfigTx - ok", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		_, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)
		for i := 0; i < 5; i++ {
			rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx("testchannelid", i)}))
		}
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx("testchannelid", 5)}))
		ctx := makeConfigTx("testchannelid", 6)
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

		block := blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx("testchannelid", 7)})
		block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: 7})})
		rl.Append(block)

		pctx := configTx(rl)
		assert.True(t, proto.Equal(pctx, ctx), "Did not select most recent config transaction")
	})
}

func TestNewRegistrar(t *testing.T) {
	//system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	// This test checks to make sure the orderer refuses to come up if it cannot find a system channel
	t.Run("No system chain - failure", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, err := fileledger.New(tmpdir, &disabled.Provider{})
		require.NoError(t, err)

		consenters := make(map[string]consensus.Consenter)
		consenters[confSys.Orderer.OrdererType] = &mockConsenter{}

		assert.NotPanics(t, func() {
			NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider).Initialize(consenters)
		}, "Should not panic when starting without a system channel")
	})

	// This test checks to make sure that the orderer refuses to come up if there are multiple system channels
	t.Run("Multiple system chains - failure", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, err := fileledger.New(tmpdir, &disabled.Provider{})
		require.NoError(t, err)

		for _, id := range []string{"foo", "bar"} {
			rl, err := lf.GetOrCreate(id)
			assert.NoError(t, err)

			err = rl.Append(encoder.New(confSys).GenesisBlockForChannel(id))
			assert.NoError(t, err)
		}

		consenters := make(map[string]consensus.Consenter)
		consenters[confSys.Orderer.OrdererType] = &mockConsenter{}

		assert.Panics(t, func() {
			NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider).Initialize(consenters)
		}, "Two system channels should have caused panic")
	})

	// This test essentially brings the entire system up and is ultimately what main.go will replicate
	t.Run("Correct flow", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenters := make(map[string]consensus.Consenter)
		consenters[confSys.Orderer.OrdererType] = &mockConsenter{}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider)
		manager.Initialize(consenters)

		chainSupport := manager.GetChain("Fake")
		assert.Nilf(t, chainSupport, "Should not have found a chain that was not created")

		chainSupport = manager.GetChain("testchannelid")
		assert.NotNilf(t, chainSupport, "Should have gotten chain which was initialized by ledger")

		testMessageOrderAndRetrieval(confSys.Orderer.BatchSize.MaxMessageCount, "testchannelid", chainSupport, rl, t)
	})
}

func TestCreateChain(t *testing.T) {
	//system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	t.Run("Create chain", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, _ := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenters := make(map[string]consensus.Consenter)
		consenters[confSys.Orderer.OrdererType] = &mockConsenter{}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider)
		manager.Initialize(consenters)

		ledger, err := lf.GetOrCreate("mychannel")
		assert.NoError(t, err)

		genesisBlock := encoder.New(confSys).GenesisBlockForChannel("mychannel")
		ledger.Append(genesisBlock)

		// Before creating the chain, it doesn't exist
		assert.Nil(t, manager.GetChain("mychannel"))
		// After creating the chain, it exists
		manager.CreateChain("mychannel")
		chain := manager.GetChain("mychannel")
		assert.NotNil(t, chain)
		// A subsequent creation, replaces the chain.
		manager.CreateChain("mychannel")
		chain2 := manager.GetChain("mychannel")
		assert.NotNil(t, chain2)
		// They are not the same
		assert.NotEqual(t, chain, chain2)
		// The old chain is halted
		_, ok := <-chain.Chain.(*mockChain).queue
		assert.False(t, ok)
		// The new chain is not halted: Close the channel to prove that.
		close(chain2.Chain.(*mockChain).queue)
	})

	// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
	t.Run("New chain", func(t *testing.T) {
		expectedLastConfigBlockNumber := uint64(0)
		expectedLastConfigSeq := uint64(1)
		newChainID := "test-new-chain"

		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenters := make(map[string]consensus.Consenter)
		consenters[confSys.Orderer.OrdererType] = &mockConsenter{}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider)
		manager.Initialize(consenters)
		orglessChannelConf := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
		orglessChannelConf.Application.Organizations = nil
		envConfigUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, mockCrypto(), orglessChannelConf)
		assert.NoError(t, err, "Constructing chain creation tx")

		res, err := manager.NewChannelConfig(envConfigUpdate)
		assert.NoError(t, err, "Constructing initial channel config")

		configEnv, err := res.ConfigtxValidator().ProposeConfigUpdate(envConfigUpdate)
		assert.NoError(t, err, "Proposing initial update")
		assert.Equal(t, expectedLastConfigSeq, configEnv.GetConfig().Sequence, "Sequence of config envelope for new channel should always be set to %d", expectedLastConfigSeq)

		ingressTx, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, newChainID, mockCrypto(), configEnv, msgVersion, epoch)
		assert.NoError(t, err, "Creating ingresstx")

		wrapped := wrapConfigTx(ingressTx)

		chainSupport := manager.GetChain(manager.SystemChannelID())
		assert.NotNilf(t, chainSupport, "Could not find system channel")

		chainSupport.Configure(wrapped, 0)
		func() {
			it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
			defer it.Close()
			block, status := it.Next()
			if status != cb.Status_SUCCESS {
				t.Fatalf("Could not retrieve block")
			}
			if len(block.Data.Data) != 1 {
				t.Fatalf("Should have had only one message in the orderer transaction block")
			}

			assert.True(t, proto.Equal(wrapped, protoutil.UnmarshalEnvelopeOrPanic(block.Data.Data[0])), "Orderer config block contains wrong transaction")
		}()

		chainSupport = manager.GetChain(newChainID)
		if chainSupport == nil {
			t.Fatalf("Should have gotten new chain which was created")
		}

		messages := make([]*cb.Envelope, confSys.Orderer.BatchSize.MaxMessageCount)
		for i := 0; i < int(confSys.Orderer.BatchSize.MaxMessageCount); i++ {
			messages[i] = makeNormalTx(newChainID, i)
		}

		for _, message := range messages {
			chainSupport.Order(message, 0)
		}

		it, _ := chainSupport.Reader().Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 0}}})
		defer it.Close()
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve new chain genesis block")
		}
		testLastConfigBlockNumber(t, block, expectedLastConfigBlockNumber)
		if len(block.Data.Data) != 1 {
			t.Fatalf("Should have had only one message in the new genesis block")
		}

		assert.True(t, proto.Equal(ingressTx, protoutil.UnmarshalEnvelopeOrPanic(block.Data.Data[0])), "Genesis block contains wrong transaction")

		block, status = it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block on new chain")
		}
		testLastConfigBlockNumber(t, block, expectedLastConfigBlockNumber)
		for i := 0; i < int(confSys.Orderer.BatchSize.MaxMessageCount); i++ {
			if !proto.Equal(protoutil.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		assert.NoError(t, err)
		rcs := newChainSupport(manager, chainSupport.ledgerResources, consenters, mockCrypto(), blockcutter.NewMetrics(&disabled.Provider{}), cryptoProvider)
		assert.Equal(t, expectedLastConfigSeq, rcs.lastConfigSeq, "On restart, incorrect lastConfigSeq")
	})
}

func testLastConfigBlockNumber(t *testing.T, block *cb.Block, expectedBlockNumber uint64) {
	metadataItem := &cb.Metadata{}
	err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG], metadataItem)
	assert.NoError(t, err, "Block should carry LAST_CONFIG metadata item")
	lastConfig := &cb.LastConfig{}
	err = proto.Unmarshal(metadataItem.Value, lastConfig)
	assert.NoError(t, err, "LAST_CONFIG metadata item should carry last config value")
	assert.Equal(t, expectedBlockNumber, lastConfig.Index, "LAST_CONFIG value should point to last config block")
}

func TestResourcesCheck(t *testing.T) {
	mockOrderer := &mocks.OrdererConfig{}
	mockOrdererCaps := &mocks.OrdererCapabilities{}
	mockOrderer.CapabilitiesReturns(mockOrdererCaps)
	mockChannel := &mocks.ChannelConfig{}
	mockChannelCaps := &mocks.ChannelCapabilities{}
	mockChannel.CapabilitiesReturns(mockChannelCaps)

	mockResources := &mocks.Resources{}
	mockResources.PolicyManagerReturns(&policies.ManagerImpl{})

	t.Run("GoodResources", func(t *testing.T) {
		mockResources.OrdererConfigReturns(mockOrderer, true)
		mockResources.ChannelConfigReturns(mockChannel)

		err := checkResources(mockResources)
		assert.NoError(t, err)
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		mockResources.OrdererConfigReturns(nil, false)

		err := checkResources(mockResources)
		assert.Error(t, err)
		assert.Regexp(t, "config does not contain orderer config", err.Error())
	})

	t.Run("MissingOrdererCapability", func(t *testing.T) {
		mockResources.OrdererConfigReturns(mockOrderer, true)
		mockOrdererCaps.SupportedReturns(errors.New("An error"))

		err := checkResources(mockResources)
		assert.Error(t, err)
		assert.Regexp(t, "config requires unsupported orderer capabilities:", err.Error())

		// reset
		mockOrdererCaps.SupportedReturns(nil)
	})

	t.Run("MissingChannelCapability", func(t *testing.T) {
		mockChannelCaps.SupportedReturns(errors.New("An error"))

		err := checkResources(mockResources)
		assert.Error(t, err)
		assert.Regexp(t, "config requires unsupported channel capabilities:", err.Error())
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		mockResources.OrdererConfigReturns(nil, false)

		assert.Panics(t, func() {
			checkResourcesOrPanic(mockResources)
		})
	})
}

// The registrar's BroadcastChannelSupport implementation should reject message types which should not be processed directly.
func TestBroadcastChannelSupport(t *testing.T) {
	// system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	t.Run("Rejection", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		ledgerFactory, _ := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)
		mockConsenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: &mockConsenter{}}
		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider)
		registrar.Initialize(mockConsenters)
		randomValue := 1
		configTx := makeConfigTx("testchannelid", randomValue)
		_, _, _, err = registrar.BroadcastChannelSupport(configTx)
		assert.Error(t, err, "Messages of type HeaderType_CONFIG should return an error.")
	})

	t.Run("No system channel", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		ledgerFactory, _ := newLedgerAndFactory(tmpdir, "", nil)
		mockConsenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: &mockConsenter{}}
		config := localconfig.TopLevel{}
		config.General.BootstrapMethod = "none"
		config.General.GenesisFile = ""
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider)
		registrar.Initialize(mockConsenters)
		configTx := makeConfigTxFull("testchannelid", 1)
		_, _, _, err = registrar.BroadcastChannelSupport(configTx)
		assert.Error(t, err)
		assert.Equal(t, "channel creation request not allowed because the orderer system channel is not yet defined", err.Error())
	})
}
