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

package config

import (
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

func ordererConfigGroup(key string, value []byte) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Groups[OrdererGroupKey] = cb.NewConfigGroup()
	result.Groups[OrdererGroupKey].Values[key] = &cb.ConfigValue{
		Value: value,
	}
	return result
}

// TemplateConsensusType creates a headerless config item representing the consensus type
func TemplateConsensusType(typeValue string) *cb.ConfigGroup {
	return ordererConfigGroup(ConsensusTypeKey, utils.MarshalOrPanic(&ab.ConsensusType{Type: typeValue}))
}

// TemplateBatchSize creates a headerless config item representing the batch size
func TemplateBatchSize(batchSize *ab.BatchSize) *cb.ConfigGroup {
	return ordererConfigGroup(BatchSizeKey, utils.MarshalOrPanic(batchSize))
}

// TemplateBatchTimeout creates a headerless config item representing the batch timeout
func TemplateBatchTimeout(batchTimeout string) *cb.ConfigGroup {
	return ordererConfigGroup(BatchTimeoutKey, utils.MarshalOrPanic(&ab.BatchTimeout{Timeout: batchTimeout}))
}

// TemplateChannelRestrictions creates a config group with ChannelRestrictions specified
func TemplateChannelRestrictions(maxChannels uint64) *cb.ConfigGroup {
	return ordererConfigGroup(ChannelRestrictionsKey, utils.MarshalOrPanic(&ab.ChannelRestrictions{MaxCount: maxChannels}))
}

// TemplateKafkaBrokers creates a headerless config item representing the kafka brokers
func TemplateKafkaBrokers(brokers []string) *cb.ConfigGroup {
	return ordererConfigGroup(KafkaBrokersKey, utils.MarshalOrPanic(&ab.KafkaBrokers{Brokers: brokers}))
}
