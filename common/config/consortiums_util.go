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
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	// ChannelCreationPolicyKey is the key for the ChannelCreationPolicy value
	ChannelCreationPolicyKey = "ChannelCreationPolicy"
)

// TemplateConsortiumsGroup creates an empty consortiums group
func TemplateConsortiumsGroup() *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Groups[ConsortiumsGroupKey] = cb.NewConfigGroup()
	return result
}

// TemplateConsortiumChannelCreationPolicy sets the ChannelCreationPolicy for a given consortium
func TemplateConsortiumChannelCreationPolicy(name string, policy *cb.Policy) *cb.ConfigGroup {
	result := TemplateConsortiumsGroup()
	result.Groups[ConsortiumsGroupKey].Groups[name] = cb.NewConfigGroup()
	result.Groups[ConsortiumsGroupKey].Groups[name].Values[ChannelCreationPolicyKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(policy),
	}
	return result
}
