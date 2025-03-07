/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"bytes"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	vsccErrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	common2 "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// GossipStateProvider is the interface to acquire sequences of the ledger blocks
// capable to full fill missing blocks by running state replication and
// sending request to get missing block to other nodes
type GossipStateProvider interface {
	AddPayload(payload *proto.Payload) error

	// Stop terminates state transfer object
	Stop()
}

const (
	stragglerWarningThreshold = 100
	defAntiEntropyBatchSize   = 10
	defMaxBlockDistance       = 20

	blocking    = true
	nonBlocking = false

	enqueueRetryInterval = time.Millisecond * 100
)

// Configuration keeps state transfer configuration parameters
type Configuration struct {
	AntiEntropyInterval             time.Duration
	AntiEntropyStateResponseTimeout time.Duration
	AntiEntropyBatchSize            uint64
	MaxBlockDistance                int
	AntiEntropyMaxRetries           int
	ChannelBufferSize               int
	EnableStateTransfer             bool
}

// GossipAdapter defines gossip/communication required interface for state provider
type GossipAdapter interface {
	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common2.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan protoext.ReceivedMessage)

	// UpdateLedgerHeight updates the ledger height the peer
	// publishes to other peers in the channel
	UpdateLedgerHeight(height uint64, channelID common2.ChannelID)

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common2.ChannelID) []discovery.NetworkMember
}

// MCSAdapter adapter of message crypto service interface to bound
// specific APIs required by state transfer service
type MCSAdapter interface {
	// VerifyBlock returns nil if the block is properly signed, and the claimed seqNum is the
	// sequence number that the block's header contains.
	// else returns error
	VerifyBlock(channelID common2.ChannelID, seqNum uint64, signedBlock *common.Block) error

	// VerifyByChannel checks that signature is a valid signature of message
	// under a peer's verification key, but also in the context of a specific channel.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If peerIdentity is nil, then the verification fails.
	VerifyByChannel(channelID common2.ChannelID, peerIdentity api.PeerIdentityType, signature, message []byte) error
}

// ledgerResources defines abilities that the ledger provides
type ledgerResources interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) error

	// StorePvtData used to persist private data into transient store
	StorePvtData(txid string, privData *transientstore.TxPvtReadWriteSetWithConfigInfo, blckHeight uint64) error

	// GetPvtDataAndBlockByNum gets block by number and also returns all related private data
	// that requesting peer is eligible for.
	// The order of private data in slice of PvtDataCollections doesn't imply the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64, peerAuthInfo protoutil.SignedData) (*common.Block, util.PvtDataCollections, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close ledgerResources
	Close()
}

// ServicesMediator aggregated adapter to compound all mediator
// required by state transfer into single struct
type ServicesMediator struct {
	GossipAdapter
	MCSAdapter
}

// GossipStateProviderImpl the implementation of the GossipStateProvider interface
// the struct to handle in memory sliding window of
// new ledger block to be acquired by hyper ledger
type GossipStateProviderImpl struct {
	logger util.Logger

	// Chain id
	chainID string

	mediator *ServicesMediator

	// Queue of payloads which wasn't acquired yet
	payloads PayloadsBuffer

	ledger ledgerResources

	stateResponseCh chan protoext.ReceivedMessage

	stateRequestCh chan protoext.ReceivedMessage

	stopCh chan struct{}

	once sync.Once

	stateTransferActive int32

	stateMetrics *metrics.StateMetrics

	requestValidator *stateRequestValidator

	blockingMode bool

	config *StateConfig
}

// stateRequestValidator facilitates validation of the state request messages
type stateRequestValidator struct{}

// validate checks for RemoteStateRequest message validity
func (v *stateRequestValidator) validate(request *proto.RemoteStateRequest, batchSize uint64) error {
	if request.StartSeqNum > request.EndSeqNum {
		return errors.Errorf("Invalid sequence interval [%d...%d).", request.StartSeqNum, request.EndSeqNum)
	}

	if request.EndSeqNum > batchSize+request.StartSeqNum {
		return errors.Errorf("Requesting blocks range [%d-%d) greater than configured allowed"+
			" (%d) batching size for anti-entropy.", request.StartSeqNum, request.EndSeqNum, batchSize)
	}
	return nil
}

// NewGossipStateProvider creates state provider with coordinator instance
// to orchestrate arrival of private rwsets and blocks before committing them into the ledger.
func NewGossipStateProvider(
	logger util.Logger,
	chainID string,
	services *ServicesMediator,
	ledger ledgerResources,
	stateMetrics *metrics.StateMetrics,
	blockingMode bool,
	config *StateConfig,
) GossipStateProvider {
	gossipChan, _ := services.Accept(func(message interface{}) bool {
		// Get only data messages
		return protoext.IsDataMsg(message.(*proto.GossipMessage)) &&
			bytes.Equal(message.(*proto.GossipMessage).Channel, []byte(chainID))
	}, false)

	remoteStateMsgFilter := func(message interface{}) bool {
		receivedMsg := message.(protoext.ReceivedMessage)
		msg := receivedMsg.GetGossipMessage()
		if !(protoext.IsRemoteStateMessage(msg.GossipMessage) || msg.GetPrivateData() != nil) {
			return false
		}
		// Ensure we deal only with messages that belong to this channel
		if !bytes.Equal(msg.Channel, []byte(chainID)) {
			return false
		}
		connInfo := receivedMsg.GetConnectionInfo()
		authErr := services.VerifyByChannel(msg.Channel, connInfo.Identity, connInfo.Auth.Signature, connInfo.Auth.SignedData)
		if authErr != nil {
			logger.Warning("Got unauthorized request from", string(connInfo.Identity))
			return false
		}
		return true
	}

	// Filter message which are only relevant for nodeMetastate transfer
	_, commChan := services.Accept(remoteStateMsgFilter, true)

	height, err := ledger.LedgerHeight()
	if height == 0 {
		// Panic here since this is an indication of invalid situation which should not happen in normal
		// code path.
		logger.Panic("Committer height cannot be zero, ledger should include at least one block (genesis).")
	}

	if err != nil {
		logger.Error("Could not read ledger info to obtain current ledger height due to: ", errors.WithStack(err))
		// Exiting as without ledger it will be impossible
		// to deliver new blocks
		return nil
	}

	s := &GossipStateProviderImpl{
		logger: logger,
		// MessageCryptoService
		mediator: services,
		// Chain ID
		chainID: chainID,
		// Create a queue for payloads, wrapped in a metrics buffer
		payloads: &metricsBuffer{
			PayloadsBuffer: NewPayloadsBuffer(height),
			sizeMetrics:    stateMetrics.PayloadBufferSize,
			chainID:        chainID,
		},
		ledger:              ledger,
		stateResponseCh:     make(chan protoext.ReceivedMessage, config.StateChannelSize),
		stateRequestCh:      make(chan protoext.ReceivedMessage, config.StateChannelSize),
		stopCh:              make(chan struct{}),
		stateTransferActive: 0,
		once:                sync.Once{},
		stateMetrics:        stateMetrics,
		requestValidator:    &stateRequestValidator{},
		blockingMode:        blockingMode,
		config:              config,
	}

	logger.Infof("Updating metadata information for channel %s, "+
		"current ledger sequence is at = %d, next expected block is = %d", chainID, height-1, s.payloads.Next())
	logger.Debug("Updating gossip ledger height to", height)
	services.UpdateLedgerHeight(height, common2.ChannelID(s.chainID))

	// Listen for incoming communication
	go s.receiveAndQueueGossipMessages(gossipChan)
	go s.receiveAndDispatchDirectMessages(commChan)
	// Deliver in order messages into the incoming channel
	go s.deliverPayloads()
	if s.config.StateEnabled {
		// Execute anti entropy to fill missing gaps
		go s.antiEntropy()
	}
	// Taking care of state request messages
	go s.processStateRequests()

	return s
}

func (s *GossipStateProviderImpl) receiveAndQueueGossipMessages(ch <-chan *proto.GossipMessage) {
	for msg := range ch {
		s.logger.Debug("Received new message via gossip channel")
		go func(msg *proto.GossipMessage) {
			if !bytes.Equal(msg.Channel, []byte(s.chainID)) {
				s.logger.Warning("Received enqueue for channel",
					string(msg.Channel), "while expecting channel", s.chainID, "ignoring enqueue")
				return
			}

			dataMsg := msg.GetDataMsg()
			if dataMsg != nil {
				if err := s.addPayload(dataMsg.GetPayload(), nonBlocking); err != nil {
					s.logger.Warningf("Block [%d] received from gossip wasn't added to payload buffer: %v", dataMsg.Payload.SeqNum, err)
					return
				}
			} else {
				s.logger.Debug("Gossip message received is not of data message type, usually this should not happen.")
			}
		}(msg)
	}
}

func (s *GossipStateProviderImpl) receiveAndDispatchDirectMessages(ch <-chan protoext.ReceivedMessage) {
	for msg := range ch {
		s.logger.Debug("Dispatching a message", msg)
		go func(msg protoext.ReceivedMessage) {
			gm := msg.GetGossipMessage()
			// Check type of the message
			if protoext.IsRemoteStateMessage(gm.GossipMessage) {
				s.logger.Debug("Handling direct state transfer message")
				// Got state transfer request response
				s.directMessage(msg)
			} else if gm.GetPrivateData() != nil {
				s.logger.Debug("Handling private data collection message")
				// Handling private data replication message
				s.privateDataMessage(msg)
			}
		}(msg)
	}
}

func (s *GossipStateProviderImpl) privateDataMessage(msg protoext.ReceivedMessage) {
	if !bytes.Equal(msg.GetGossipMessage().Channel, []byte(s.chainID)) {
		s.logger.Warning("Received state transfer request for channel",
			string(msg.GetGossipMessage().Channel), "while expecting channel", s.chainID, "skipping request...")
		return
	}

	gossipMsg := msg.GetGossipMessage()
	pvtDataMsg := gossipMsg.GetPrivateData()

	if pvtDataMsg.Payload == nil {
		s.logger.Warning("Malformed private data message, no payload provided")
		return
	}

	collectionName := pvtDataMsg.Payload.CollectionName
	txID := pvtDataMsg.Payload.TxId
	pvtRwSet := pvtDataMsg.Payload.PrivateRwset

	if len(pvtRwSet) == 0 {
		s.logger.Warning("Malformed private data message, no rwset provided, collection name = ", collectionName)
		return
	}

	txPvtRwSet := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: pvtDataMsg.Payload.Namespace,
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{{
					CollectionName: collectionName,
					Rwset:          pvtRwSet,
				}},
			},
		},
	}

	txPvtRwSetWithConfig := &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: txPvtRwSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			pvtDataMsg.Payload.Namespace: pvtDataMsg.Payload.CollectionConfigs,
		},
	}

	if err := s.ledger.StorePvtData(txID, txPvtRwSetWithConfig, pvtDataMsg.Payload.PrivateSimHeight); err != nil {
		s.logger.Errorf("Wasn't able to persist private data for collection %s, due to %s", collectionName, err)
		msg.Ack(err) // Sending NACK to indicate failure of storing collection
	}

	msg.Ack(nil)
	s.logger.Debug("Private data for collection", collectionName, "has been stored")
}

func (s *GossipStateProviderImpl) directMessage(msg protoext.ReceivedMessage) {
	s.logger.Debug("[ENTER] -> directMessage")
	defer s.logger.Debug("[EXIT] ->  directMessage")

	if msg == nil {
		s.logger.Error("Got nil message via end-to-end channel, should not happen!")
		return
	}

	if !bytes.Equal(msg.GetGossipMessage().Channel, []byte(s.chainID)) {
		s.logger.Warning("Received state transfer request for channel",
			string(msg.GetGossipMessage().Channel), "while expecting channel", s.chainID, "skipping request...")
		return
	}

	incoming := msg.GetGossipMessage()

	if incoming.GetStateRequest() != nil {
		if len(s.stateRequestCh) < s.config.StateChannelSize {
			// Forward state request to the channel, if there are too
			// many message of state request ignore to avoid flooding.
			s.stateRequestCh <- msg
		}
	} else if incoming.GetStateResponse() != nil {
		// If no state transfer procedure activate there is
		// no reason to process the message
		if atomic.LoadInt32(&s.stateTransferActive) == 1 {
			// Send signal of state response message
			s.stateResponseCh <- msg
		}
	}
}

func (s *GossipStateProviderImpl) processStateRequests() {
	for {
		msg, stillOpen := <-s.stateRequestCh
		if !stillOpen {
			return
		}
		s.handleStateRequest(msg)
	}
}

// handleStateRequest handles state request message, validate batch size, reads current leader state to
// obtain required blocks, builds response message and send it back
func (s *GossipStateProviderImpl) handleStateRequest(msg protoext.ReceivedMessage) {
	if msg == nil {
		return
	}
	request := msg.GetGossipMessage().GetStateRequest()

	if err := s.requestValidator.validate(request, s.config.StateBatchSize); err != nil {
		s.logger.Errorf("State request validation failed, %s. Ignoring request...", err)
		return
	}

	currentHeight, err := s.ledger.LedgerHeight()
	if err != nil {
		s.logger.Errorf("Cannot access to current ledger height, due to %+v", err)
		return
	}
	if currentHeight < request.EndSeqNum {
		s.logger.Warningf("Received state request to transfer blocks with sequence numbers higher  [%d...%d] "+
			"than available in ledger (%d)", request.StartSeqNum, request.StartSeqNum, currentHeight)
	}

	endSeqNum := min(currentHeight, request.EndSeqNum)

	response := &proto.RemoteStateResponse{Payloads: make([]*proto.Payload, 0)}
	for seqNum := request.StartSeqNum; seqNum <= endSeqNum; seqNum++ {
		s.logger.Debug("Reading block ", seqNum, " with private data from the coordinator service")
		connInfo := msg.GetConnectionInfo()
		peerAuthInfo := protoutil.SignedData{
			Data:      connInfo.Auth.SignedData,
			Signature: connInfo.Auth.Signature,
			Identity:  connInfo.Identity,
		}
		block, pvtData, err := s.ledger.GetPvtDataAndBlockByNum(seqNum, peerAuthInfo)
		if err != nil {
			s.logger.Errorf("cannot read block number %d from ledger, because %+v, skipping...", seqNum, err)
			continue
		}

		if block == nil {
			s.logger.Errorf("Wasn't able to read block with sequence number %d from ledger, skipping....", seqNum)
			continue
		}

		blockBytes, err := pb.Marshal(block)
		if err != nil {
			s.logger.Errorf("Could not marshal block: %+v", errors.WithStack(err))
			continue
		}

		var pvtBytes [][]byte
		if pvtData != nil {
			// Marshal private data
			pvtBytes, err = pvtData.Marshal()
			if err != nil {
				s.logger.Errorf("Failed to marshal private rwset for block %d due to %+v", seqNum, errors.WithStack(err))
				continue
			}
		}

		// Appending result to the response
		response.Payloads = append(response.Payloads, &proto.Payload{
			SeqNum:      seqNum,
			Data:        blockBytes,
			PrivateData: pvtBytes,
		})
	}
	// Sending back response with missing blocks
	msg.Respond(&proto.GossipMessage{
		// Copy nonce field from the request, so it will be possible to match response
		Nonce:   msg.GetGossipMessage().Nonce,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Channel: []byte(s.chainID),
		Content: &proto.GossipMessage_StateResponse{StateResponse: response},
	})
}

func (s *GossipStateProviderImpl) handleStateResponse(msg protoext.ReceivedMessage) (uint64, error) {
	max := uint64(0)
	// Send signal that response for given nonce has been received
	response := msg.GetGossipMessage().GetStateResponse()
	// Extract payloads, verify and push into buffer
	if len(response.GetPayloads()) == 0 {
		return uint64(0), errors.New("Received state transfer response without payload")
	}
	for _, payload := range response.GetPayloads() {
		s.logger.Debugf("Received payload with sequence number %d.", payload.SeqNum)
		block, err := protoutil.UnmarshalBlock(payload.Data)
		if err != nil {
			s.logger.Warningf("Error unmarshalling payload to block for sequence number %d, due to %+v", payload.SeqNum, err)
			return uint64(0), err
		}

		if err := s.mediator.VerifyBlock(common2.ChannelID(s.chainID), payload.SeqNum, block); err != nil {
			err = errors.WithStack(err)
			s.logger.Warningf("Error verifying block with sequence number %d, due to %+v", payload.SeqNum, err)
			return uint64(0), err
		}
		if max < payload.SeqNum {
			max = payload.SeqNum
		}

		err = s.addPayload(payload, blocking)
		if err != nil {
			s.logger.Warningf("Block [%d] received from block transfer wasn't added to payload buffer: %v", payload.SeqNum, err)
		}
	}
	return max, nil
}

// Stop function sends halting signal to all go routines
func (s *GossipStateProviderImpl) Stop() {
	// Make sure stop won't be executed twice
	// and stop channel won't be used again
	s.once.Do(func() {
		close(s.stopCh)
		// Close all resources
		s.ledger.Close()
		close(s.stateRequestCh)
		close(s.stateResponseCh)
	})
}

func (s *GossipStateProviderImpl) deliverPayloads() {
	for {
		select {
		// Wait for notification that next seq has arrived
		case <-s.payloads.Ready():
			s.logger.Debugf("[%s] Ready to transfer payloads (blocks) to the ledger, next block number is = [%d]", s.chainID, s.payloads.Next())
			// Collect all subsequent payloads
			for payload := s.payloads.Pop(); payload != nil; payload = s.payloads.Pop() {
				rawBlock := &common.Block{}
				if err := pb.Unmarshal(payload.Data, rawBlock); err != nil {
					s.logger.Errorf("Error getting block with seqNum = %d due to (%+v)...dropping block", payload.SeqNum, errors.WithStack(err))
					continue
				}
				if rawBlock.Data == nil || rawBlock.Header == nil {
					s.logger.Errorf("Block with claimed sequence %d has no header (%v) or data (%v)",
						payload.SeqNum, rawBlock.Header, rawBlock.Data)
					continue
				}
				s.logger.Debugf("[%s] Transferring block [%d] with %d transaction(s) to the ledger", s.chainID, payload.SeqNum, len(rawBlock.Data.Data))

				// Read all private data into slice
				var p util.PvtDataCollections
				if payload.PrivateData != nil {
					err := p.Unmarshal(payload.PrivateData)
					if err != nil {
						s.logger.Errorf("Wasn't able to unmarshal private data for block seqNum = %d due to (%+v)...dropping block", payload.SeqNum, errors.WithStack(err))
						continue
					}
				}
				if err := s.commitBlock(rawBlock, p); err != nil {
					if executionErr, isExecutionErr := err.(*vsccErrors.VSCCExecutionFailureError); isExecutionErr {
						s.logger.Errorf("Failed executing VSCC due to %v. Aborting chain processing", executionErr)
						return
					}
					s.logger.Panicf("Cannot commit block to the ledger due to %+v", errors.WithStack(err))
				}
			}
		case <-s.stopCh:
			s.logger.Debug("State provider has been stopped, finishing to push new blocks.")
			return
		}
	}
}

func (s *GossipStateProviderImpl) antiEntropy() {
	defer s.logger.Debug("State Provider stopped, stopping anti entropy procedure.")

	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(s.config.StateCheckInterval):
			ourHeight, err := s.ledger.LedgerHeight()
			if err != nil {
				// Unable to read from ledger continue to the next round
				s.logger.Errorf("Cannot obtain ledger height, due to %+v", errors.WithStack(err))
				continue
			}
			if ourHeight == 0 {
				s.logger.Error("Ledger reported block height of 0 but this should be impossible")
				continue
			}
			maxHeight := s.maxAvailableLedgerHeight()
			if ourHeight >= maxHeight {
				continue
			}

			s.requestBlocksInRange(uint64(ourHeight), uint64(maxHeight)-1)
		}
	}
}

// maxAvailableLedgerHeight iterates over all available peers and checks advertised meta state to
// find maximum available ledger height across peers
func (s *GossipStateProviderImpl) maxAvailableLedgerHeight() uint64 {
	max := uint64(0)
	for _, p := range s.mediator.PeersOfChannel(common2.ChannelID(s.chainID)) {
		if p.Properties == nil {
			s.logger.Debug("Peer", p.PreferredEndpoint(), "doesn't have properties, skipping it")
			continue
		}
		peerHeight := p.Properties.LedgerHeight
		if max < peerHeight {
			max = peerHeight
		}
	}
	return max
}

// requestBlocksInRange capable to acquire blocks with sequence
// numbers in the range [start...end).
func (s *GossipStateProviderImpl) requestBlocksInRange(start uint64, end uint64) {
	atomic.StoreInt32(&s.stateTransferActive, 1)
	defer atomic.StoreInt32(&s.stateTransferActive, 0)

	for prev := start; prev <= end; {
		next := min(end, prev+s.config.StateBatchSize)

		gossipMsg := s.stateRequestMessage(prev, next)

		responseReceived := false
		tryCounts := 0

		for !responseReceived {
			if tryCounts > s.config.StateMaxRetries {
				s.logger.Warningf("Wasn't  able to get blocks in range [%d...%d), after %d retries",
					prev, next, tryCounts)
				return
			}
			// Select peers to ask for blocks
			peer, err := s.selectPeerToRequestFrom(next)
			if err != nil {
				s.logger.Warningf("Cannot send state request for blocks in range [%d...%d), due to %+v",
					prev, next, errors.WithStack(err))
				return
			}

			s.logger.Debugf("State transfer, with peer %s, requesting blocks in range [%d...%d), "+
				"for chainID %s", peer.Endpoint, prev, next, s.chainID)

			s.mediator.Send(gossipMsg, peer)
			tryCounts++

			// Wait until timeout or response arrival
			select {
			case msg, stillOpen := <-s.stateResponseCh:
				if !stillOpen {
					return
				}
				if msg.GetGossipMessage().Nonce !=
					gossipMsg.Nonce {
					continue
				}
				// Got corresponding response for state request, can continue
				index, err := s.handleStateResponse(msg)
				if err != nil {
					s.logger.Warningf("Wasn't able to process state response for "+
						"blocks [%d...%d], due to %+v", prev, next, errors.WithStack(err))
					continue
				}
				prev = index + 1
				responseReceived = true
			case <-time.After(s.config.StateResponseTimeout):
			}
		}
	}
}

// stateRequestMessage generates state request message for given blocks in range [beginSeq...endSeq]
func (s *GossipStateProviderImpl) stateRequestMessage(beginSeq uint64, endSeq uint64) *proto.GossipMessage {
	return &proto.GossipMessage{
		Nonce:   util.RandomUInt64(),
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Channel: []byte(s.chainID),
		Content: &proto.GossipMessage_StateRequest{
			StateRequest: &proto.RemoteStateRequest{
				StartSeqNum: beginSeq,
				EndSeqNum:   endSeq,
			},
		},
	}
}

// selectPeerToRequestFrom selects peer which has required blocks to ask missing blocks from
func (s *GossipStateProviderImpl) selectPeerToRequestFrom(height uint64) (*comm.RemotePeer, error) {
	// Filter peers which posses required range of missing blocks
	peers := s.filterPeers(s.hasRequiredHeight(height))

	n := len(peers)
	if n == 0 {
		return nil, errors.New("there are no peers to ask for missing blocks from")
	}

	// Select peer to ask for blocks
	return peers[util.RandomInt(n)], nil
}

// filterPeers returns list of peers which aligns the predicate provided
func (s *GossipStateProviderImpl) filterPeers(predicate func(peer discovery.NetworkMember) bool) []*comm.RemotePeer {
	var peers []*comm.RemotePeer

	for _, member := range s.mediator.PeersOfChannel(common2.ChannelID(s.chainID)) {
		if predicate(member) {
			peers = append(peers, &comm.RemotePeer{Endpoint: member.PreferredEndpoint(), PKIID: member.PKIid})
		}
	}

	return peers
}

// hasRequiredHeight returns predicate which is capable to filter peers with ledger height above than indicated
// by provided input parameter
func (s *GossipStateProviderImpl) hasRequiredHeight(height uint64) func(peer discovery.NetworkMember) bool {
	return func(peer discovery.NetworkMember) bool {
		if peer.Properties != nil {
			return peer.Properties.LedgerHeight >= height
		}
		s.logger.Debug(peer.PreferredEndpoint(), "doesn't have properties")
		return false
	}
}

// AddPayload adds new payload into state.
func (s *GossipStateProviderImpl) AddPayload(payload *proto.Payload) error {
	return s.addPayload(payload, s.blockingMode)
}

// addPayload adds new payload into state. It may (or may not) block according to the
// given parameter. If it gets a block while in blocking mode - it would wait until
// the block is sent into the payloads buffer.
// Else - it may drop the block, if the payload buffer is too full.
func (s *GossipStateProviderImpl) addPayload(payload *proto.Payload, blockingMode bool) error {
	if payload == nil {
		return errors.New("Given payload is nil")
	}
	s.logger.Debugf("[%s] Adding payload to local buffer, blockNum = [%d]", s.chainID, payload.SeqNum)
	height, err := s.ledger.LedgerHeight()
	if err != nil {
		return errors.Wrap(err, "Failed obtaining ledger height")
	}

	if !blockingMode && payload.SeqNum-height >= uint64(s.config.StateBlockBufferSize) {
		if s.straggler(height, payload) {
			s.logger.Warningf("[%s] Current block height (%d) is too far behind other peers at height (%d) to be able to receive blocks "+
				"without state transfer which is disabled in the configuration "+
				"(peer.gossip.state.enabled = false). Consider enabling it or setting the peer explicitly to be a leader (peer.gossip.orgLeader = true) "+
				"in order to pull blocks directly from the ordering service.",
				s.chainID, height, payload.SeqNum+1)
		}
		return errors.Errorf("Ledger height is at %d, cannot enqueue block with sequence of %d", height, payload.SeqNum)
	}

	for blockingMode && s.payloads.Size() > s.config.StateBlockBufferSize*2 {
		time.Sleep(enqueueRetryInterval)
	}

	s.payloads.Push(payload)
	s.logger.Debugf("Blocks payloads buffer size for channel [%s] is %d blocks", s.chainID, s.payloads.Size())
	return nil
}

func (s *GossipStateProviderImpl) straggler(currHeight uint64, receivedPayload *proto.Payload) bool {
	// If state transfer is disabled, there is no way to request blocks from peers that their ledger has advanced too far.
	stateDisabled := !s.config.StateEnabled
	// We are too far behind if we received a block with a sequence number more than stragglerWarningThreshold ahead of our height.
	tooFarBehind := currHeight+stragglerWarningThreshold < receivedPayload.SeqNum
	// We depend on other peers for blocks if we use leader election, or we are not explicitly configured to be an org leader.
	peerDependent := s.config.UseLeaderElection || !s.config.OrgLeader
	return stateDisabled && tooFarBehind && peerDependent
}

func (s *GossipStateProviderImpl) commitBlock(block *common.Block, pvtData util.PvtDataCollections) error {
	t1 := time.Now()

	// Commit block with available private transactions
	if err := s.ledger.StoreBlock(block, pvtData); err != nil {
		s.logger.Errorf("Got error while committing(%+v)", errors.WithStack(err))
		return err
	}

	sinceT1 := time.Since(t1)
	s.stateMetrics.CommitDuration.With("channel", s.chainID).Observe(sinceT1.Seconds())

	// Update ledger height
	s.mediator.UpdateLedgerHeight(block.Header.Number+1, common2.ChannelID(s.chainID))
	s.logger.Debugf("[%s] Committed block [%d] with %d transaction(s)",
		s.chainID, block.Header.Number, len(block.Data.Data))

	s.stateMetrics.Height.With("channel", s.chainID).Set(float64(block.Header.Number + 1))

	return nil
}
