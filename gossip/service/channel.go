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

package service

import (
	"fmt"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/utils"
)

// unMarshal is used to un-marshall proto-buffer types.
// In tests, I override this variable with a function
var unMarshal func (buf []byte, pb proto.Message) error

func init() {
	unMarshal = proto.Unmarshal
}

// This is an implementation of api.JoinChannelMessage.
// This object is created from a *common.Block
type joinChannelMessage struct {
	seqNum uint64
	anchorPeers []api.AnchorPeer
}

// JoinChannelMessageFromBlock returns an api.JoinChannelMessage, nil
// or nil, error if the block is not a valid channel configuration block
func JoinChannelMessageFromBlock(block *common.Block) (api.JoinChannelMessage, error) {
	anchorPeers, err := parseBlock(block)
	if err != nil {
		return nil, err
	}
	jcm := &joinChannelMessage{seqNum: block.Header.Number, anchorPeers: []api.AnchorPeer{}}
	for _, ap := range anchorPeers.AnchorPees {
		anchorPeer := api.AnchorPeer{
			Host: ap.Host,
			Port: int(ap.Port),
			Cert: api.PeerIdentityType(ap.Cert),
		}
		jcm.anchorPeers = append(jcm.anchorPeers, anchorPeer)
	}
	return jcm, nil
}

// parseBlock parses a configuration block, and returns error
// if it's not a valid channel configuration block
func parseBlock(block *common.Block) (*peer.AnchorPeers, error) {
	confEnvelope, err := extractConfigurationEnvelope(block)
	if err != nil {
		return nil, err
	}
	// Find anchor peer configuration
	found := false
	var anchorPeerConfig *common.ConfigurationItem
	for _, item := range confEnvelope.Items {
		rawConfItem := item.ConfigurationItem
		confItem := &common.ConfigurationItem{}
		if err := unMarshal(rawConfItem, confItem); err != nil {
			return nil, fmt.Errorf("Failed unmarshalling configuration item")
		}
		if confItem.Header.Type != int32(common.HeaderType_CONFIGURATION_ITEM) {
			continue
		}
		if confItem.Type != common.ConfigurationItem_Peer {
			continue
		}
		if confItem.Key != utils.AnchorPeerConfItemKey {
			continue
		}
		if found {
			return nil, fmt.Errorf("Found multiple definition of AnchorPeers instead of a single one")
		}
		found = true
		anchorPeerConfig = confItem
	}
	if ! found {
		return nil, fmt.Errorf("Didn't find AnchorPeer definition in configuration block")
	}
	rawAnchorPeersBytes := anchorPeerConfig.Value
	anchorPeers := &peer.AnchorPeers{}
	if err := unMarshal(rawAnchorPeersBytes, anchorPeers); err != nil {
		return nil, fmt.Errorf("Failed deserializing anchor peers from configuration item")
	}
	if len(anchorPeers.AnchorPees) == 0 {
		return nil, fmt.Errorf("AnchorPeers field in configuration block was found, but is empty")
	}
	return anchorPeers, nil
}

// extractConfigurationEnvelope extracts the configuration envelope from a block,
// or returns nil and an error if extraction fails
func extractConfigurationEnvelope(block *common.Block) (*common.ConfigurationEnvelope, error) {
	if block.Header == nil {
		return nil, fmt.Errorf("Block header is empty")
	}
	if block.Data == nil {
		return nil, fmt.Errorf("Block data is empty")
	}
	env := &common.Envelope{}
	if len(block.Data.Data) == 0 {
		return nil, fmt.Errorf("Empty data in block")
	}
	if len(block.Data.Data) != 1 {
		return nil, fmt.Errorf("More than 1 transaction in a block")
	}
	if err := unMarshal(block.Data.Data[0], env); err != nil {
		return nil, fmt.Errorf("Failed unmarshalling envelope from block: %v", err)
	}
	payload := &common.Payload{}
	if err := unMarshal(env.Payload, payload); err != nil {
		return nil, fmt.Errorf("Failed unmarshalling payload from envelope: %v", err)
	}
	confEnvelope := &common.ConfigurationEnvelope{}
	if err := unMarshal(payload.Data, confEnvelope); err != nil {
		return nil, fmt.Errorf("Failed unmarshalling configuration envelope from payload: %v", err)
	}
	if len(confEnvelope.Items) == 0 {
		return nil, fmt.Errorf("Empty configuration envelope, no items detected")
	}
	return confEnvelope, nil
}

func (jcm *joinChannelMessage) SequenceNumber() uint64 {
	return jcm.seqNum
}

func (jcm *joinChannelMessage) AnchorPeers() []api.AnchorPeer {
	return jcm.anchorPeers
}
