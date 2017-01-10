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

package deliverclient

import (
	"math"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/events/producer"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	gossip_proto "github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("noopssinglechain.client")
}

// DeliverService used to communicate with orderers to obtain
// new block and send the to the committer service
type DeliverService struct {
	client orderer.AtomicBroadcast_DeliverClient

	chainID string
	conn    *grpc.ClientConn
}

// StopDeliveryService sends stop to the delivery service reference
func StopDeliveryService(service *DeliverService) {
	if service != nil {
		service.Stop()
	}
}

// NewDeliverService construction function to create and initilize
// delivery service instance
func NewDeliverService(chainID string) *DeliverService {
	if viper.GetBool("peer.committer.enabled") {
		logger.Infof("Creating committer for single noops endorser")
		deliverService := &DeliverService{
			// Instance of RawLedger
			chainID: chainID,
		}

		return deliverService
	}
	logger.Infof("Committer disabled")
	return nil
}

func (d *DeliverService) startDeliver(committer committer.Committer) error {
	logger.Info("Starting deliver service client")
	err := d.initDeliver()

	if err != nil {
		logger.Errorf("Can't initiate deliver protocol [%s]", err)
		return err
	}

	height, err := committer.LedgerHeight()
	if err != nil {
		logger.Errorf("Can't get legder height from committer [%s]", err)
		return err
	}

	if height > 0 {
		logger.Debugf("Starting deliver with block [%d]", height)
		if err := d.seekLatestFromCommitter(height); err != nil {
			return err
		}

	} else {
		logger.Debug("Starting deliver with olders block")
		if err := d.seekOldest(); err != nil {
			return err
		}

	}

	d.readUntilClose()

	return nil
}

func (d *DeliverService) initDeliver() error {
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(3 * time.Second), grpc.WithBlock()}
	endpoint := viper.GetString("peer.committer.ledger.orderer")
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		logger.Errorf("Cannot dial to %s, because of %s", endpoint, err)
		return err
	}
	var abc orderer.AtomicBroadcast_DeliverClient
	abc, err = orderer.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
	if err != nil {
		logger.Errorf("Unable to initialize atomic broadcast, due to %s", err)
		return err
	}

	// Atomic Broadcast Deliver Client
	d.client = abc
	d.conn = conn
	return nil

}

func (d *DeliverService) stopDeliver() {
	if d.conn != nil {
		d.conn.Close()
	}
}

// Stop all service and release resources
func (d *DeliverService) Stop() {
	d.stopDeliver()
}

// Start delivery service
func (d *DeliverService) Start(committer committer.Committer) {
	go d.checkLeaderAndRunDeliver(committer)
}

func (d *DeliverService) checkLeaderAndRunDeliver(committer committer.Committer) {
	isLeader := viper.GetBool("peer.gossip.orgLeader")

	if isLeader {
		d.startDeliver(committer)
	}
}

func (d *DeliverService) seekOldest() error {
	return d.client.Send(&orderer.SeekInfo{
		ChainID:  d.chainID,
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Oldest{Oldest: &orderer.SeekOldest{}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	})
}

func (d *DeliverService) seekLatestFromCommitter(height uint64) error {
	return d.client.Send(&orderer.SeekInfo{
		ChainID:  d.chainID,
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: height}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	})
}

func (d *DeliverService) readUntilClose() {
	for {
		msg, err := d.client.Recv()
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

			// Create new transactions validator
			validator := txvalidator.NewTxValidator(peer.GetLedger(d.chainID))
			// Validate and mark invalid transactions
			logger.Debug("Validating block, chainID", d.chainID)
			validator.Validate(t.Block)

			numberOfPeers := len(service.GetGossipService().PeersOfChannel(gossipcommon.ChainID(d.chainID)))
			// Create payload with a block received
			payload := createPayload(seqNum, t.Block)
			// Use payload to create gossip message
			gossipMsg := createGossipMsg(d.chainID, payload)
			logger.Debug("Creating gossip message", gossipMsg)

			logger.Debugf("Adding payload locally, buffer seqNum = [%d], peers number [%d]", seqNum, numberOfPeers)
			// Add payload to local state payloads buffer
			service.GetGossipService().AddPayload(d.chainID, payload)

			// Gossip messages with other nodes
			logger.Debugf("Gossiping block [%d], peers number [%d]", seqNum, numberOfPeers)
			service.GetGossipService().Gossip(gossipMsg)
			if err = producer.SendProducerBlockEvent(t.Block); err != nil {
				logger.Errorf("Error sending block event %s", err)
			}

		default:
			logger.Warning("Received unknown: ", t)
			return
		}
	}
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
