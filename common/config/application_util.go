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
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func applicationConfigGroup(orgID string, key string, value []byte) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey].Groups[orgID] = cb.NewConfigGroup()
	result.Groups[ApplicationGroupKey].Groups[orgID].Values[key] = &cb.ConfigValue{
		Value: value,
	}
	return result
}

// TemplateAnchorPeers creates a headerless config item representing the anchor peers
func TemplateAnchorPeers(orgID string, anchorPeers []*pb.AnchorPeer) *cb.ConfigGroup {
	return applicationConfigGroup(orgID, AnchorPeersKey, utils.MarshalOrPanic(&pb.AnchorPeers{AnchorPeers: anchorPeers}))
}
