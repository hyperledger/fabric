/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger/mocks"
	"github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestChainSupportBlock(t *testing.T) {
	ledger := &mocks.ReadWriter{}
	ledger.On("Height").Return(uint64(100))
	iterator := &mock.BlockIterator{}
	iterator.NextReturns(&common.Block{Header: &common.BlockHeader{Number: 99}}, common.Status_SUCCESS)
	ledger.On("Iterator", &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Specified{
			Specified: &orderer.SeekSpecified{Number: 99},
		},
	}).Return(iterator, uint64(99))
	cs := &ChainSupport{ledgerResources: &ledgerResources{ReadWriter: ledger}}

	assert.Nil(t, cs.Block(100))
	assert.Equal(t, uint64(99), cs.Block(99).Header.Number)
}

type mutableResourcesMock struct {
	config.Resources
}

func (*mutableResourcesMock) Update(*channelconfig.Bundle) {
	panic("implement me")
}

func TestVerifyBlockSignature(t *testing.T) {
	policyMgr := &mockpolicies.Manager{
		PolicyMap: make(map[string]policies.Policy),
	}
	ms := &mutableResourcesMock{
		Resources: config.Resources{
			ConfigtxValidatorVal: &mockconfigtx.Validator{ChainIDVal: "mychannel"},
			PolicyManagerVal:     policyMgr,
		},
	}
	cs := &ChainSupport{
		ledgerResources: &ledgerResources{
			configResources: &configResources{
				mutableResources: ms,
			},
		},
	}

	// Scenario I: Policy manager isn't initialized
	// and thus policy cannot be found
	err := cs.VerifyBlockSignature([]*common.SignedData{}, nil)
	assert.EqualError(t, err, "policy /Channel/Orderer/BlockValidation wasn't found")

	// Scenario II: Policy manager finds policy, but it evaluates
	// to error.
	policyMgr.PolicyMap["/Channel/Orderer/BlockValidation"] = &mockpolicies.Policy{
		Err: errors.New("invalid signature"),
	}
	err = cs.VerifyBlockSignature([]*common.SignedData{}, nil)
	assert.EqualError(t, err, "block verification failed: invalid signature")

	// Scenario III: Policy manager finds policy, and it evaluates to success
	policyMgr.PolicyMap["/Channel/Orderer/BlockValidation"] = &mockpolicies.Policy{
		Err: nil,
	}
	assert.NoError(t, cs.VerifyBlockSignature([]*common.SignedData{}, nil))

	// Scenario IV: A bad config envelope is passed
	err = cs.VerifyBlockSignature([]*common.SignedData{}, &common.ConfigEnvelope{})
	assert.EqualError(t, err, "channelconfig Config cannot be nil")

	// Scenario V: A valid config envelope is passed
	assert.NoError(t, cs.VerifyBlockSignature([]*common.SignedData{}, testConfigEnvelope(t)))

}

func testConfigEnvelope(t *testing.T) *common.ConfigEnvelope {
	config := configtxgentest.Load(localconfig.SampleInsecureSoloProfile)
	group, err := encoder.NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)
	return &common.ConfigEnvelope{
		Config: &common.Config{
			ChannelGroup: group,
		},
	}
}

func TestDetectMigration(t *testing.T) {
	confSys := configtxgentest.Load(localconfig.SampleInsecureSoloProfile)
	confSys.Orderer.Capabilities[capabilities.OrdererV1_4_2] = true
	confSys.Capabilities[capabilities.ChannelV1_4_2] = true
	confSys.Orderer.OrdererType = "kafka"
	genesisBlock := encoder.New(confSys).GenesisBlock()
	metadataFull := utils.MarshalOrPanic(&common.Metadata{Value: []byte{1, 2, 3, 4}})
	metadataEmpty := utils.MarshalOrPanic(&common.Metadata{Value: []byte{}})

	t.Run("Correct flow", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataFull, capabilities.OrdererV1_4_2)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "etcdraft", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataEmpty, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.True(t, chainSupport.DetectConsensusMigration(), "correct flow")
	})

	t.Run("Correct flow, disabled", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataFull, capabilities.OrdererV1_1)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "etcdraft", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataEmpty, capabilities.OrdererV1_1)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "disabled")
	})

	t.Run("On genesis block, height=1, no migration", func(t *testing.T) {
		lf, _ := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("On config tx, NORMAL state, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_NORMAL,
			metadataFull, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("One tx in MAINTENANCE state, not empty metadata, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataFull, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("One tx in MAINTENANCE state, no previous MAINTENANCE, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataEmpty, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("Two txs in MAINTENANCE state, no type change, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataFull, capabilities.OrdererV1_4_2)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataEmpty, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("Normal block, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextNormalBlock(t, rl, localconfig.TestChainID, 0, metadataFull)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})

	t.Run("Normal block, one tx in MAINTENANCE state, no previous MAINTENANCE, no migration", func(t *testing.T) {
		lf, rl := newRAMLedgerAndFactory(10, localconfig.TestChainID, genesisBlock)
		appendNextNormalBlock(t, rl, localconfig.TestChainID, 0, metadataFull)
		appendNextMigBlock(t, rl, localconfig.TestChainID, "kafka", orderer.ConsensusType_STATE_MAINTENANCE,
			metadataEmpty, capabilities.OrdererV1_4_2)
		_, _, chainSupport := createChainsupport(t, lf)
		assert.False(t, chainSupport.DetectConsensusMigration(), "no migration")
	})
}

func appendNextMigBlock(t *testing.T, rl blockledger.ReadWriter, chainID string,
	ordererType string, state orderer.ConsensusType_State, ordererMetadata []byte, ordCapability string) {
	nextBlock := blockledger.CreateNextBlock(rl, []*common.Envelope{
		makeMigConfigTx(t, chainID, ordererType, state, ordCapability),
	})
	nextBlock.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: rl.Height()}),
	})
	nextBlock.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = ordererMetadata
	err := rl.Append(nextBlock)
	require.NoError(t, err)
	return
}

func appendNextNormalBlock(t *testing.T, rl blockledger.ReadWriter, chainID string, lastConfig uint64, ordererMetadata []byte) {
	nextBlock := blockledger.CreateNextBlock(rl,
		[]*common.Envelope{
			makeMigNormalTx(t, chainID, fmt.Sprintf("tx-%d-1", rl.Height())),
			makeMigNormalTx(t, chainID, fmt.Sprintf("tx-%d-2", rl.Height())),
		})
	nextBlock.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: lastConfig}),
	})
	nextBlock.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = ordererMetadata
	err := rl.Append(nextBlock)
	require.NoError(t, err)
	return
}

func createChainsupport(t *testing.T, lf blockledger.Factory) (map[string]consensus.Consenter, *Registrar, *ChainSupport) {
	consenters := make(map[string]consensus.Consenter)
	consenters["solo"] = &mockConsenter{}
	consenters["kafka"] = &mockConsenter{}
	consenters["etcdraft"] = &mockConsenter{}
	manager := NewRegistrar(lf, mockCrypto(), &disabled.Provider{})
	manager.Initialize(consenters)
	require.NotNil(t, manager)
	chainSupport := manager.GetChain(localconfig.TestChainID)
	require.NotNil(t, chainSupport)
	return consenters, manager, chainSupport
}

func makeMigConfigTx(t *testing.T, chainID string, ordererType string, state orderer.ConsensusType_State, ordCapability string) *common.Envelope {
	gConf := configtxgentest.Load(localconfig.SampleInsecureSoloProfile)
	gConf.Orderer.Capabilities = map[string]bool{
		ordCapability: true,
	}
	gConf.Orderer.OrdererType = "kafka"
	channelGroup, err := encoder.NewChannelGroup(gConf)
	require.NoError(t, err)

	channelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: utils.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:  ordererType,
				State: state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	configUpdateEnv := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(&common.ConfigUpdate{
			WriteSet: channelGroup,
		}),
	}

	configUpdateTx, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, chainID, mockCrypto(), configUpdateEnv, 0, 0)
	require.NoError(t, err)

	configTx, err := utils.CreateSignedEnvelope(
		common.HeaderType_CONFIG,
		chainID,
		mockCrypto(),
		&common.ConfigEnvelope{
			Config: &common.Config{
				Sequence:     1,
				ChannelGroup: configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate).WriteSet,
			},
			LastUpdate: configUpdateTx,
		},
		0,
		0)
	require.NoError(t, err)

	return configTx
}

func makeMigNormalTx(t *testing.T, chainID string, txId string) *common.Envelope {
	txBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	cHdr := &common.ChannelHeader{
		TxId:      txId,
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
		ChannelId: chainID,
	}
	cHdrBytes, err := proto.Marshal(cHdr)
	require.NoError(t, err)
	commonPayload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: cHdrBytes,
		},
		Data: txBytes,
	}

	payloadBytes, err := proto.Marshal(commonPayload)
	require.NoError(t, err)
	envelope := &common.Envelope{
		Payload: payloadBytes,
	}

	return envelope
}
