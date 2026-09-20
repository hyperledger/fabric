/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestConfigTxCreateLedger(t *testing.T) {
	helper := newTestHelper(t)
	channelID := "testchain1"
	tempdir := t.TempDir()

	ledgerMgr, err := constructLedgerMgrWithTestDefaults(tempdir)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer func() {
		ledgerMgr.Close()
	}()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, channelID, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	ledger, err := ledgerMgr.CreateLedger(channelID, genesisBlock)
	require.NoError(t, err)

	retrievedchanConf, err := RetrievePersistedChannelConfig(ledger)
	require.NoError(t, err)
	require.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))
}

func TestConfigTxErrorScenarios(t *testing.T) {
	configTxProcessor := &ConfigTxProcessor{}
	// wrong tx type
	configEnvWrongTxType := &common.ConfigEnvelope{}
	txEnvelope, err := protoutil.CreateSignedEnvelope(common.HeaderType_DELIVER_SEEK_INFO, "channelID", nil, configEnvWrongTxType, 0, 0)
	require.NoError(t, err)
	err = configTxProcessor.GenerateSimulationResults(txEnvelope, nil, false)
	require.EqualError(t, err, "tx type [DELIVER_SEEK_INFO] is not expected")

	// empty channelConfig
	txEnvelope, err = protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG, "channelID", nil, &common.ConfigEnvelope{}, 0, 0)
	require.NoError(t, err)
	err = configTxProcessor.GenerateSimulationResults(txEnvelope, nil, false)
	require.EqualError(t, err, "channel config found nil")
}

func TestConfigTxUpdateChanConfig(t *testing.T) {
	helper := newTestHelper(t)
	channelID := "testchain1"
	tempdir := t.TempDir()

	ledgerMgr, err := constructLedgerMgrWithTestDefaults(tempdir)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer func() {
		ledgerMgr.Close()
	}()

	chanConf := helper.sampleChannelConfig(1, true)
	genesisTx := helper.constructGenesisTx(t, channelID, chanConf)
	genesisBlock := helper.constructBlock(genesisTx, 0, nil)
	lgr, err := ledgerMgr.CreateLedger(channelID, genesisBlock)
	require.NoError(t, err)

	retrievedchanConf, err := RetrievePersistedChannelConfig(lgr)
	require.NoError(t, err)
	require.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(retrievedchanConf))

	helper.mockCreateChain(t, channelID, lgr)
	defer helper.clearMockChains()

	bs := helper.peer.channels[channelID].bundleSource
	inMemoryChanConf := bs.ConfigtxValidator().ConfigProto()
	require.Equal(t, proto.CompactTextString(chanConf), proto.CompactTextString(inMemoryChanConf))

	retrievedchanConf, err = RetrievePersistedChannelConfig(lgr)
	require.NoError(t, err)
	require.Equal(t, proto.CompactTextString(bs.ConfigtxValidator().ConfigProto()), proto.CompactTextString(retrievedchanConf))

	lgr.Close()
	helper.clearMockChains()
	_, err = ledgerMgr.OpenLedger(channelID)
	require.NoError(t, err)
}

func TestGenesisBlockCreateLedger(t *testing.T) {
	b, err := configtxtest.MakeGenesisBlock("testchain")
	require.NoError(t, err)
	tempdir := t.TempDir()

	ledgerMgr, err := constructLedgerMgrWithTestDefaults(tempdir)
	if err != nil {
		t.Fatalf("Failed to create test environment: %s", err)
	}

	defer func() {
		ledgerMgr.Close()
	}()

	lgr, err := ledgerMgr.CreateLedger("testchain", b)
	require.NoError(t, err)
	chanConf, err := RetrievePersistedChannelConfig(lgr)
	require.NoError(t, err)
	require.NotNil(t, chanConf)
	t.Logf("chanConf = %s", chanConf)
}

type testHelper struct {
	t    *testing.T
	peer *Peer
}

func newTestHelper(t *testing.T) *testHelper {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	return &testHelper{
		t:    t,
		peer: &Peer{CryptoProvider: cryptoProvider},
	}
}

func (h *testHelper) sampleChannelConfig(sequence uint64, enableV11Capability bool) *common.Config {
	profile := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
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
	require.NoError(t, err)
	return txEnvelope
}

func (h *testHelper) constructBlock(txEnvelope *common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	return testutil.NewBlock([]*common.Envelope{txEnvelope}, blockNum, previousHash)
}

func (h *testHelper) constructLastUpdateField(channelID string) *common.Envelope {
	configUpdate := protoutil.MarshalOrPanic(&common.ConfigUpdate{
		ChannelId: channelID,
	})
	envelopeForLastUpdateField, _ := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channelID,
		nil,
		&common.ConfigUpdateEnvelope{ConfigUpdate: configUpdate},
		0,
		0,
	)
	return envelopeForLastUpdateField
}

func (h *testHelper) mockCreateChain(t *testing.T, channelID string, ledger ledger.PeerLedger) {
	chanBundle, err := h.constructChannelBundle(channelID, ledger)
	require.NoError(t, err)
	if h.peer.channels == nil {
		h.peer.channels = map[string]*Channel{}
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	h.peer.channels[channelID] = &Channel{
		bundleSource:   channelconfig.NewBundleSource(chanBundle),
		ledger:         ledger,
		cryptoProvider: cryptoProvider,
	}
}

func (h *testHelper) clearMockChains() {
	h.peer.channels = make(map[string]*Channel)
}

func (h *testHelper) constructChannelBundle(channelID string, ledger ledger.PeerLedger) (*channelconfig.Bundle, error) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}

	chanConf, err := RetrievePersistedChannelConfig(ledger)
	if err != nil {
		return nil, err
	}

	return channelconfig.NewBundle(channelID, chanConf, cryptoProvider)
}
