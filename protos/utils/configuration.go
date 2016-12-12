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

package utils

import (
	cu "github.com/hyperledger/fabric/core/util"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

const CreationPolicyKey = "CreationPolicy"

// ChainCreationConfiguration creates a new chain creation configuration envelope from
// the supplied creationPolicy, new chainID, and a template configuration envelope
// The template configuration envelope will have the correct chainID set on all items,
// and the first item will be a CreationPolicy which is ready for the signatures as
// required by the policy
func ChainCreationConfiguration(creationPolicy, newChainID string, template *cb.ConfigurationEnvelope) *cb.ConfigurationEnvelope {
	newConfigItems := make([]*cb.SignedConfigurationItem, len(template.Items))
	var hashBytes []byte

	for i, item := range template.Items {
		configItem := UnmarshalConfigurationItemOrPanic(item.ConfigurationItem)
		configItem.Header.ChainID = newChainID
		newConfigItems[i] = &cb.SignedConfigurationItem{
			ConfigurationItem: MarshalOrPanic(configItem),
		}
		hashBytes = append(hashBytes, newConfigItems[i].ConfigurationItem...)
	}

	digest := cu.ComputeCryptoHash(hashBytes)

	authorizeItem := &cb.SignedConfigurationItem{
		ConfigurationItem: MarshalOrPanic(&cb.ConfigurationItem{
			Header: &cb.ChainHeader{
				ChainID: newChainID,
				Type:    int32(cb.HeaderType_CONFIGURATION_ITEM),
			},
			Type: cb.ConfigurationItem_Orderer,
			Key:  CreationPolicyKey,
			Value: MarshalOrPanic(&ab.CreationPolicy{
				Policy: creationPolicy,
				Digest: digest,
			}),
		}),
	}

	authorizedConfig := append([]*cb.SignedConfigurationItem{authorizeItem}, newConfigItems...)

	return &cb.ConfigurationEnvelope{
		Items: authorizedConfig,
	}
}

// ChainCreationConfigurationTransaction creates a new chain creation configuration transaction
// by invoking ChainCreationConfiguration and embedding the resulting configuration envelope is a
// configuration transaction
func ChainCreationConfigurationTransaction(creationPolicy, newChainID string, template *cb.ConfigurationEnvelope) *cb.Envelope {
	configurationEnvelope := ChainCreationConfiguration(creationPolicy, newChainID, template)

	newGenesisTx := &cb.Envelope{
		Payload: MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChainHeader: &cb.ChainHeader{
					Type:    int32(cb.HeaderType_CONFIGURATION_TRANSACTION),
					ChainID: newChainID,
				},
			},
			Data: MarshalOrPanic(configurationEnvelope),
		}),
	}

	return newGenesisTx
}
