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

package static

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

// TestChainID is the default chain ID which is used by all statically bootstrapped networks
// This is necessary to allow test clients to connect without being rejected for targetting
// a chain which does not exist
const TestChainID = "**TEST_CHAINID**"

const msgVersion = int32(1)

type bootstrapper struct {
	chainID       string
	lastModified  uint64
	epoch         uint64
	consensusType string
	batchSize     int32
}

const (
	// DefaultBatchSize is the default value of BatchSizeKey
	DefaultBatchSize = 10

	// DefaultConsensusType is the default value of ConsensusTypeKey
	DefaultConsensusType = "solo"

	// AcceptAllPolicyKey is the key of the AcceptAllPolicy
	AcceptAllPolicyKey = "AcceptAllPolicy"
)

var (
	// DefaultChainCreators is the default value of ChainCreatorsKey
	DefaultChainCreators = []string{AcceptAllPolicyKey}
)

// New returns a new static bootstrap helper.
func New() bootstrap.Helper {
	return &bootstrapper{
		chainID:       TestChainID,
		consensusType: DefaultConsensusType,
		batchSize:     DefaultBatchSize,
	}
}

func (b *bootstrapper) encodeConsensusType() *cb.SignedConfigurationItem {
	configItemKey := sharedconfig.ConsensusTypeKey
	configItemValue := utils.MarshalOrPanic(&ab.ConsensusType{Type: b.consensusType})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: utils.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) encodeBatchSize() *cb.SignedConfigurationItem {
	configItemKey := sharedconfig.BatchSizeKey
	configItemValue := utils.MarshalOrPanic(&ab.BatchSize{Messages: b.batchSize})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: utils.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) encodeChainCreators() *cb.SignedConfigurationItem {
	configItemKey := sharedconfig.ChainCreatorsKey
	configItemValue := utils.MarshalOrPanic(&ab.ChainCreators{Policies: DefaultChainCreators})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: utils.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) encodeAcceptAllPolicy() *cb.SignedConfigurationItem {
	configItemKey := AcceptAllPolicyKey
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.AcceptAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: utils.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) lockDefaultModificationPolicy() *cb.SignedConfigurationItem {
	// Lock down the default modification policy to prevent any further policy modifications
	configItemKey := configtx.DefaultModificationPolicyID
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.RejectAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: utils.MarshalOrPanic(configItem), Signatures: nil}
}

// GenesisBlock returns the genesis block to be used for bootstrapping
func (b *bootstrapper) GenesisBlock() (*cb.Block, error) {
	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)

	configEnvelope := utils.MakeConfigurationEnvelope(
		b.encodeConsensusType(),
		b.encodeBatchSize(),
		b.encodeChainCreators(),
		b.encodeAcceptAllPolicy(),
		b.lockDefaultModificationPolicy(),
	)
	payloadChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, configItemChainHeader.Version, b.chainID, b.epoch)
	payloadSignatureHeader := utils.MakeSignatureHeader(nil, utils.CreateNonceOrPanic())
	payloadHeader := utils.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(configEnvelope)}
	envelope := &cb.Envelope{Payload: utils.MarshalOrPanic(payload), Signature: nil}

	blockData := &cb.BlockData{Data: [][]byte{utils.MarshalOrPanic(envelope)}}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: nil,
			DataHash:     blockData.Hash(),
		},
		Data:     blockData,
		Metadata: nil,
	}, nil
}
