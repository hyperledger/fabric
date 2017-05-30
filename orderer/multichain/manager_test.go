/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multichain

import (
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

var conf *genesisconfig.Profile
var genesisBlock = cb.NewBlock(0, nil) // *cb.Block
var mockSigningIdentity msp.SigningIdentity

func init() {
	conf = genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	logging.SetLevel(logging.DEBUG, "")
	genesisBlock = provisional.New(conf).GenesisBlock()
	mockSigningIdentity, _ = mmsp.NewNoopMsp().GetDefaultSigningIdentity()
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

	if !reflect.DeepEqual(ctx, pctx) {
		t.Fatalf("Did not select most recent config transaction")
	}
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
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panic-ed because there was no config tx")
		}
	}()
	getConfigTx(rl)

}

// This test checks to make sure the orderer refuses to come up if it cannot find a system channel
func TestNoSystemChain(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panicked when starting without a system chain")
		}
	}()

	lf := ramledger.New(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	NewManagerImpl(lf, consenters, mockCrypto())
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

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	_, ok := manager.GetChain("Fake")
	if ok {
		t.Errorf("Should not have found a chain that was not created")
	}

	chainSupport, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

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
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block")
		}
		for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}

func TestNewChannelConfig(t *testing.T) {

	lf, _ := NewRAMLedgerAndFactory(3)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}
	manager := NewManagerImpl(lf, consenters, mockCrypto())

	t.Run("BadPayload", func(t *testing.T) {
		_, err := manager.NewChannelConfig(&cb.Envelope{Payload: []byte("bad payload")})
		assert.Error(t, err, "Should bot be able to create new channel config from bad payload.")
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
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("bad config update envelope data"),
				}),
			},
			"^Failing initial channel config creation because of config update unmarshaling error:",
		},
		{
			"EmptyConfigUpdateWriteSet",
			&cb.Payload{
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

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	expectedLastConfigBlockNumber := uint64(0)
	expectedLastConfigSeq := uint64(1)
	newChainID := "TestNewChain"

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
