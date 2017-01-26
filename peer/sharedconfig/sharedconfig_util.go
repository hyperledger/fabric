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
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

var defaultAnchorPeers = []*pb.AnchorPeer{}

// TemplateAnchorPeers creates a headerless configuration item representing the anchor peers
func TemplateAnchorPeers(anchorPeers []*pb.AnchorPeer) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Peer,
		Key:   AnchorPeersKey,
		Value: utils.MarshalOrPanic(&pb.AnchorPeers{AnchorPeers: anchorPeers}),
	}
}

// DefaultAnchorPeers creates a headerless configuration item for the default orderer addresses
func DefaultAnchorPeers() *cb.ConfigurationItem {
	return TemplateAnchorPeers(defaultAnchorPeers)
}
