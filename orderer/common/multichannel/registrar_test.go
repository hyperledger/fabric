/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	ramledger "github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockcrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var conf *genesisconfig.Profile
var genesisBlock *cb.Block
var mockSigningIdentity msp.SigningIdentity

const NoConsortiumChain = "no-consortium-chain"

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
	mockSigningIdentity, _ = mmsp.NewNoopMsp().GetDefaultSigningIdentity()

	conf = configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlock = encoder.New(conf).GenesisBlock()
}

func mockCrypto() crypto.LocalSigner {
	return mockcrypto.FakeLocalSigner
}

func NewRAMLedgerAndFactory(maxSize int) (blockledger.Factory, blockledger.ReadWriter) {
	rlf := ramledger.New(maxSize)
	rl, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf, rl
}

func NewRAMLedger(maxSize int) blockledger.ReadWriter {
	_, rl := NewRAMLedgerAndFactory(maxSize)
	return rl
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 5; i++ {
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(genesisconfig.TestChainID, i)}))
	}
	rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx(genesisconfig.TestChainID, 5)}))
	ctx := makeConfigTx(genesisconfig.TestChainID, 6)
	rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

	block := blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(genesisconfig.TestChainID, 7)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&cb.Metadata{Value: utils.MarshalOrPanic(&cb.LastConfig{Index: 7})})
	rl.Append(block)

	pctx := getConfigTx(rl)
	assert.Equal(t, pctx, ctx, "Did not select most recent config transaction")
}

// Tests a chain which contains blocks with multi-transactions mixed with config txs, and a single tx which is not a config tx, none count as config blocks so nil should return
func TestGetConfigTxFailure(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 10; i++ {
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{
			makeNormalTx(genesisconfig.TestChainID, i),
			makeConfigTx(genesisconfig.TestChainID, i),
		}))
	}
	rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(genesisconfig.TestChainID, 11)}))
	assert.Panics(t, func() { getConfigTx(rl) }, "Should have panicked because there was no config tx")

	block := blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(genesisconfig.TestChainID, 12)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = []byte("bad metadata")
	assert.Panics(t, func() { getConfigTx(rl) }, "Should have panicked because of bad last config metadata")
}

// This test checks to make sure the orderer refuses to come up if it cannot find a system channel
func TestNoSystemChain(t *testing.T) {
	lf := ramledger.New(10)

	consenters := make(map[string]consensus.Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	assert.Panics(t, func() { NewRegistrar(lf, consenters, mockCrypto()) }, "Should have panicked when starting without a system chain")
}

// This test checks to make sure that the orderer refuses to come up if there are multiple system channels
func TestMultiSystemChannel(t *testing.T) {
	lf := ramledger.New(10)

	for _, id := range []string{"foo", "bar"} {
		rl, err := lf.GetOrCreate(id)
		assert.NoError(t, err)

		err = rl.Append(encoder.New(conf).GenesisBlockForChannel(id))
		assert.NoError(t, err)
	}

	consenters := make(map[string]consensus.Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	assert.Panics(t, func() { NewRegistrar(lf, consenters, mockCrypto()) }, "Two system channels should have caused panic")
}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]consensus.Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewRegistrar(lf, consenters, mockCrypto())

	_, ok := manager.GetChain("Fake")
	assert.False(t, ok, "Should not have found a chain that was not created")

	chainSupport, ok := manager.GetChain(genesisconfig.TestChainID)
	assert.True(t, ok, "Should have gotten chain which was initialized by ramledger")

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(genesisconfig.TestChainID, i)
	}

	for _, message := range messages {
		chainSupport.Order(message, 0)
	}

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	defer it.Close()
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		assert.Equal(t, cb.Status_SUCCESS, status, "Could not retrieve block")
		for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
			assert.Equal(t, messages[i], utils.ExtractEnvelopeOrPanic(block, i), "Block contents wrong at index %d", i)
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	expectedLastConfigBlockNumber := uint64(0)
	expectedLastConfigSeq := uint64(1)
	newChainID := "test-new-chain"

	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]consensus.Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewRegistrar(lf, consenters, mockCrypto())
	orglessChannelConf := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
	orglessChannelConf.Application.Organizations = nil
	envConfigUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, mockCrypto(), orglessChannelConf)
	assert.NoError(t, err, "Constructing chain creation tx")

	res, err := manager.NewChannelConfig(envConfigUpdate)
	assert.NoError(t, err, "Constructing initial channel config")

	configEnv, err := res.ConfigtxValidator().ProposeConfigUpdate(envConfigUpdate)
	assert.NoError(t, err, "Proposing initial update")
	assert.Equal(t, expectedLastConfigSeq, configEnv.GetConfig().Sequence, "Sequence of config envelope for new channel should always be set to %d", expectedLastConfigSeq)

	ingressTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, newChainID, mockCrypto(), configEnv, msgVersion, epoch)
	assert.NoError(t, err, "Creating ingresstx")

	wrapped := wrapConfigTx(ingressTx)

	chainSupport, ok := manager.GetChain(manager.SystemChannelID())
	assert.True(t, ok, "Could not find system channel")

	chainSupport.Configure(wrapped, 0)
	func() {
		it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
		defer it.Close()
		select {
		case <-it.ReadyChan():
			block, status := it.Next()
			if status != cb.Status_SUCCESS {
				t.Fatalf("Could not retrieve block")
			}
			if len(block.Data.Data) != 1 {
				t.Fatalf("Should have had only one message in the orderer transaction block")
			}

			assert.Equal(t, wrapped, utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]), "Orderer config block contains wrong transaction")
		case <-time.After(time.Second):
			t.Fatalf("Block 1 not produced after timeout in system chain")
		}
	}()

	chainSupport, ok = manager.GetChain(newChainID)

	if !ok {
		t.Fatalf("Should have gotten new chain which was created")
	}

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(newChainID, i)
	}

	for _, message := range messages {
		chainSupport.Order(message, 0)
	}

	it, _ := chainSupport.Reader().Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 0}}})
	defer it.Close()
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve new chain genesis block")
		}
		testLastConfigBlockNumber(t, block, expectedLastConfigBlockNumber)
		if len(block.Data.Data) != 1 {
			t.Fatalf("Should have had only one message in the new genesis block")
		}

		assert.Equal(t, ingressTx, utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]), "Genesis block contains wrong transaction")
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout in system chain")
	}

	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block on new chain")
		}
		testLastConfigBlockNumber(t, block, expectedLastConfigBlockNumber)
		for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout on new chain")
	}

	rcs := newChainSupport(manager, chainSupport.ledgerResources, consenters, mockCrypto())
	assert.Equal(t, expectedLastConfigSeq, rcs.lastConfigSeq, "On restart, incorrect lastConfigSeq")
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
	t.Run("GoodResources", func(t *testing.T) {
		err := checkResources(&mockchannelconfig.Resources{
			PolicyManagerVal: &mockpolicies.Manager{},
			OrdererConfigVal: &mockchannelconfig.Orderer{
				CapabilitiesVal: &mockchannelconfig.OrdererCapabilities{},
			},
			ChannelConfigVal: &mockchannelconfig.Channel{
				CapabilitiesVal: &mockchannelconfig.ChannelCapabilities{},
			},
		})

		assert.NoError(t, err)
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		err := checkResources(&mockchannelconfig.Resources{
			PolicyManagerVal: &mockpolicies.Manager{},
		})

		assert.Error(t, err)
		assert.Regexp(t, "config does not contain orderer config", err.Error())
	})

	t.Run("MissingOrdererCapability", func(t *testing.T) {
		err := checkResources(&mockchannelconfig.Resources{
			PolicyManagerVal: &mockpolicies.Manager{},
			OrdererConfigVal: &mockchannelconfig.Orderer{
				CapabilitiesVal: &mockchannelconfig.OrdererCapabilities{
					SupportedErr: errors.New("An error"),
				},
			},
		})

		assert.Error(t, err)
		assert.Regexp(t, "config requires unsupported orderer capabilities:", err.Error())
	})

	t.Run("MissingChannelCapability", func(t *testing.T) {
		err := checkResources(&mockchannelconfig.Resources{
			PolicyManagerVal: &mockpolicies.Manager{},
			OrdererConfigVal: &mockchannelconfig.Orderer{
				CapabilitiesVal: &mockchannelconfig.OrdererCapabilities{},
			},
			ChannelConfigVal: &mockchannelconfig.Channel{
				CapabilitiesVal: &mockchannelconfig.ChannelCapabilities{
					SupportedErr: errors.New("An error"),
				},
			},
		})

		assert.Error(t, err)
		assert.Regexp(t, "config requires unsupported channel capabilities:", err.Error())
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		assert.Panics(t, func() {
			checkResourcesOrPanic(&mockchannelconfig.Resources{
				PolicyManagerVal: &mockpolicies.Manager{},
			})
		})
	})
}

// The registrar's BroadcastChannelSupport implementation should reject message types which should not be processed directly.
func TestBroadcastChannelSupportRejection(t *testing.T) {
	ledgerFactory, _ := NewRAMLedgerAndFactory(10)
	mockConsenters := map[string]consensus.Consenter{conf.Orderer.OrdererType: &mockConsenter{}}
	registrar := NewRegistrar(ledgerFactory, mockConsenters, mockCrypto())
	randomValue := 1
	configTx := makeConfigTx(genesisconfig.TestChainID, randomValue)
	_, _, _, err := registrar.BroadcastChannelSupport(configTx)
	assert.Error(t, err, "Messages of type HeaderType_CONFIG should return an error.")
}
