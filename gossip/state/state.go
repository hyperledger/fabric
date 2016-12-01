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

package state

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

// GossipStateProvider is the interface to acquire sequences of the ledger blocks
// capable to full fill missing blocks by running state replication and
// sending request to get missing block to other nodes
type GossipStateProvider interface {

	// Retrieve block with sequence number equal to index
	GetBlock(index uint64) *common.Block

	AddPayload(payload *proto.Payload) error

	// Stop terminates state transfer object
	Stop()
}

var logFormat = logging.MustStringFormatter(
	`%{color}%{level} %{longfunc}():%{color:reset}(%{module})%{message}`,
)

const (
	defPollingPeriod       = 200 * time.Millisecond
	defAntiEntropyInterval = 10 * time.Second
)

// GossipStateProviderImpl the implementation of the GossipStateProvider interface
// the struct to handle in memory sliding window of
// new ledger block to be acquired by hyper ledger
type GossipStateProviderImpl struct {
	// The gossiping service
	gossip gossip.Gossip

	// Channel to read gossip messages from
	gossipChan <-chan *proto.GossipMessage

	commChan <-chan comm.ReceivedMessage

	// Flag which signals for termination
	stopFlag int32

	mutex sync.RWMutex

	// Queue of payloads which wasn't acquired yet
	payloads PayloadsBuffer

	comm comm.Comm

	committer committer.Committer

	logger *logging.Logger

	done sync.WaitGroup
}

// NewGossipStateProvider creates initialized instance of gossip state provider
func NewGossipStateProvider(g gossip.Gossip, c comm.Comm, committer committer.Committer) GossipStateProvider {
	logger, _ := logging.GetLogger("GossipStateProvider")
	logging.SetLevel(logging.DEBUG, logger.Module)

	gossipChan := g.Accept(func(message interface{}) bool {
		// Get only data messages
		return message.(*proto.GossipMessage).GetDataMsg() != nil
	})

	// Filter message which are only relevant for state transfer
	commChan := c.Accept(func(message interface{}) bool {
		return message.(comm.ReceivedMessage).GetGossipMessage().GetStateRequest() != nil ||
			message.(comm.ReceivedMessage).GetGossipMessage().GetStateResponse() != nil
	})

	height, err := committer.LedgerHeight()

	if err != nil {
		logger.Error("Could not read ledger info to obtain current ledger height due to: ", err)
		// Exiting as without ledger it will be impossible
		// to deliver new blocks
		return nil
	}

	s := &GossipStateProviderImpl{
		// Instance of the gossip
		gossip: g,

		// Channel to read new messages from
		gossipChan: gossipChan,

		// Channel to read direct messages from other peers
		commChan: commChan,

		stopFlag: 0,
		// Create a queue for payload received
		payloads: NewPayloadsBuffer(height + 1),

		comm: c,

		committer: committer,

		logger: logger,
	}

	logging.SetFormatter(logFormat)

	state := NewNodeMetastate(height)

	s.logger.Infof("Updating node metadata information, current ledger sequence is at = %d, next expected block is = %d", state.LedgerHeight, s.payloads.Next())
	bytes, err := state.Bytes()
	if err == nil {
		g.UpdateMetadata(bytes)
	} else {
		s.logger.Errorf("Unable to serialize node meta state, error = %s", err)
	}

	s.done.Add(3)

	// Listen for incoming communication
	go s.listen()
	// Deliver in order messages into the incoming channel
	go s.deliverPayloads()
	// Execute anti entropy to fill missing gaps
	go s.antiEntropy()

	return s
}

func (s *GossipStateProviderImpl) listen() {
	for !s.isDone() {
		// Do not block on waiting message from channel
		// check each 500ms whenever is done indicates to
		// finish
	next:
		select {
		case msg := <-s.gossipChan:
			{
				s.logger.Debug("Received new message via gossip channel")
				go s.queueNewMessage(msg)
			}
		case msg := <-s.commChan:
			{
				s.logger.Debug("Direct message ", msg)
				go s.directMessage(msg)
			}
		case <-time.After(defPollingPeriod):
			break next
		}
	}
	s.logger.Debug("[XXX]: Stop listening for new messages")
	s.done.Done()
}

func (s *GossipStateProviderImpl) directMessage(msg comm.ReceivedMessage) {
	s.logger.Debug("[ENTER] -> directMessage")
	defer s.logger.Debug("[EXIT] ->  directMessage")

	if msg == nil {
		s.logger.Error("Got nil message via end-to-end channel, should not happen!")
		return
	}

	incoming := msg.GetGossipMessage()

	if incoming.GetStateRequest() != nil {
		s.handleStateRequest(msg)
	} else if incoming.GetStateResponse() != nil {
		s.handleStateResponse(msg)
	}
}

func (s *GossipStateProviderImpl) handleStateRequest(msg comm.ReceivedMessage) {
	request := msg.GetGossipMessage().GetStateRequest()
	response := &proto.RemoteStateResponse{Payloads: make([]*proto.Payload, 0)}
	for _, seqNum := range request.SeqNums {
		s.logger.Debug("Reading block ", seqNum, " from the committer service")
		blocks := s.committer.GetBlocks([]uint64{seqNum})

		if blocks == nil || len(blocks) < 1 {
			s.logger.Errorf("Wasn't able to read block with sequence number %d from ledger, skipping....", seqNum)
			continue
		}

		blockBytes, err := pb.Marshal(blocks[0])
		if err != nil {
			s.logger.Errorf("Could not marshal block: %s", err)
		}

		if err != nil {
			s.logger.Errorf("Could not calculate hash of block: %s", err)
		}

		response.Payloads = append(response.Payloads, &proto.Payload{
			SeqNum: seqNum,
			Data:   blockBytes,
			// TODO: Check hash generation for given block from the ledger
			Hash: "",
		})
	}
	// Sending back response with missing blocks
	msg.Respond(&proto.GossipMessage{
		Content: &proto.GossipMessage_StateResponse{response},
	})
}

func (s *GossipStateProviderImpl) handleStateResponse(msg comm.ReceivedMessage) {
	response := msg.GetGossipMessage().GetStateResponse()
	for _, payload := range response.GetPayloads() {
		s.logger.Debugf("Received payload with sequence number %d.", payload.SeqNum)
		err := s.payloads.Push(payload)
		if err != nil {
			s.logger.Warningf("Payload with sequence number %d was received earlier", payload.SeqNum)
		}
	}
}

// Internal function to check whenever we need to finish listening
// for new messages to arrive
func (s *GossipStateProviderImpl) isDone() bool {
	return atomic.LoadInt32(&s.stopFlag) == 1
}

// Stop function send halting signal to all go routines
func (s *GossipStateProviderImpl) Stop() {
	atomic.StoreInt32(&s.stopFlag, 1)
	s.done.Wait()
	s.committer.Close()
}

// New message notification/handler
func (s *GossipStateProviderImpl) queueNewMessage(msg *proto.GossipMessage) {
	dataMsg := msg.GetDataMsg()
	if dataMsg != nil {
		// Add new payload to ordered set
		s.logger.Debugf("Received new payload with sequence number = [%d]", dataMsg.Payload.SeqNum)
		s.payloads.Push(dataMsg.GetPayload())
	} else {
		s.logger.Debug("Gossip message received is not of data message type, usually this should not happen.")
	}
}

func (s *GossipStateProviderImpl) deliverPayloads() {
	for !s.isDone() {
	next:
		select {
		// Wait for notification that next seq has arrived
		case <-s.payloads.Ready():
			{
				s.logger.Debugf("Ready to transfer payloads to the ledger, next sequence number is = [%d]", s.payloads.Next())
				// Collect all subsequent payloads
				for payload := s.payloads.Pop(); payload != nil; payload = s.payloads.Pop() {
					rawblock := &common.Block{}
					if err := pb.Unmarshal(payload.Data, rawblock); err != nil {
						s.logger.Errorf("Error getting block with seqNum = %d due to (%s)...dropping block\n", payload.SeqNum, err)
						continue
					}
					s.logger.Debug("New block with sequence number ", payload.SeqNum, " transactions num ", len(rawblock.Data.Data))
					s.commitBlock(rawblock, payload.SeqNum)
				}
			}
		case <-time.After(defPollingPeriod):
			{
				break next
			}
		}
	}
	s.logger.Debug("State provider has been stoped, finishing to push new blocks.")
	s.done.Done()
}

func (s *GossipStateProviderImpl) antiEntropy() {
	checkPoint := time.Now()
	for !s.isDone() {
		time.Sleep(defPollingPeriod)
		if time.Since(checkPoint).Nanoseconds() <= defAntiEntropyInterval.Nanoseconds() {
			continue
		}
		checkPoint = time.Now()

		current, _ := s.committer.LedgerHeight()
		max, _ := s.committer.LedgerHeight()

		for _, p := range s.gossip.GetPeers() {
			if state, err := FromBytes(p.Metadata); err == nil {
				if max < state.LedgerHeight {
					max = state.LedgerHeight
				}
			}
		}

		if current == max {
			// No messages in the buffer or there are no gaps
			s.logger.Debugf("Current ledger height is the same as ledger height on other peers.")
			continue
		}

		s.logger.Debugf("Requesting new blocks in range [%d...%d].", current+1, max)
		s.requestBlocksInRange(uint64(current+1), uint64(max))
	}
	s.logger.Debug("[XXX]: Stateprovider stopped, stoping anti entropy procedure.")
	s.done.Done()
}

// GetBlocksInRange capable to acquire blocks with sequence
// numbers in the range [start...end].
func (s *GossipStateProviderImpl) requestBlocksInRange(start uint64, end uint64) {
	var peers []*comm.RemotePeer
	// Filtering peers which might have relevant blocks
	for _, value := range s.gossip.GetPeers() {
		nodeMetadata, err := FromBytes(value.Metadata)
		if err == nil {
			if nodeMetadata.LedgerHeight >= end {
				peers = append(peers, &comm.RemotePeer{Endpoint: value.Endpoint, PKIID: value.PKIid})
			}
		} else {
			s.logger.Errorf("Unable to de-serialize node meta state, error = %s", err)
		}
	}

	n := len(peers)
	if n == 0 {
		s.logger.Warningf("There is not peer nodes to ask for missing blocks in range [%d, %d)", start, end)
		return
	}
	// Select peers to ask for blocks
	peer := peers[rand.Intn(n)]
	s.logger.Infof("State transfer, with peer %s, the min available sequence number %d next block %d", peer.Endpoint, start, end)

	request := &proto.RemoteStateRequest{
		SeqNums: make([]uint64, 0),
	}

	for i := start; i <= end; i++ {
		request.SeqNums = append(request.SeqNums, uint64(i))
	}

	s.logger.Debug("[$$$$$$$$$$$$$$$$]: Sending direct request to complete missing blocks, ", request)
	s.comm.Send(&proto.GossipMessage{
		Content: &proto.GossipMessage_StateRequest{request},
	}, peer)
}

func (s *GossipStateProviderImpl) GetBlock(index uint64) *common.Block {
	// Try to read missing block from the ledger, should return no nil with
	// content including at least one block
	if blocks := s.committer.GetBlocks([]uint64{index}); blocks != nil && len(blocks) > 0 {
		return blocks[0]
	}

	return nil
}

func (s *GossipStateProviderImpl) AddPayload(payload *proto.Payload) error {
	return s.payloads.Push(payload)
}

func (s *GossipStateProviderImpl) commitBlock(block *common.Block, seqNum uint64) error {
	if err := s.committer.CommitBlock(block); err != nil {
		s.logger.Errorf("Got error while committing(%s)\n", err)
		return err
	}

	// Update ledger level within node metadata
	state := NewNodeMetastate(seqNum)
	// Decode state to byte array
	bytes, err := state.Bytes()
	if err == nil {
		s.gossip.UpdateMetadata(bytes)
	} else {
		s.logger.Errorf("Unable to serialize node meta state, error = %s", err)
	}

	s.logger.Debug("[XXX]: Commit success, created a block!")
	return nil
}
