/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichain

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	mockcrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/ledger"
	ramledger "github.com/hyperledger/fabric/orderer/ledger/ram"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

var conf, singleMSPConf, noConsortiumConf *genesisconfig.Profile
var genesisBlock, singleMSPGenesisBlock, noConsortiumGenesisBlock *cb.Block
var mockSigningIdentity msp.SigningIdentity

const NoConsortiumChain = "no-consortium-chain"

func init() {
	logging.SetLevel(logging.DEBUG, "")
	mockSigningIdentity, _ = mmsp.NewNoopMsp().GetDefaultSigningIdentity()

	conf = genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	genesisBlock = provisional.New(conf).GenesisBlock()

	singleMSPConf = genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile)
	singleMSPGenesisBlock = provisional.New(singleMSPConf).GenesisBlock()

	noConsortiumConf = genesisconfig.Load("SampleNoConsortium")
	noConsortiumGenesisBlock = provisional.New(noConsortiumConf).GenesisBlockForChannel(NoConsortiumChain)
}

func mockCrypto() *mockCryptoHelper {
	return &mockCryptoHelper{LocalSigner: mockcrypto.FakeLocalSigner}
}

type mockCryptoHelper struct {
	*mockcrypto.LocalSigner
}

func (mch mockCryptoHelper) VerifySignature(sd *cb.SignedData) error {
	return nil
}

func NewRAMLedgerAndFactory(maxSize int) (ledger.Factory, ledger.ReadWriter) {
	rlf := ramledger.New(10)
	rl, err := rlf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf, rl
}

func NewRAMLedgerAndFactoryWithMSP() (ledger.Factory, ledger.ReadWriter) {
	rlf := ramledger.New(10)

	rl, err := rlf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(singleMSPGenesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf, rl
}

func NewRAMLedger(maxSize int) ledger.ReadWriter {
	_, rl := NewRAMLedgerAndFactory(maxSize)
	return rl
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 5; i++ {
		rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, i)}))
	}
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx(provisional.TestChainID, 5)}))
	ctx := makeConfigTx(provisional.TestChainID, 6)
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

	block := ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 7)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&cb.Metadata{Value: utils.MarshalOrPanic(&cb.LastConfig{Index: 7})})
	rl.Append(block)

	pctx := getConfigTx(rl)
	assert.Equal(t, pctx, ctx, "Did not select most recent config transaction")
}

// Tests a chain which contains blocks with multi-transactions mixed with config txs, and a single tx which is not a config tx, none count as config blocks so nil should return
func TestGetConfigTxFailure(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 10; i++ {
		rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{
			makeNormalTx(provisional.TestChainID, i),
			makeConfigTx(provisional.TestChainID, i),
		}))
	}
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 11)}))
	assert.Panics(t, func() { getConfigTx(rl) }, "Should have panicked because there was no config tx")

	block := ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 12)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = []byte("bad metadata")
	assert.Panics(t, func() { getConfigTx(rl) }, "Should have panicked because of bad last config metadata")
}

// This test checks to make sure the orderer refuses to come up if it cannot find a system channel
func TestNoSystemChain(t *testing.T) {
	lf := ramledger.New(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	assert.Panics(t, func() { NewManagerImpl(lf, consenters, mockCrypto()) }, "Should have panicked when starting without a system chain")
}

// This test checks to make sure that the orderer refuses to come up if there are multiple system channels
func TestMultiSystemChannel(t *testing.T) {
	lf := ramledger.New(10)

	for _, id := range []string{"foo", "bar"} {
		rl, err := lf.GetOrCreate(id)
		assert.NoError(t, err)

		err = rl.Append(provisional.New(conf).GenesisBlockForChannel(id))
		assert.NoError(t, err)
	}

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	assert.Panics(t, func() { NewManagerImpl(lf, consenters, mockCrypto()) }, "Two system channels should have caused panic")
}

// This test checks to make sure that the orderer creates different type of filters given different type of channel
func TestFilterCreation(t *testing.T) {
	lf := ramledger.New(10)
	rl, err := lf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}

	// Creating a non-system chain to test that NewManagerImpl could handle the diversity
	rl, err = lf.GetOrCreate(NoConsortiumChain)
	if err != nil {
		panic(err)
	}
	err = rl.Append(noConsortiumGenesisBlock)
	if err != nil {
		panic(err)
	}

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	_, ok := manager.GetChain(provisional.TestChainID)
	assert.True(t, ok, "Should have found chain: %d", provisional.TestChainID)

	chainSupport, ok := manager.GetChain(NoConsortiumChain)
	assert.True(t, ok, "Should have retrieved chain: %d", NoConsortiumChain)

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
		messages[i] = &cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						// For testing purpose, we are injecting configTx into non-system channel.
						// Set Type to HeaderType_ORDERER_TRANSACTION to verify this message is NOT
						// filtered by SystemChainFilter, so we know we are creating correct type
						// of filter for the chain.
						Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						ChannelId: NoConsortiumChain,
					}),
					SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
				},
				Data: []byte(fmt.Sprintf("%d", i)),
			}),
		}

		assert.True(t, chainSupport.Enqueue(messages[i]), "Should have successfully enqueued message")
	}

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
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

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	_, ok := manager.GetChain("Fake")
	assert.False(t, ok, "Should not have found a chain that was not created")

	chainSupport, ok := manager.GetChain(provisional.TestChainID)
	assert.True(t, ok, "Should have gotten chain which was initialized by ramledger")

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(provisional.TestChainID, i)
	}

	for _, message := range messages {
		chainSupport.Enqueue(message)
	}

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
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

func TestNewChannelConfig(t *testing.T) {
	lf, _ := NewRAMLedgerAndFactoryWithMSP()

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}
	manager := NewManagerImpl(lf, consenters, mockCrypto())

	t.Run("BadPayload", func(t *testing.T) {
		_, err := manager.NewChannelConfig(&cb.Envelope{Payload: []byte("bad payload")})
		assert.Error(t, err, "Should not be able to create new channel config from bad payload.")
	})

	for _, tc := range []struct {
		name    string
		payload *cb.Payload
		regex   string
	}{
		{
			"BadPayloadData",
			&cb.Payload{
				Data: []byte("bad payload data"),
			},
			"^Failing initial channel config creation because of config update envelope unmarshaling error:",
		},
		{
			"BadConfigUpdate",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("bad config update envelope data"),
				}),
			},
			"^Failing initial channel config creation because of config update unmarshaling error:",
		},
		{
			"EmptyConfigUpdateWriteSet",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{},
					),
				}),
			},
			"^Config update has an empty writeset$",
		},
		{
			"WriteSetNoGroups",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"WriteSetNoApplicationGroup",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{},
							},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"BadWriteSetApplicationGroupVersion",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 100,
									},
								},
							},
						},
					),
				}),
			},
			"^Config update for channel creation does not set application group version to 1,",
		},
		{
			"MissingWriteSetConsortiumValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{},
							},
						},
					),
				}),
			},
			"^Consortium config value missing$",
		},
		{
			"BadWriteSetConsortiumValueValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									config.ConsortiumKey: &cb.ConfigValue{
										Value: []byte("bad consortium value"),
									},
								},
							},
						},
					),
				}),
			},
			"^Error reading unmarshaling consortium name:",
		},
		{
			"UnknownConsortiumName",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									config.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: "NotTheNameYouAreLookingFor",
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"^Unknown consortium name:",
		},
		{
			"Missing consortium members",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									config.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Proposed configuration has no application group members, but consortium contains members",
		},
		{
			"Member not in consortium",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									config.ApplicationGroupKey: &cb.ConfigGroup{
										Version: 1,
										Groups: map[string]*cb.ConfigGroup{
											"BadOrgName": &cb.ConfigGroup{},
										},
									},
								},
								Values: map[string]*cb.ConfigValue{
									config.ConsortiumKey: &cb.ConfigValue{
										Value: utils.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Attempted to include a member which is not in the consortium",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.NewChannelConfig(&cb.Envelope{Payload: utils.MarshalOrPanic(tc.payload)})
			if assert.Error(t, err) {
				assert.Regexp(t, tc.regex, err.Error())
			}
		})
	}
	// SampleConsortium
}

func TestMismatchedChannelIDs(t *testing.T) {
	innerChannelID := "foo"
	outerChannelID := "bar"
	template := configtx.NewChainCreationTemplate(genesisconfig.SampleConsortiumName, nil)
	configUpdateEnvelope, err := template.Envelope(innerChannelID)
	createTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, outerChannelID, nil, configUpdateEnvelope, msgVersion, epoch)
	assert.NoError(t, err)

	lf, _ := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	_, err = manager.NewChannelConfig(createTx)
	assert.Error(t, err, "Mismatched channel IDs")
	assert.Regexp(t, "mismatched channel IDs", err.Error())
}

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	expectedLastConfigBlockNumber := uint64(0)
	expectedLastConfigSeq := uint64(1)
	newChainID := "test-new-chain"

	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	envConfigUpdate, err := configtx.MakeChainCreationTransaction(newChainID, genesisconfig.SampleConsortiumName, mockSigningIdentity)
	assert.NoError(t, err, "Constructing chain creation tx")

	cm, err := manager.NewChannelConfig(envConfigUpdate)
	assert.NoError(t, err, "Constructing initial channel config")

	configEnv, err := cm.ProposeConfigUpdate(envConfigUpdate)
	assert.NoError(t, err, "Proposing initial update")
	assert.Equal(t, expectedLastConfigSeq, configEnv.GetConfig().Sequence, "Sequence of config envelope for new channel should always be set to %d", expectedLastConfigSeq)

	ingressTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, newChainID, mockCrypto(), configEnv, msgVersion, epoch)
	assert.NoError(t, err, "Creating ingresstx")

	wrapped := wrapConfigTx(ingressTx)

	chainSupport, ok := manager.GetChain(manager.SystemChannelID())
	assert.True(t, ok, "Could not find system channel")

	chainSupport.Enqueue(wrapped)

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
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

	chainSupport, ok = manager.GetChain(newChainID)

	if !ok {
		t.Fatalf("Should have gotten new chain which was created")
	}

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(newChainID, i)
	}

	for _, message := range messages {
		chainSupport.Enqueue(message)
	}

	it, _ = chainSupport.Reader().Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 0}}})
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

	testRestartedChainSupport(t, chainSupport, consenters, expectedLastConfigSeq)
}

func testRestartedChainSupport(t *testing.T, cs ChainSupport, consenters map[string]Consenter, expectedLastConfigSeq uint64) {
	ccs, ok := cs.(*chainSupport)
	assert.True(t, ok, "Casting error")
	rcs := newChainSupport(ccs.filters, ccs.ledgerResources, consenters, mockCrypto())
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
