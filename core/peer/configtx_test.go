/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestConfigTxCreateLedger(t *testing.T) {
	helper := &testHelper{t: t}

	channelID := "testchain1"
	cleanup, err := ledgermgmt.InitializeTestEnvWithInitializer(
		&ledgermgmt.Initializer{
			CustomTxProcessors: customtx.Processors{
				common.HeaderType_CONFIG: &ConfigTxProcessor{},
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer cleanup()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, channelID, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	ledger, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)

	retrievedchanConf, err := retrievePersistedChannelConfig(ledger)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))
}

func TestConfigTxUpdateChanConfig(t *testing.T) {
	helper := &testHelper{t: t}
	channelID := "testchain1"
	cleanup, err := ledgermgmt.InitializeTestEnvWithInitializer(
		&ledgermgmt.Initializer{
			CustomTxProcessors: customtx.Processors{
				common.HeaderType_CONFIG: &ConfigTxProcessor{},
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer cleanup()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, channelID, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	lgr, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)

	retrievedchanConf, err := retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))

	helper.mockCreateChain(t, channelID, lgr)
	defer helper.clearMockChains()

	bs := Default.channels[channelID].bundleSource
	inMemoryChanConf := bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(inMemoryChanConf))

	retrievedchanConf, err = retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(bs.ConfigtxValidator().ConfigProto()), proto.CompactTextString(retrievedchanConf))

	lgr.Close()
	helper.clearMockChains()
	_, err = ledgermgmt.OpenLedger(channelID)
	assert.NoError(t, err)
}

func TestGenesisBlockCreateLedger(t *testing.T) {
	b, err := configtxtest.MakeGenesisBlock("testchain")
	assert.NoError(t, err)

	cleanup, err := ledgermgmt.InitializeTestEnvWithInitializer(
		&ledgermgmt.Initializer{
			CustomTxProcessors: customtx.Processors{
				common.HeaderType_CONFIG: &ConfigTxProcessor{},
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer cleanup()

	lgr, err := ledgermgmt.CreateLedger(b)
	assert.NoError(t, err)
	chanConf, err := retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.NotNil(t, chanConf)
	t.Logf("chanConf = %s", chanConf)
}

type testHelper struct {
	t *testing.T
}

func (h *testHelper) sampleChannelConfig(sequence uint64, enableV11Capability bool) *common.Config {
	profile := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
	if enableV11Capability {
		profile.Orderer.Capabilities = make(map[string]bool)
		profile.Orderer.Capabilities[capabilities.ApplicationV1_1] = true
		profile.Application.Capabilities = make(map[string]bool)
		profile.Application.Capabilities[capabilities.ApplicationV1_2] = true
	}
	channelGroup, _ := encoder.NewChannelGroup(profile)
	return &common.Config{
		Sequence:     sequence,
		ChannelGroup: channelGroup,
	}
}

func (h *testHelper) constructGenesisTx(t *testing.T, channelID string, chanConf *common.Config) *common.Envelope {
	configEnvelop := &common.ConfigEnvelope{
		Config:     chanConf,
		LastUpdate: h.constructLastUpdateField(channelID),
	}
	txEnvelope, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG, channelID, nil, configEnvelop, 0, 0)
	assert.NoError(t, err)
	return txEnvelope
}

func (h *testHelper) constructBlock(txEnvelope *common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	return testutil.NewBlock([]*common.Envelope{txEnvelope}, blockNum, previousHash)
}

func (h *testHelper) constructLastUpdateField(channelID string) *common.Envelope {
	configUpdate := protoutil.MarshalOrPanic(&common.ConfigUpdate{
		ChannelId: channelID,
	})
	envelopeForLastUpdateField, _ := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, channelID, nil, &common.ConfigUpdateEnvelope{ConfigUpdate: configUpdate}, 0, 0)
	return envelopeForLastUpdateField
}

func (h *testHelper) mockCreateChain(t *testing.T, channelID string, ledger ledger.PeerLedger) {
	chanBundle, err := h.constructChannelBundle(channelID, ledger)
	assert.NoError(t, err)
	if Default.channels == nil {
		Default.channels = map[string]*Channel{}
	}
	Default.channels[channelID] = &Channel{
		bundleSource: channelconfig.NewBundleSource(chanBundle),
		ledger:       ledger,
	}
}

func (h *testHelper) clearMockChains() {
	Default.channels = make(map[string]*Channel)
}

func (h *testHelper) constructChannelBundle(channelID string, ledger ledger.PeerLedger) (*channelconfig.Bundle, error) {
	chanConf, err := retrievePersistedChannelConfig(ledger)
	if err != nil {
		return nil, err
	}

	return channelconfig.NewBundle(channelID, chanConf)
}
