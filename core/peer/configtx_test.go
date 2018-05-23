/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	ordererconfig "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPeerFS(t *testing.T) (cleanup func()) {
	tempDir, err := ioutil.TempDir("", "peer-fs")
	require.NoError(t, err)

	viper.Set("peer.fileSystemPath", tempDir)
	return func() { os.RemoveAll(tempDir) }
}

func TestConfigTxCreateLedger(t *testing.T) {
	helper := &testHelper{t: t}
	cleanup := setupPeerFS(t)
	defer cleanup()

	chainid := "testchain1"
	ledgermgmt.InitializeTestEnvWithCustomProcessors(ConfigTxProcessors)
	defer ledgermgmt.CleanupTestEnv()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, chainid, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	ledger, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)

	retrievedchanConf, err := retrievePersistedChannelConfig(ledger)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))
}

func TestConfigTxUpdateChanConfig(t *testing.T) {
	helper := &testHelper{t: t}
	cleanup := setupPeerFS(t)
	defer cleanup()
	chainid := "testchain1"
	ledgermgmt.InitializeTestEnvWithCustomProcessors(ConfigTxProcessors)
	defer ledgermgmt.CleanupTestEnv()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, chainid, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	lgr, err := ledgermgmt.CreateLedger(genesisBlock)
	assert.NoError(t, err)

	retrievedchanConf, err := retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))

	helper.mockCreateChain(t, chainid, lgr)
	defer helper.clearMockChains()

	bs := chains.list[chainid].cs.bundleSource
	inMemoryChanConf := bs.ConfigtxValidator().ConfigProto()
	assert.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(inMemoryChanConf))

	retrievedchanConf, err = retrievePersistedChannelConfig(lgr)
	assert.NoError(t, err)
	assert.Equal(t, proto.CompactTextString(bs.ConfigtxValidator().ConfigProto()), proto.CompactTextString(retrievedchanConf))

	lgr.Close()
	helper.clearMockChains()
	lgr, err = ledgermgmt.OpenLedger(chainid)
	assert.NoError(t, err)
}

func TestGenesisBlockCreateLedger(t *testing.T) {
	cleanup := setupPeerFS(t)
	defer cleanup()

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

func (h *testHelper) constructConfigTx(t *testing.T, txType common.HeaderType, chainid string, config *common.Config) *common.Envelope {
	env, err := utils.CreateSignedEnvelope(txType, chainid, nil, &common.ConfigEnvelope{Config: config}, 0, 0)
	assert.NoError(t, err)
	return env
}

func (h *testHelper) constructGenesisTx(t *testing.T, chainid string, chanConf *common.Config) *common.Envelope {
	configEnvelop := &common.ConfigEnvelope{
		Config:     chanConf,
		LastUpdate: h.constructLastUpdateField(chainid),
	}
	txEnvelope, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG, chainid, nil, configEnvelop, 0, 0)
	assert.NoError(t, err)
	return txEnvelope
}

func (h *testHelper) constructBlock(txEnvelope *common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	return testutil.NewBlock([]*common.Envelope{txEnvelope}, blockNum, previousHash)
}

func (h *testHelper) constructLastUpdateField(chainid string) *common.Envelope {
	configUpdate := utils.MarshalOrPanic(&common.ConfigUpdate{
		ChannelId: chainid,
	})
	envelopeForLastUpdateField, _ := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, chainid, nil, &common.ConfigUpdateEnvelope{ConfigUpdate: configUpdate}, 0, 0)
	return envelopeForLastUpdateField
}

func (h *testHelper) mockCreateChain(t *testing.T, chainid string, ledger ledger.PeerLedger) {
	chanBundle, err := h.constructChannelBundle(chainid, ledger)
	assert.NoError(t, err)
	chains.list[chainid] = &chain{
		cs: &chainSupport{
			bundleSource: channelconfig.NewBundleSource(chanBundle),
			ledger:       ledger},
	}
}

func (h *testHelper) clearMockChains() {
	chains.list = make(map[string]*chain)
}

func (h *testHelper) constructChannelBundle(chainid string, ledger ledger.PeerLedger) (*channelconfig.Bundle, error) {
	chanConf, err := retrievePersistedChannelConfig(ledger)
	if err != nil {
		return nil, err
	}

	return channelconfig.NewBundle(chainid, chanConf)
}

func (h *testHelper) initLocalMSP() {
	rand.Seed(time.Now().UnixNano())
	cleanup := configtest.SetDevFabricConfigPath(h.t)
	defer cleanup()
	conf, err := ordererconfig.Load()
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
