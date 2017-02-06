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

package blocksprovider

import (
	"math"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"

	"github.com/hyperledger/fabric/protos/common"
	gossip_proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

// LedgerInfo an adapter to provide the interface to query
// the ledger committer for current ledger height
type LedgerInfo interface {
	// LedgerHeight returns current local ledger height
	LedgerHeight() (uint64, error)
}

// GossipServiceAdapter serves to provide basic functionality
// required from gossip service by delivery service
type GossipServiceAdapter interface {
	// PeersOfChannel returns slice with members of specified channel
	PeersOfChannel(gossipcommon.ChainID) []discovery.NetworkMember

	// AddPayload adds payload to the local state sync buffer
	AddPayload(chainID string, payload *gossip_proto.Payload) error

	// Gossip the message across the peers
	Gossip(msg *gossip_proto.GossipMessage)
}

// BlocksProvider used to read blocks from the ordering service
// for specified chain it subscribed to
type BlocksProvider interface {
	// RequestBlock acquire new blocks from ordering service based on
	// information provided by ledger info instance
	RequestBlocks(ledgerInfoProvider LedgerInfo) error

	// DeliverBlocks starts delivering and disseminating blocks
	DeliverBlocks()

	// Stop shutdowns blocks provider and stops delivering new blocks
	Stop()
}

// BlocksDeliverer defines interface which actually helps
// to abstract the AtomicBroadcast_DeliverClient with only
// required method for blocks provider. This also help to
// build up mocking facilities for testing purposes
type BlocksDeliverer interface {
	// Recv capable to bring new blocks from the ordering service
	Recv() (*orderer.DeliverResponse, error)

	// Send used to send request to the ordering service to obtain new blocks
	Send(*common.Envelope) error
}

// blocksProviderImpl the actual implementation for BlocksProvider interface
type blocksProviderImpl struct {
	chainID string

	client BlocksDeliverer

	gossip GossipServiceAdapter

	done int32
}

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("blocksProvider")
}

// NewBlocksProvider constructor function to creare blocks deliverer instance
func NewBlocksProvider(chainID string, client BlocksDeliverer, gossip GossipServiceAdapter) BlocksProvider {
	return &blocksProviderImpl{
		chainID: chainID,
		client:  client,
		gossip:  gossip,
	}
}

// DeliverBlocks used to pull out blocks from the ordering service to
// distributed them across peers
func (b *blocksProviderImpl) DeliverBlocks() {
	for !b.isDone() {
		msg, err := b.client.Recv()
		if err != nil {
			logger.Warningf("Receive error: %s", err.Error())
			return
		}
		switch t := msg.Type.(type) {
		case *orderer.DeliverResponse_Status:
			if t.Status == common.Status_SUCCESS {
				logger.Warning("ERROR! Received success for a seek that should never complete")
				return
			}
			logger.Warning("Got error ", t)
		case *orderer.DeliverResponse_Block:
			seqNum := t.Block.Header.Number

			numberOfPeers := len(b.gossip.PeersOfChannel(gossipcommon.ChainID(b.chainID)))
			// Create payload with a block received
			payload := createPayload(seqNum, t.Block)
			// Use payload to create gossip message
			gossipMsg := createGossipMsg(b.chainID, payload)

			logger.Debugf("Adding payload locally, buffer seqNum = [%d], peers number [%d]", seqNum, numberOfPeers)
			// Add payload to local state payloads buffer
			b.gossip.AddPayload(b.chainID, payload)

			// Gossip messages with other nodes
			logger.Debugf("Gossiping block [%d], peers number [%d]", seqNum, numberOfPeers)
			b.gossip.Gossip(gossipMsg)
		default:
			logger.Warning("Received unknown: ", t)
			return
		}
	}
}

// Stops blocks delivery provider
func (b *blocksProviderImpl) Stop() {
	atomic.StoreInt32(&b.done, 1)
}

// Check whenever provider is stopped
func (b *blocksProviderImpl) isDone() bool {
	return atomic.LoadInt32(&b.done) == 1
}

func (b *blocksProviderImpl) RequestBlocks(ledgerInfoProvider LedgerInfo) error {
	height, err := ledgerInfoProvider.LedgerHeight()
	if err != nil {
		logger.Errorf("Can't get legder height from committer [%s]", err)
		return err
	}

	if height > 0 {
		logger.Debugf("Starting deliver with block [%d]", height)
		if err := b.seekLatestFromCommitter(height); err != nil {
			return err
		}
	} else {
		logger.Debug("Starting deliver with olders block")
		if err := b.seekOldest(); err != nil {
			return err
		}
	}

	return nil
}

func (b *blocksProviderImpl) seekOldest() error {
	return b.client.Send(&common.Envelope{
		Payload: utils.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChainHeader: &common.ChainHeader{
					ChainID: b.chainID,
				},
				SignatureHeader: &common.SignatureHeader{},
			},
			Data: utils.MarshalOrPanic(&orderer.SeekInfo{
				Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Oldest{Oldest: &orderer.SeekOldest{}}},
				Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
				Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	})
}

func (b *blocksProviderImpl) seekLatestFromCommitter(height uint64) error {
	return b.client.Send(&common.Envelope{
		Payload: utils.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChainHeader: &common.ChainHeader{
					ChainID: b.chainID,
				},
				SignatureHeader: &common.SignatureHeader{},
			},
			Data: utils.MarshalOrPanic(&orderer.SeekInfo{
				Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: height}}},
				Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
				Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	})
}

func createGossipMsg(chainID string, payload *gossip_proto.Payload) *gossip_proto.GossipMessage {
	gossipMsg := &gossip_proto.GossipMessage{
		Nonce:   0,
		Tag:     gossip_proto.GossipMessage_CHAN_AND_ORG,
		Channel: []byte(chainID),
		Content: &gossip_proto.GossipMessage_DataMsg{
			DataMsg: &gossip_proto.DataMessage{
				Payload: payload,
			},
		},
	}
	return gossipMsg
}

func createPayload(seqNum uint64, block *common.Block) *gossip_proto.Payload {
	marshaledBlock, _ := proto.Marshal(block)
	return &gossip_proto.Payload{
		Data:   marshaledBlock,
		SeqNum: seqNum,
	}
}
