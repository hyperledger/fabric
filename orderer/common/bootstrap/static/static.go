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
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var TestChainID = "**TEST_CHAINID**"

const msgVersion = int32(1)

type bootstrapper struct {
	chainID       string
	lastModified  uint64
	epoch         uint64
	consensusType string
	batchSize     int32
}

// New returns a new static bootstrap helper.
func New() bootstrap.Helper {
	return &bootstrapper{
		chainID:       TestChainID,
		consensusType: "solo",
		batchSize:     10,
	}
}

func (b *bootstrapper) encodeConsensusType() *cb.SignedConfigurationItem {
	configItemKey := sharedconfig.ConsensusTypeKey
	configItemValue := util.MarshalOrPanic(&ab.ConsensusType{Type: b.consensusType})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := util.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: util.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) encodeBatchSize() *cb.SignedConfigurationItem {
	configItemKey := sharedconfig.BatchSizeKey
	configItemValue := util.MarshalOrPanic(&ab.BatchSize{Messages: b.batchSize})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := util.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: util.MarshalOrPanic(configItem), Signatures: nil}
}

func (b *bootstrapper) lockDefaultModificationPolicy() *cb.SignedConfigurationItem {
	// Lock down the default modification policy to prevent any further policy modifications
	configItemKey := configtx.DefaultModificationPolicyID
	configItemValue := util.MarshalOrPanic(util.MakePolicyOrPanic(cauthdsl.RejectAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)
	configItem := util.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, b.lastModified, modPolicy, configItemKey, configItemValue)
	return &cb.SignedConfigurationItem{ConfigurationItem: util.MarshalOrPanic(configItem), Signatures: nil}
}

// GenesisBlock returns the genesis block to be used for bootstrapping
func (b *bootstrapper) GenesisBlock() (*cb.Block, error) {
	configItemChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, b.epoch)

	configEnvelope := util.MakeConfigurationEnvelope(
		b.encodeConsensusType(),
		b.encodeBatchSize(),
		b.lockDefaultModificationPolicy(),
	)
	payloadChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, configItemChainHeader.Version, b.chainID, b.epoch)
	payloadSignatureHeader := util.MakeSignatureHeader(nil, util.CreateNonceOrPanic())
	payloadHeader := util.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: util.MarshalOrPanic(configEnvelope)}
	envelope := &cb.Envelope{Payload: util.MarshalOrPanic(payload), Signature: nil}

	blockData := &cb.BlockData{Data: [][]byte{util.MarshalOrPanic(envelope)}}

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
