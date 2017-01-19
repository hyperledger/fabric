/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package sharedconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

// TemplateConsensusType creates a headerless configuration item representing the consensus type
func TemplateConsensusType(typeValue string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   ConsensusTypeKey,
		Value: utils.MarshalOrPanic(&ab.ConsensusType{Type: typeValue}),
	}
}

// TemplateBatchSize creates a headerless configuration item representing the batch size
func TemplateBatchSize(batchSize *ab.BatchSize) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   BatchSizeKey,
		Value: utils.MarshalOrPanic(batchSize),
	}
}

// TemplateBatchTimeout creates a headerless configuration item representing the batch timeout
func TemplateBatchTimeout(batchTimeout string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   BatchTimeoutKey,
		Value: utils.MarshalOrPanic(&ab.BatchTimeout{Timeout: batchTimeout}),
	}
}

// TemplateChainCreationPolicyNames creates a headerless configuraiton item representing the chain creation policy names
func TemplateChainCreationPolicyNames(names []string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   ChainCreationPolicyNamesKey,
		Value: utils.MarshalOrPanic(&ab.ChainCreationPolicyNames{Names: names}),
	}
}

// TemplateIngressPolicyNames creates a headerless configuration item representing the ingress policy names
func TemplateIngressPolicyNames(names []string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   IngressPolicyNamesKey,
		Value: utils.MarshalOrPanic(&ab.IngressPolicyNames{Names: names}),
	}
}

// TemplateEgressPolicyNames creates a headerless configuration item representing the egress policy names
func TemplateEgressPolicyNames(names []string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   EgressPolicyNamesKey,
		Value: utils.MarshalOrPanic(&ab.EgressPolicyNames{Names: names}),
	}
}

// TemplateKafkaBrokers creates a headerless configuration item representing the kafka brokers
func TemplateKafkaBrokers(brokers []string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Orderer,
		Key:   KafkaBrokersKey,
		Value: utils.MarshalOrPanic(&ab.KafkaBrokers{Brokers: brokers}),
	}
}
