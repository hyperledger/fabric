/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func applicationOrgConfigGroup(orgID string, key string, value []byte) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey].Groups[orgID] = cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey].Groups[orgID].Values[key] = &cb.ConfigValue{
		Value: value,
	}
	return result
}

func applicationConfigGroup(key string, value []byte) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey].Values[key] = &cb.ConfigValue{
		Value: value,
	}
	return result
}

// TemplateAnchorPeers creates a headerless config item representing the anchor peers
func TemplateAnchorPeers(orgID string, anchorPeers []*pb.AnchorPeer) *cb.ConfigGroup {
	return applicationOrgConfigGroup(orgID, AnchorPeersKey, utils.MarshalOrPanic(&pb.AnchorPeers{AnchorPeers: anchorPeers}))
}

// TemplateApplicationCapabilities creates a config value representing the application capabilities
func TemplateApplicationCapabilities(capabilities map[string]bool) *cb.ConfigGroup {
	return applicationConfigGroup(CapabilitiesKey, utils.MarshalOrPanic(capabilitiesFromBoolMap(capabilities)))
}
