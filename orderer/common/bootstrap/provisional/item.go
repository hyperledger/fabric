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

func (cbs *commonBootstrapper) templateConsensusType() *cb.ConfigurationItem {
	configItemKey := sharedconfig.ConsensusTypeKey
	configItemValue := utils.MarshalOrPanic(&ab.ConsensusType{Type: cbs.consensusType})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateBatchSize() *cb.ConfigurationItem {
	configItemKey := sharedconfig.BatchSizeKey
	configItemValue := utils.MarshalOrPanic(cbs.batchSize)
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateBatchTimeout() *cb.ConfigurationItem {
	configItemKey := sharedconfig.BatchTimeoutKey
	configItemValue := utils.MarshalOrPanic(&ab.BatchTimeout{Timeout: cbs.batchTimeout})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateChainCreationPolicyNames() *cb.ConfigurationItem {
	configItemKey := sharedconfig.ChainCreationPolicyNamesKey
	configItemValue := utils.MarshalOrPanic(&ab.ChainCreationPolicyNames{Names: DefaultChainCreationPolicyNames})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateAcceptAllPolicy() *cb.ConfigurationItem {
	configItemKey := AcceptAllPolicyKey
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.AcceptAllPolicy))
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateIngressPolicyNames() *cb.ConfigurationItem {
	configItemKey := sharedconfig.IngressPolicyNamesKey
	configItemValue := utils.MarshalOrPanic(&ab.IngressPolicyNames{Names: []string{AcceptAllPolicyKey}})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateEgressPolicyNames() *cb.ConfigurationItem {
	configItemKey := sharedconfig.EgressPolicyNamesKey
	configItemValue := utils.MarshalOrPanic(&ab.EgressPolicyNames{Names: []string{AcceptAllPolicyKey}})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}

func (cbs *commonBootstrapper) templateRejectAllPolicy() *cb.ConfigurationItem {
	// Lock down the new configuration item policy to prevent any new configuration items from being created
	configItemKey := configtx.NewConfigurationItemPolicyKey
	configItemValue := utils.MarshalOrPanic(utils.MakePolicyOrPanic(cauthdsl.RejectAllPolicy))
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, lastModified, modPolicy, configItemKey, configItemValue)
}

func (kbs *kafkaBootstrapper) templateKafkaBrokers() *cb.ConfigurationItem {
	configItemKey := sharedconfig.KafkaBrokersKey
	configItemValue := utils.MarshalOrPanic(&ab.KafkaBrokers{Brokers: kbs.kafkaBrokers})
	modPolicy := configtx.NewConfigurationItemPolicyKey

	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, kbs.chainID, epoch)
	return utils.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Orderer, lastModified, modPolicy, configItemKey, configItemValue)
}
