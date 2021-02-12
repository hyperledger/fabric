/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"testing"

	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/types"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	msgprocessormocks "github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestConsensusMetadataValidation(t *testing.T) {
	oldConsensusMetadata := []byte("old consensus metadata")
	newConsensusMetadata := []byte("new consensus metadata")
	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ChannelIDReturns("mychannel")
	mockValidator.ProposeConfigUpdateReturns(testConfigEnvelope(t), nil)
	mockOrderer := &mocks.OrdererConfig{}
	mockOrderer.ConsensusMetadataReturns(oldConsensusMetadata)
	mockResources := &mocks.Resources{}
	mockResources.ConfigtxValidatorReturns(mockValidator)
	mockResources.OrdererConfigReturns(mockOrderer, true)

	ms := &mutableResourcesMock{
		Resources:               mockResources,
		newConsensusMetadataVal: newConsensusMetadata,
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mv := &msgprocessormocks.MetadataValidator{}
	cs := &ChainSupport{
		ledgerResources: &ledgerResources{
			configResources: &configResources{
				mutableResources: ms,
				bccsp:            cryptoProvider,
			},
		},
		MetadataValidator: mv,
		BCCSP:             cryptoProvider,
	}

	// case 1: valid consensus metadata update
	_, err = cs.ProposeConfigUpdate(&common.Envelope{})
	require.NoError(t, err)

	// validate arguments to ValidateConsensusMetadata
	require.Equal(t, 1, mv.ValidateConsensusMetadataCallCount())
	om, nm, nc := mv.ValidateConsensusMetadataArgsForCall(0)
	require.False(t, nc)
	require.Equal(t, oldConsensusMetadata, om.ConsensusMetadata())
	require.Equal(t, newConsensusMetadata, nm.ConsensusMetadata())

	// case 2: invalid consensus metadata update
	mv.ValidateConsensusMetadataReturns(errors.New("bananas"))
	_, err = cs.ProposeConfigUpdate(&common.Envelope{})
	require.EqualError(t, err, "consensus metadata update for channel config update is invalid: bananas")
}

func TestNewOnboardingChainSupport(t *testing.T) {
	mockResources := &mocks.Resources{}
	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ChannelIDReturns("mychannel")
	mockResources.ConfigtxValidatorReturns(mockValidator)

	ms := &mutableResourcesMock{
		Resources: mockResources,
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	mockRW := &mocks.ReadWriter{}
	mockRW.HeightReturns(7)
	ledgerRes := &ledgerResources{
		configResources: &configResources{
			mutableResources: ms,
			bccsp:            cryptoProvider,
		},
		ReadWriter: mockRW,
	}

	cs, err := newOnBoardingChainSupport(ledgerRes, localconfig.TopLevel{}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, cs)

	errStr := "system channel creation pending: server requires restart"
	require.EqualError(t, cs.Order(nil, 0), errStr)
	require.EqualError(t, cs.Configure(nil, 0), errStr)
	require.EqualError(t, cs.WaitReady(), errStr)
	require.NotPanics(t, cs.Start)
	require.NotPanics(t, cs.Halt)
	_, open := <-cs.Errored()
	require.False(t, open)

	cRel, status := cs.StatusReport()
	require.Equal(t, types.ConsensusRelationConsenter, cRel)
	require.Equal(t, types.StatusInactive, status)

	require.Equal(t, uint64(7), cs.Height(), "ledger ReadWriter is initialized")
	require.Equal(t, "mychannel", cs.ConfigtxValidator().ChannelID(), "ChannelConfig is initialized")
	require.Equal(t, msgprocessor.ConfigUpdateMsg,
		cs.ClassifyMsg(&common.ChannelHeader{
			Type:      int32(common.HeaderType_CONFIG_UPDATE),
			ChannelId: "mychannel",
		}), "Message processor is initialized")
}
