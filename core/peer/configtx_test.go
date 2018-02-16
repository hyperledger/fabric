/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	config "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	protospeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfigTxSerializeDeserialize(t *testing.T) {
	helper := &testHelper{}
	config := helper.sampleResourceConfig(0, "cc1", "cc2")
	configBytes, err := serialize(config)
	assert.NoError(t, err)
	deserializedConfig, err := deserialize(configBytes)
	assert.NoError(t, err)
	assert.Equal(t, config, deserializedConfig)
}

func TestConfigTxResConfigCapability(t *testing.T) {
	chainid := "testchain"
	helper := &testHelper{}
	config := helper.sampleChannelConfig(1, false)
	confCapabilityOn, err := isResConfigCapabilityOn(chainid, config)
	assert.NoError(t, err)
	assert.False(t, confCapabilityOn)

	config = helper.sampleChannelConfig(1, true)
	confCapabilityOn, err = isResConfigCapabilityOn(chainid, config)
	assert.NoError(t, err)
	assert.True(t, confCapabilityOn)
}

func TestConfigTxExtractFullConfigFromSeedTx(t *testing.T) {
	helper := &testHelper{}
	chanConf := helper.sampleChannelConfig(1, true)
	resConf := helper.sampleResourceConfig(0, "cc1", "cc2")
	genesisTx := helper.constructGenesisTx(t, "foo", chanConf, resConf)
	payload := utils.UnmarshalPayloadOrPanic(genesisTx.Payload)
	configEnvelope := configtx.UnmarshalConfigEnvelopeOrPanic(payload.Data)
	extractedResConf, err := extractFullConfigFromSeedTx(configEnvelope)
	assert.NoError(t, err)
	assert.Equal(t, resConf, extractedResConf)
}

func TestConfigTxCreateLedger(t *testing.T) {
	helper := &testHelper{}
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	chainid := "testchain1"
	ledgermgmt.InitializeTestEnvWithCustomProcessors(ConfigTxProcessors)
	defer ledgermgmt.CleanupTestEnv()

	chanConf := helper.sampleChannelConfig(1, true)
	resConf := helper.sampleResourceConfig(0, "cc1", "cc2")
	genesisTx := helper.constructGenesisTx(t, chainid, chanConf, resConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	ledger, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)

	retrievedResConf, err := retrievePersistedResourceConfig(ledger)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(resConf), proto.CompactTextString(retrievedResConf))

	retrievedchanConf, err := retrievePersistedChannelConfig(ledger)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))
}

func TestConfigTxUpdateResConfig(t *testing.T) {
	helper := &testHelper{}
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")
	chainid := "testchain1"
	ledgermgmt.InitializeTestEnvWithCustomProcessors(ConfigTxProcessors)
	defer ledgermgmt.CleanupTestEnv()

	chanConf := helper.sampleChannelConfig(1, true)
	resConf := helper.sampleResourceConfig(0, "cc1", "cc2")
	genesisTx := helper.constructGenesisTx(t, chainid, chanConf, resConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	lgr, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)
	retrievedResConf, err := retrievePersistedResourceConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(resConf), proto.CompactTextString(retrievedResConf))

	retrievedchanConf, err := retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))

	helper.mockCreateChain(t, chainid, lgr)
	defer helper.clearMockChains()

	bs := chains.list[chainid].cs.bundleSource
	inMemoryChanConf := bs.ChannelConfig().ConfigtxValidator().ConfigProto()
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(inMemoryChanConf))

	inMemoryResConf := bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, proto.CompactTextString(resConf), proto.CompactTextString(inMemoryResConf))

	resConf1 := helper.sampleResourceConfig(0, "cc1", "cc2", "cc3")
	resConfTx1 := helper.computeDeltaResConfTx(t, chainid, resConf, resConf1)
	blk1 := helper.constructBlock(resConfTx1, 1, genesisBlock.Header.Hash())
	err = lgr.CommitWithPvtData(&ledger.BlockAndPvtData{Block: blk1})
	assert.NoError(t, err)
	inMemoryResConf = bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, uint64(1), inMemoryResConf.Sequence)

	resConf2 := helper.sampleResourceConfig(0, "cc1", "cc2", "cc3", "cc4")
	resConfTx2 := helper.computeDeltaResConfTx(t, chainid, inMemoryResConf, resConf2)
	blk2 := helper.constructBlock(resConfTx2, 2, blk1.Header.Hash())
	err = lgr.CommitWithPvtData(&ledger.BlockAndPvtData{Block: blk2})
	assert.NoError(t, err)
	inMemoryResConf = bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, uint64(2), inMemoryResConf.Sequence)

	retrievedResConf, err = retrievePersistedResourceConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(inMemoryResConf), proto.CompactTextString(retrievedResConf))

	retrievedchanConf, err = retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(bs.ChannelConfig().ConfigtxValidator().ConfigProto()), proto.CompactTextString(retrievedchanConf))

	lgr.Close()
	helper.clearMockChains()
	lgr, err = ledgermgmt.OpenLedger(chainid)
	assert.NoError(t, err)
	helper.mockCreateChain(t, chainid, lgr)
	inMemoryResConf1 := bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, inMemoryResConf, inMemoryResConf1)
}

func TestGenesisBlockCreateLedger(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	b, err := configtxtest.MakeGenesisBlock("testchain")
	assert.NoError(t, err)

	ledgermgmt.InitializeTestEnvWithCustomProcessors(ConfigTxProcessors)
	defer ledgermgmt.CleanupTestEnv()

	lgr, err := ledgermgmt.CreateLedger(b)
	assert.NoError(t, err)
	chanConf, err := retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.NotNil(t, chanConf)
	t.Logf("chanConf = %s", chanConf)
}

type testHelper struct {
}

func (h *testHelper) sampleResourceConfig(version uint64, entries ...string) *common.Config {
	modPolicy := "/Channel/Application/Admins"
	ccGroups := make(map[string]*common.ConfigGroup)
	for _, entry := range entries {
		ccGroups[entry] = &common.ConfigGroup{ModPolicy: modPolicy}
	}
	return &common.Config{
		ChannelGroup: &common.ConfigGroup{
			Version: version,
			Groups: map[string]*common.ConfigGroup{
				resourcesconfig.ChaincodesGroupKey: {
					Groups:    ccGroups,
					ModPolicy: modPolicy,
				},
			},
		},
	}
}

func (h *testHelper) sampleChannelConfig(sequence uint64, enableV11Capability bool) *common.Config {
	profile := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	if enableV11Capability {
		profile.Orderer.Capabilities = make(map[string]bool)
		profile.Orderer.Capabilities[capabilities.ApplicationV1_1] = true
		profile.Application.Capabilities = make(map[string]bool)
		profile.Application.Capabilities[capabilities.ApplicationV1_1] = true
		profile.Application.Capabilities[capabilities.ApplicationResourcesTreeExperimental] = true
	}
	channelGroup, _ := encoder.NewChannelGroup(profile)
	return &common.Config{
		Sequence:     sequence,
		ChannelGroup: channelGroup,
		Type:         int32(common.ConfigType_CHANNEL),
	}
}

func (h *testHelper) constructConfigTx(t *testing.T, txType common.HeaderType, chainid string, config *common.Config) *common.Envelope {
	env, err := utils.CreateSignedEnvelope(txType, chainid, nil, &common.ConfigEnvelope{Config: config}, 0, 0)
	assert.NoError(t, err)
	return env
}

func (h *testHelper) constructGenesisTx(t *testing.T, chainid string, chanConf, resConf *common.Config) *common.Envelope {
	configEnvelop := &common.ConfigEnvelope{
		Config:     chanConf,
		LastUpdate: h.constructLastUpdateField(chainid, resConf),
	}
	txEnvelope, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG, chainid, nil, configEnvelop, 0, 0)
	assert.NoError(t, err)
	return txEnvelope
}

func (h *testHelper) constructBlock(txEnvelope *common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	return testutil.NewBlock([]*common.Envelope{txEnvelope}, blockNum, previousHash)
}

func (h *testHelper) constructLastUpdateField(chainid string, resConf *common.Config) *common.Envelope {
	configUpdate := utils.MarshalOrPanic(&common.ConfigUpdate{
		ChannelId:    chainid,
		IsolatedData: map[string][]byte{protospeer.ResourceConfigSeedDataKey: utils.MarshalOrPanic(resConf)},
	})
	envelopeForLastUpdateField, _ := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, chainid, nil, &common.ConfigUpdateEnvelope{ConfigUpdate: configUpdate}, 0, 0)
	return envelopeForLastUpdateField
}

func (h *testHelper) mockCreateChain(t *testing.T, chainid string, ledger ledger.PeerLedger) {
	resBundle, err := h.constructResourceBundle(chainid, ledger)
	assert.NoError(t, err)
	chains.list[chainid] = &chain{
		cs: &chainSupport{
			bundleSource: resourcesconfig.NewBundleSource(resBundle),
			ledger:       ledger},
	}
}

func (h *testHelper) clearMockChains() {
	chains.list = make(map[string]*chain)
}

func (h *testHelper) constructResourceBundle(chainid string, ledger ledger.PeerLedger) (*resourcesconfig.Bundle, error) {
	chanConf, err := retrievePersistedChannelConfig(ledger)
	if err != nil {
		return nil, err
	}

	chanConfigBundle, err := channelconfig.NewBundle(chainid, chanConf)
	appConfig, capabilityOn := chanConfigBundle.ApplicationConfig()

	resConf := &common.Config{ChannelGroup: &common.ConfigGroup{}}
	if capabilityOn && appConfig.Capabilities().ResourcesTree() {
		resConf, err = retrievePersistedResourceConfig(ledger)
		if err != nil {
			return nil, err
		}
	}
	return resourcesconfig.NewBundle(chainid, resConf, chanConfigBundle)
}

func (h *testHelper) computeDeltaResConfTx(t *testing.T, chainid string, source, destination *common.Config) *common.Envelope {
	configUpdate, err := update.Compute(source, destination)
	configUpdate.ChannelId = chainid
	assert.NoError(t, err)
	t.Logf("configUpdate=%s", configUpdate)

	confUpdateEnv := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configUpdate),
	}

	h.initLocalMSP()
	sigHeader, err := localmsp.NewSigner().NewSignatureHeader()
	assert.NoError(t, err)

	confUpdateEnv.Signatures = []*common.ConfigSignature{{
		SignatureHeader: utils.MarshalOrPanic(sigHeader)}}

	sigs, err := localmsp.NewSigner().Sign(util.ConcatenateBytes(confUpdateEnv.Signatures[0].SignatureHeader, confUpdateEnv.ConfigUpdate))
	assert.NoError(t, err)
	confUpdateEnv.Signatures[0].Signature = sigs

	env, err := utils.CreateSignedEnvelope(common.HeaderType_PEER_RESOURCE_UPDATE, chainid, nil, confUpdateEnv, 0, 0)
	assert.NoError(t, err)
	return env
}

func (h *testHelper) initLocalMSP() {
	rand.Seed(time.Now().UnixNano())
	conf, err := config.Load()
	if err != nil {
		panic(fmt.Errorf("failed to load config: %s", err))
	}

	// Load local MSP
	err = mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		panic(fmt.Errorf("Failed to initialize local MSP: %s", err))
	}
	msp := mspmgmt.GetLocalMSP()
	_, err = msp.GetDefaultSigningIdentity()
	if err != nil {
		panic(fmt.Errorf("Failed to get default signer: %s", err))
	}
}
