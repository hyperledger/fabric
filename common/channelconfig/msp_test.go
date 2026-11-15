/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMSPConfigManager(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	conf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	require.NoError(t, err)

	// test success:

	mspVers := []msp.MSPVersion{msp.MSPv1_0, msp.MSPv1_1}

	for _, ver := range mspVers {
		mspCH := NewMSPConfigHandler(ver, factory.GetDefault())

		_, err = mspCH.ProposeMSP(conf)
		require.NoError(t, err)

		mgr, err := mspCH.CreateMSPManager()
		require.NoError(t, err)
		require.NotNil(t, mgr)

		msps, err := mgr.GetMSPs()
		require.NoError(t, err)

		if len(msps) == 0 {
			t.Fatalf("There are no MSPS in the manager")
		}

		for _, mspInst := range msps {
			require.Equal(t, mspInst.GetVersion(), ver)
		}
	}
}

func TestMSPConfigFailure(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mspCH := NewMSPConfigHandler(msp.MSPv1_0, cryptoProvider)

	// begin/propose/commit
	t.Run("Bad proto", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Config: []byte("BARF!")})
		require.Error(t, err)
	})

	t.Run("Bad MSP Type", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Type: int32(10)})
		require.Error(t, err)
	})
}

func TestPQRequiredChannelRejectsIdemixMSP(t *testing.T) {
	mspCH := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())
	mspCH.hybridRequired = true

	_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Type: int32(msp.IDEMIX)})
	require.ErrorContains(t, err, "PQ-required channel does not permit an Idemix MSP")
}

func TestPQRequiredChannelRejectsClassicalOnlyMSP(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	mspConfig, err := msp.GetVerifyingMspConfig(
		configtest.GetDevMspDir(),
		"SampleOrg",
		msp.ProviderTypeToString(msp.FABRIC),
	)
	require.NoError(t, err)
	fabricConfig := &mspprotos.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(mspConfig.GetConfig(), fabricConfig))
	fabricConfig.FabricNodeOus = &mspprotos.FabricNodeOUs{
		Enable:              true,
		ClientOuIdentifier:  &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "client"},
		PeerOuIdentifier:    &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "peer"},
		AdminOuIdentifier:   &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "admin"},
		OrdererOuIdentifier: &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "orderer"},
	}
	mspConfig.Config, err = proto.Marshal(fabricConfig)
	require.NoError(t, err)

	mspCH := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())
	mspCH.hybridRequired = true
	_, err = mspCH.ProposeMSP(mspConfig)
	require.ErrorContains(t, err, "Pactway Hybrid Certificate v1")
}

func TestPQRequiredMSPRequiresPeerAndOrdererNodeOUs(t *testing.T) {
	nodeOUs := func() *mspprotos.FabricNodeOUs {
		return &mspprotos.FabricNodeOUs{
			Enable:              true,
			ClientOuIdentifier:  &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "client"},
			PeerOuIdentifier:    &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "peer"},
			AdminOuIdentifier:   &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "admin"},
			OrdererOuIdentifier: &mspprotos.FabricOUIdentifier{OrganizationalUnitIdentifier: "orderer"},
		}
	}

	for _, test := range []struct {
		name        string
		config      func() *mspprotos.FabricMSPConfig
		expectedErr string
	}{
		{
			name:        "missing NodeOUs",
			config:      func() *mspprotos.FabricMSPConfig { return &mspprotos.FabricMSPConfig{} },
			expectedErr: "must enable NodeOUs",
		},
		{
			name: "NodeOUs disabled",
			config: func() *mspprotos.FabricMSPConfig {
				nodeOUConfig := nodeOUs()
				nodeOUConfig.Enable = false
				return &mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUConfig}
			},
			expectedErr: "must enable NodeOUs",
		},
		{
			name: "missing peer role",
			config: func() *mspprotos.FabricMSPConfig {
				nodeOUConfig := nodeOUs()
				nodeOUConfig.PeerOuIdentifier = nil
				return &mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUConfig}
			},
			expectedErr: "must define a non-empty peer NodeOU",
		},
		{
			name: "missing orderer role",
			config: func() *mspprotos.FabricMSPConfig {
				nodeOUConfig := nodeOUs()
				nodeOUConfig.OrdererOuIdentifier = nil
				return &mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUConfig}
			},
			expectedErr: "must define a non-empty orderer NodeOU",
		},
		{
			name: "empty orderer role",
			config: func() *mspprotos.FabricMSPConfig {
				nodeOUConfig := nodeOUs()
				nodeOUConfig.OrdererOuIdentifier.OrganizationalUnitIdentifier = ""
				return &mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUConfig}
			},
			expectedErr: "orderer NodeOU identifier must not be empty",
		},
		{
			name: "overlapping peer and orderer roles",
			config: func() *mspprotos.FabricMSPConfig {
				nodeOUConfig := nodeOUs()
				nodeOUConfig.OrdererOuIdentifier.OrganizationalUnitIdentifier = "peer"
				return &mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUConfig}
			},
			expectedErr: "peer and orderer must be distinct",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateHybridNodeOUs(test.config())
			require.ErrorContains(t, err, test.expectedErr)
		})
	}

	require.NoError(t, validateHybridNodeOUs(&mspprotos.FabricMSPConfig{FabricNodeOus: nodeOUs()}))
}

func TestPQRequiredMSPConfigRejectsMissingNodeOUs(t *testing.T) {
	serialized, err := proto.Marshal(&mspprotos.FabricMSPConfig{Name: "PQ2TestMSP"})
	require.NoError(t, err)
	handler := NewMSPConfigHandler(msp.MSPv1_4_3, factory.GetDefault())
	handler.hybridRequired = true

	_, err = handler.ProposeMSP(&mspprotos.MSPConfig{
		Type:   int32(msp.FABRIC),
		Config: serialized,
	})
	require.ErrorContains(t, err, "PQ-required Fabric MSP must enable NodeOUs")
}
