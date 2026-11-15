/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

//go:generate counterfeiter -o mocks/policy.go --fake-name Policy . policy

type policy interface{ policies.Policy }

//go:generate counterfeiter -o mocks/policy_manager.go --fake-name PolicyManager . policyManager

type policyManager interface{ policies.Manager }

//go:generate counterfeiter -o mocks/read_writer.go --fake-name ReadWriter . readWriter

type readWriter interface{ blockledger.ReadWriter }

func TestChainSupportBlock(t *testing.T) {
	ledger := &mocks.ReadWriter{}
	ledger.HeightReturns(100)
	iterator := &mock.BlockIterator{}
	iterator.NextReturns(&common.Block{Header: &common.BlockHeader{Number: 99}}, common.Status_SUCCESS)
	ledger.IteratorReturns(iterator, 99)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cs := &ChainSupport{
		ledgerResources: &ledgerResources{ReadWriter: ledger},
		BCCSP:           cryptoProvider,
	}

	require.Nil(t, cs.Block(100))
	require.Equal(t, uint64(99), cs.Block(99).GetHeader().GetNumber())
}

type mutableResourcesMock struct {
	*mocks.Resources
	newConsensusMetadataVal []byte
}

func (*mutableResourcesMock) Update(*channelconfig.Bundle) {
	panic("implement me")
}

func (mrm *mutableResourcesMock) CreateBundle(channelID string, c *common.Config) (channelconfig.Resources, error) {
	mockOrderer := &mocks.OrdererConfig{}
	mockOrderer.ConsensusMetadataReturns(mrm.newConsensusMetadataVal)
	mockResources := &mocks.Resources{}
	mockResources.OrdererConfigReturns(mockOrderer, true)

	return mockResources, nil
}

func testConfigEnvelope(t *testing.T) *common.ConfigEnvelope {
	conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
	conf.Orderer.Organizations[0].OrdererEndpoints = []string{"127.0.0.1:7050"}
	group, err := encoder.NewChannelGroup(conf)
	require.NoError(t, err)
	group.Groups["Orderer"].Values["ConsensusType"].Value, err = proto.Marshal(&orderer.ConsensusType{
		Metadata: []byte("new consensus metadata"),
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	return &common.ConfigEnvelope{
		Config: &common.Config{
			ChannelGroup: group,
		},
	}
}

func TestHybridCapabilityActivationStopsIncompatibleOrderer(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	config := testConfigEnvelope(t).GetConfig()
	group := config.GetChannelGroup()
	clear(group.GetGroups()[channelconfig.OrdererGroupKey].GetGroups())
	clear(group.GetGroups()[channelconfig.ApplicationGroupKey].GetGroups())
	delete(group.GetGroups(), channelconfig.ConsortiumsGroupKey)
	caps := &common.Capabilities{Capabilities: map[string]*common.Capability{
		capabilities.ChannelV1_4_2:              {},
		capabilities.PactwayHybridCertificateV1: {},
	}}
	capBytes, err := proto.Marshal(caps)
	require.NoError(t, err)
	group.GetValues()[channelconfig.CapabilitiesKey] = &common.ConfigValue{Value: capBytes}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	bundle, err := channelconfig.NewBundle("testchannel", config, cryptoProvider)
	require.NoError(t, err)

	signer := &mocks.SignerSerializer{}
	signer.SerializeReturns([]byte("not a channel identity"), nil)
	resources := &configResources{
		mutableResources: &mutableResourcesMock{},
		signer:           signer,
	}
	func() {
		defer func() {
			recovered := recover()
			require.NotNil(t, recovered)
			require.Contains(t, fmt.Sprint(recovered), "incompatible local signing identity")
		}()
		resources.Update(bundle)
	}()
	require.Equal(t, 1, signer.SerializeCallCount())
}
