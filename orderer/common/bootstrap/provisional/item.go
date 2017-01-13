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

package provisional

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

func (cbs *commonBootstrapper) encodeConsensusType() *cb.ConfigurationItem {
	configItemKey := sharedconfig.ConsensusTypeKey
	configItemValue := utils.MarshalOrPanic(&ab.ConsensusType{Type: cbs.consensusType})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeBatchSize() *cb.ConfigurationItem {
	configItemKey := sharedconfig.BatchSizeKey
	configItemValue := utils.MarshalOrPanic(cbs.batchSize)
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeBatchTimeout() *cb.ConfigurationItem {
	configItemKey := sharedconfig.BatchTimeoutKey
	configItemValue := utils.MarshalOrPanic(&ab.BatchTimeout{Timeout: cbs.batchTimeout})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeChainCreators() *cb.ConfigurationItem {
	configItemKey := sharedconfig.ChainCreatorsKey
	configItemValue := utils.MarshalOrPanic(&ab.ChainCreators{Policies: DefaultChainCreators})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeAcceptAllPolicy() *cb.ConfigurationItem {
	configItemKey := AcceptAllPolicyKey
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.AcceptAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeIngressPolicy() *cb.ConfigurationItem {
	configItemKey := sharedconfig.IngressPolicyKey
	configItemValue := utils.MarshalOrPanic(&ab.IngressPolicy{Name: AcceptAllPolicyKey})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) encodeEgressPolicy() *cb.ConfigurationItem {
	configItemKey := sharedconfig.EgressPolicyKey
	configItemValue := utils.MarshalOrPanic(&ab.EgressPolicy{Name: AcceptAllPolicyKey})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) lockDefaultModificationPolicy() *cb.ConfigurationItem {
	// Lock down the default modification policy to prevent any further policy modifications
	configItemKey := configtx.DefaultModificationPolicyID
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.RejectAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, lastModified, modPolicy, configItemKey, configItemValue)
}

func (kbs *kafkaBootstrapper) encodeKafkaBrokers() *cb.ConfigurationItem {
	configItemKey := sharedconfig.KafkaBrokersKey
	configItemValue := utils.MarshalOrPanic(&ab.KafkaBrokers{Brokers: kbs.kafkaBrokers})
	modPolicy := configtx.DefaultModificationPolicyID

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, kbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}
