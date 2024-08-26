/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// GossipServiceAdapter serves to provide basic functionality
// required from gossip service by delivery service
//
//go:generate counterfeiter -o fake/gossip_service_adapter.go --fake-name GossipServiceAdapter . GossipServiceAdapter
type GossipServiceAdapter interface {
	// AddPayload adds payload to the local state sync buffer
	AddPayload(chainID string, payload *gossip.Payload) error

	// Gossip the message across the peers
	Gossip(msg *gossip.GossipMessage)
}

type GossipBlockHandler struct {
	gossip              GossipServiceAdapter
	blockGossipDisabled bool
	logger              *flogging.FabricLogger
}

func (h *GossipBlockHandler) HandleBlock(channelID string, block *common.Block) error {
	if block == nil {
		return errors.New("block from orderer could not be re-marshaled: proto: Marshal called with nil")
	}
	marshaledBlock, err := proto.Marshal(block)
	if err != nil {
		return errors.WithMessage(err, "block from orderer could not be re-marshaled")
	}

	// Create payload with a block received
	blockNum := block.GetHeader().GetNumber()
	payload := &gossip.Payload{
		Data:   marshaledBlock,
		SeqNum: blockNum,
	}

	// Use payload to create gossip message
	gossipMsg := &gossip.GossipMessage{
		Nonce:   0,
		Tag:     gossip.GossipMessage_CHAN_AND_ORG,
		Channel: []byte(channelID),
		Content: &gossip.GossipMessage_DataMsg{
			DataMsg: &gossip.DataMessage{
				Payload: payload,
			},
		},
	}

	h.logger.Debugf("Adding payload to local buffer, blockNum = [%d]", blockNum)
	// Add payload to local state payloads buffer
	if err := h.gossip.AddPayload(channelID, payload); err != nil {
		h.logger.Warningf("Block [%d] received from ordering service wasn't added to payload buffer: %v", blockNum, err)
		return errors.WithMessage(err, "could not add block as payload")
	}
	if h.blockGossipDisabled {
		return nil
	}
	// Gossip messages with other nodes
	h.logger.Debugf("Gossiping block [%d]", blockNum)
	h.gossip.Gossip(gossipMsg)

	return nil
}
