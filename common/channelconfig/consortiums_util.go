/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

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
