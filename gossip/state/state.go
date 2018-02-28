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
	vsccErrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	common2 "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
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
	defAntiEntropyInterval             = 10 * time.Second
	defAntiEntropyStateResponseTimeout = 3 * time.Second
	defAntiEntropyBatchSize            = 10

	defChannelBufferSize     = 100
	defAntiEntropyMaxRetries = 3

	defMaxBlockDistance = 100

	blocking    = true
	nonBlocking = false

	enqueueRetryInterval = time.Millisecond * 100
)

// GossipAdapter defines gossip/communication required interface for state provider
type GossipAdapter interface {
	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common2.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage)

	// UpdateChannelMetadata updates the self metadata the peer
	// publishes to other peers about its channel-related state
	UpdateChannelMetadata(metadata []byte, chainID common2.ChainID)

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common2.ChainID) []discovery.NetworkMember
}

// MCSAdapter adapter of message crypto service interface to bound
// specific APIs required by state transfer service
type MCSAdapter interface {
	// VerifyBlock returns nil if the block is properly signed, and the claimed seqNum is the
	// sequence number that the block's header contains.
	// else returns error
	VerifyBlock(chainID common2.ChainID, seqNum uint64, signedBlock []byte) error

	// VerifyByChannel checks that signature is a valid signature of message
	// under a peer's verification key, but also in the context of a specific channel.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If peerIdentity is nil, then the verification fails.
	VerifyByChannel(chainID common2.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error
}

// ledgerResources defines abilities that the ledger provides
type ledgerResources interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) error

	// StorePvtData used to persist private date into transient store
	StorePvtData(txid string, privData *rwset.TxPvtReadWriteSet) error

	// GetPvtDataAndBlockByNum get block by number and returns also all related private data
	// the order of private data in slice of PvtDataCollections doesn't imply the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64, peerAuthInfo common.SignedData) (*common.Block, util.PvtDataCollections, error)

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
	// Chain id
	chainID string

	mediator *ServicesMediator

	// Channel to read gossip messages from
	gossipChan <-chan *proto.GossipMessage

	commChan <-chan proto.ReceivedMessage

	// Queue of payloads which wasn't acquired yet
	payloads PayloadsBuffer

	ledger ledgerResources

	stateResponseCh chan proto.ReceivedMessage

	stateRequestCh chan proto.ReceivedMessage

	stopCh chan struct{}

	done sync.WaitGroup

	once sync.Once

	stateTransferActive int32
}

var logger = util.GetLogger(util.LoggingStateModule, "")

// NewGossipStateProvider creates state provider with coordinator instance
// to orchestrate arrival of private rwsets and blocks before committing them into the ledger.
func NewGossipStateProvider(chainID string, services *ServicesMediator, ledger ledgerResources) GossipStateProvider {

	gossipChan, _ := services.Accept(func(message interface{}) bool {
		// Get only data messages
		return message.(*proto.GossipMessage).IsDataMsg() &&
			bytes.Equal(message.(*proto.GossipMessage).Channel, []byte(chainID))
	}, false)

	remoteStateMsgFilter := func(message interface{}) bool {
		receivedMsg := message.(proto.ReceivedMessage)
		msg := receivedMsg.GetGossipMessage()
		if !(msg.IsRemoteStateMessage() || msg.GetPrivateData() != nil) {
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
		// MessageCryptoService
		mediator: services,

		// Chain ID
		chainID: chainID,

		// Channel to read new messages from
		gossipChan: gossipChan,

		// Channel to read direct messages from other peers
		commChan: commChan,

		// Create a queue for payload received
		payloads: NewPayloadsBuffer(height),

		ledger: ledger,

		stateResponseCh: make(chan proto.ReceivedMessage, defChannelBufferSize),

		stateRequestCh: make(chan proto.ReceivedMessage, defChannelBufferSize),

		stopCh: make(chan struct{}, 1),

		stateTransferActive: 0,

		once: sync.Once{},
	}

	nodeMetastate := common2.NewNodeMetastate(height - 1)

	logger.Infof("Updating node metadata information, "+
		"current ledger sequence is at = %d, next expected block is = %d", nodeMetastate.LedgerHeight, s.payloads.Next())

	b, err := nodeMetastate.Bytes()
	if err == nil {
		logger.Debug("Updating gossip metadate nodeMetastate", nodeMetastate)
		services.UpdateChannelMetadata(b, common2.ChainID(s.chainID))
	} else {
		logger.Errorf("Unable to serialize node meta nodeMetastate, error = %+v", errors.WithStack(err))
	}

	s.done.Add(4)

	// Listen for incoming communication
	go s.listen()
	// Deliver in order messages into the incoming channel
	go s.deliverPayloads()
	// Execute anti entropy to fill missing gaps
	go s.antiEntropy()
	// Taking care of state request messages
	go s.processStateRequests()

	return s
}

func (s *GossipStateProviderImpl) listen() {
	defer s.done.Done()

	for {
		select {
		case msg := <-s.gossipChan:
			logger.Debug("Received new message via gossip channel")
			go s.queueNewMessage(msg)
		case msg := <-s.commChan:
			logger.Debug("Dispatching a message", msg)
			go s.dispatch(msg)
		case <-s.stopCh:
			s.stopCh <- struct{}{}
			logger.Debug("Stop listening for new messages")
			return
		}
	}
}
func (s *GossipStateProviderImpl) dispatch(msg proto.ReceivedMessage) {
	// Check type of the message
	if msg.GetGossipMessage().IsRemoteStateMessage() {
		logger.Debug("Handling direct state transfer message")
		// Got state transfer request response
		s.directMessage(msg)
	} else if msg.GetGossipMessage().GetPrivateData() != nil {
		logger.Debug("Handling private data collection message")
		// Handling private data replication message
		s.privateDataMessage(msg)
	}

}
func (s *GossipStateProviderImpl) privateDataMessage(msg proto.ReceivedMessage) {
	if !bytes.Equal(msg.GetGossipMessage().Channel, []byte(s.chainID)) {
		logger.Warning("Received state transfer request for channel",
			string(msg.GetGossipMessage().Channel), "while expecting channel", s.chainID, "skipping request...")
		return
	}

	gossipMsg := msg.GetGossipMessage()
	pvtDataMsg := gossipMsg.GetPrivateData()

	collectionName := pvtDataMsg.Payload.CollectionName
	txID := pvtDataMsg.Payload.TxId
	pvtRwSet := pvtDataMsg.Payload.PrivateRwset

	if len(pvtRwSet) == 0 {
		logger.Warning("Malformed private data message, no rwset provided, collection name = ", collectionName)
		return
	}

	txPvtRwSet := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{{
			Namespace: pvtDataMsg.Payload.Namespace,
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{{
				CollectionName: collectionName,
				Rwset:          pvtRwSet,
			}}},
		},
	}

	if err := s.ledger.StorePvtData(txID, txPvtRwSet); err != nil {
		logger.Errorf("Wasn't able to persist private data for collection %s, due to %s", collectionName, err)
		msg.Ack(err) // Sending NACK to indicate failure of storing collection
	}

	msg.Ack(nil)
	logger.Debug("Private data for collection", collectionName, "has been stored")
}

func (s *GossipStateProviderImpl) directMessage(msg proto.ReceivedMessage) {
	logger.Debug("[ENTER] -> directMessage")
	defer logger.Debug("[EXIT] ->  directMessage")

	if msg == nil {
		logger.Error("Got nil message via end-to-end channel, should not happen!")
		return
	}

	if !bytes.Equal(msg.GetGossipMessage().Channel, []byte(s.chainID)) {
		logger.Warning("Received state transfer request for channel",
			string(msg.GetGossipMessage().Channel), "while expecting channel", s.chainID, "skipping request...")
		return
	}

	incoming := msg.GetGossipMessage()

	if incoming.GetStateRequest() != nil {
		if len(s.stateRequestCh) < defChannelBufferSize {
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
	defer s.done.Done()

	for {
		select {
		case msg := <-s.stateRequestCh:
			s.handleStateRequest(msg)
		case <-s.stopCh:
			s.stopCh <- struct{}{}
			return
		}
	}
}

// Handle state request message, validate batch size, read current leader state to
// obtain required blocks, build response message and send it back
func (s *GossipStateProviderImpl) handleStateRequest(msg proto.ReceivedMessage) {
	if msg == nil {
		return
	}
	request := msg.GetGossipMessage().GetStateRequest()

	batchSize := request.EndSeqNum - request.StartSeqNum
	if batchSize > defAntiEntropyBatchSize {
		logger.Errorf("Requesting blocks batchSize size (%d) greater than configured allowed"+
			" (%d) batching for anti-entropy. Ignoring request...", batchSize, defAntiEntropyBatchSize)
		return
	}

	if request.StartSeqNum > request.EndSeqNum {
		logger.Errorf("Invalid sequence interval [%d...%d], ignoring request...", request.StartSeqNum, request.EndSeqNum)
		return
	}

	currentHeight, err := s.ledger.LedgerHeight()
	if err != nil {
		logger.Errorf("Cannot access to current ledger height, due to %+v", errors.WithStack(err))
		return
	}
	if currentHeight < request.EndSeqNum {
		logger.Warningf("Received state request to transfer blocks with sequence numbers higher  [%d...%d] "+
			"than available in ledger (%d)", request.StartSeqNum, request.StartSeqNum, currentHeight)
	}

	endSeqNum := min(currentHeight, request.EndSeqNum)

	response := &proto.RemoteStateResponse{Payloads: make([]*proto.Payload, 0)}
	for seqNum := request.StartSeqNum; seqNum <= endSeqNum; seqNum++ {
		logger.Debug("Reading block ", seqNum, " with private data from the coordinator service")
		connInfo := msg.GetConnectionInfo()
		peerAuthInfo := common.SignedData{
			Data:      connInfo.Auth.SignedData,
			Signature: connInfo.Auth.Signature,
			Identity:  connInfo.Identity,
		}
		block, pvtData, err := s.ledger.GetPvtDataAndBlockByNum(seqNum, peerAuthInfo)

		if err != nil {
			logger.Errorf("Wasn't able to read block with sequence number %d from ledger, "+
				"due to %+v skipping....", seqNum, errors.WithStack(err))
			continue
		}

		if block == nil {
			logger.Errorf("Wasn't able to read block with sequence number %d from ledger, skipping....", seqNum)
			continue
		}

		blockBytes, err := pb.Marshal(block)

		if err != nil {
			logger.Errorf("Could not marshal block: %+v", errors.WithStack(err))
			continue
		}

		var pvtBytes [][]byte
		if pvtData != nil {
			// Marshal private data
			pvtBytes, err = pvtData.Marshal()
			if err != nil {
				logger.Errorf("Failed to marshal private rwset for block %d due to %+v", seqNum, errors.WithStack(err))
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

func (s *GossipStateProviderImpl) handleStateResponse(msg proto.ReceivedMessage) (uint64, error) {
	max := uint64(0)
	// Send signal that response for given nonce has been received
	response := msg.GetGossipMessage().GetStateResponse()
	// Extract payloads, verify and push into buffer
	if len(response.GetPayloads()) == 0 {
		return uint64(0), errors.New("Received state transfer response without payload")
	}
	for _, payload := range response.GetPayloads() {
		logger.Debugf("Received payload with sequence number %d.", payload.SeqNum)
		if err := s.mediator.VerifyBlock(common2.ChainID(s.chainID), payload.SeqNum, payload.Data); err != nil {
			err = errors.WithStack(err)
			logger.Warningf("Error verifying block with sequence number %d, due to %+v", payload.SeqNum, err)
			return uint64(0), err
		}
		if max < payload.SeqNum {
			max = payload.SeqNum
		}

		err := s.addPayload(payload, blocking)
		if err != nil {
			logger.Warningf("Payload with sequence number %d wasn't added to payload buffer: %v", payload.SeqNum, err)
		}
	}
	return max, nil
}

// Stop function send halting signal to all go routines
func (s *GossipStateProviderImpl) Stop() {
	// Make sure stop won't be executed twice
	// and stop channel won't be used again
	s.once.Do(func() {
		s.stopCh <- struct{}{}
		// Make sure all go-routines has finished
		s.done.Wait()
		// Close all resources
		s.ledger.Close()
		close(s.stateRequestCh)
		close(s.stateResponseCh)
		close(s.stopCh)
	})
}

// New message notification/handler
func (s *GossipStateProviderImpl) queueNewMessage(msg *proto.GossipMessage) {
	if !bytes.Equal(msg.Channel, []byte(s.chainID)) {
		logger.Warning("Received enqueue for channel",
			string(msg.Channel), "while expecting channel", s.chainID, "ignoring enqueue")
		return
	}

	dataMsg := msg.GetDataMsg()
	if dataMsg != nil {
		if err := s.addPayload(dataMsg.GetPayload(), nonBlocking); err != nil {
			logger.Warning("Failed adding payload:", err)
			return
		}
		logger.Debugf("Received new payload with sequence number = [%d]", dataMsg.Payload.SeqNum)
	} else {
		logger.Debug("Gossip message received is not of data message type, usually this should not happen.")
	}
}

func (s *GossipStateProviderImpl) deliverPayloads() {
	defer s.done.Done()

	for {
		select {
		// Wait for notification that next seq has arrived
		case <-s.payloads.Ready():
			logger.Debugf("Ready to transfer payloads to the ledger, next sequence number is = [%d]", s.payloads.Next())
			// Collect all subsequent payloads
			for payload := s.payloads.Pop(); payload != nil; payload = s.payloads.Pop() {
				rawBlock := &common.Block{}
				if err := pb.Unmarshal(payload.Data, rawBlock); err != nil {
					logger.Errorf("Error getting block with seqNum = %d due to (%+v)...dropping block", payload.SeqNum, errors.WithStack(err))
					continue
				}
				if rawBlock.Data == nil || rawBlock.Header == nil {
					logger.Errorf("Block with claimed sequence %d has no header (%v) or data (%v)",
						payload.SeqNum, rawBlock.Header, rawBlock.Data)
					continue
				}
				logger.Debug("New block with claimed sequence number ", payload.SeqNum, " transactions num ", len(rawBlock.Data.Data))

				// Read all private data into slice
				var p util.PvtDataCollections
				if payload.PrivateData != nil {
					err := p.Unmarshal(payload.PrivateData)
					if err != nil {
						logger.Errorf("Wasn't able to unmarshal private data for block seqNum = %d due to (%+v)...dropping block", payload.SeqNum, errors.WithStack(err))
						continue
					}
				}
				if err := s.commitBlock(rawBlock, p); err != nil {
					if executionErr, isExecutionErr := err.(*vsccErrors.VSCCExecutionFailureError); isExecutionErr {
						logger.Errorf("Failed executing VSCC due to %v. Aborting chain processing", executionErr)
						return
					}
					logger.Panicf("Cannot commit block to the ledger due to %+v", errors.WithStack(err))
				}
			}
		case <-s.stopCh:
			s.stopCh <- struct{}{}
			logger.Debug("State provider has been stopped, finishing to push new blocks.")
			return
		}
	}
}

func (s *GossipStateProviderImpl) antiEntropy() {
	defer s.done.Done()
	defer logger.Debug("State Provider stopped, stopping anti entropy procedure.")

	for {
		select {
		case <-s.stopCh:
			s.stopCh <- struct{}{}
			return
		case <-time.After(defAntiEntropyInterval):
			current, err := s.ledger.LedgerHeight()
			if err != nil {
				// Unable to read from ledger continue to the next round
				logger.Errorf("Cannot obtain ledger height, due to %+v", errors.WithStack(err))
				continue
			}
			if current == 0 {
				logger.Error("Ledger reported block height of 0 but this should be impossible")
				continue
			}
			max := s.maxAvailableLedgerHeight()

			if current-1 >= max {
				continue
			}

			s.requestBlocksInRange(uint64(current), uint64(max))
		}
	}
}

// Iterate over all available peers and check advertised meta state to
// find maximum available ledger height across peers
func (s *GossipStateProviderImpl) maxAvailableLedgerHeight() uint64 {
	max := uint64(0)
	for _, p := range s.mediator.PeersOfChannel(common2.ChainID(s.chainID)) {
		var peerHeight uint64
		if p.Properties != nil {
			peerHeight = p.Properties.LedgerHeight
		} else if nodeMetastate, err := common2.FromBytes(p.Metadata); err == nil {
			peerHeight = nodeMetastate.LedgerHeight
		}

		if max < peerHeight {
			max = peerHeight
		}
	}
	return max
}

// GetBlocksInRange capable to acquire blocks with sequence
// numbers in the range [start...end].
func (s *GossipStateProviderImpl) requestBlocksInRange(start uint64, end uint64) {
	atomic.StoreInt32(&s.stateTransferActive, 1)
	defer atomic.StoreInt32(&s.stateTransferActive, 0)

	for prev := start; prev <= end; {
		next := min(end, prev+defAntiEntropyBatchSize)

		gossipMsg := s.stateRequestMessage(prev, next)

		responseReceived := false
		tryCounts := 0

		for !responseReceived {
			if tryCounts > defAntiEntropyMaxRetries {
				logger.Warningf("Wasn't  able to get blocks in range [%d...%d], after %d retries",
					prev, next, tryCounts)
				return
			}
			// Select peers to ask for blocks
			peer, err := s.selectPeerToRequestFrom(next)
			if err != nil {
				logger.Warningf("Cannot send state request for blocks in range [%d...%d], due to %+v",
					prev, next, errors.WithStack(err))
				return
			}

			logger.Debugf("State transfer, with peer %s, requesting blocks in range [%d...%d], "+
				"for chainID %s", peer.Endpoint, prev, next, s.chainID)

			s.mediator.Send(gossipMsg, peer)
			tryCounts++

			// Wait until timeout or response arrival
			select {
			case msg := <-s.stateResponseCh:
				if msg.GetGossipMessage().Nonce != gossipMsg.Nonce {
					continue
				}
				// Got corresponding response for state request, can continue
				index, err := s.handleStateResponse(msg)
				if err != nil {
					logger.Warningf("Wasn't able to process state response for "+
						"blocks [%d...%d], due to %+v", prev, next, errors.WithStack(err))
					continue
				}
				prev = index + 1
				responseReceived = true
			case <-time.After(defAntiEntropyStateResponseTimeout):
			case <-s.stopCh:
				s.stopCh <- struct{}{}
				return
			}
		}
	}
}

// Generate state request message for given blocks in range [beginSeq...endSeq]
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

// Select peer which has required blocks to ask missing blocks from
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

// filterPeers return list of peers which aligns the predicate provided
func (s *GossipStateProviderImpl) filterPeers(predicate func(peer discovery.NetworkMember) bool) []*comm.RemotePeer {
	var peers []*comm.RemotePeer

	for _, member := range s.mediator.PeersOfChannel(common2.ChainID(s.chainID)) {
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
		if nodeMetadata, err := common2.FromBytes(peer.Metadata); err != nil {
			logger.Errorf("Unable to de-serialize node meta state, error = %+v", errors.WithStack(err))
		} else if nodeMetadata.LedgerHeight >= height {
			return true
		}

		return false
	}
}

// AddPayload add new payload into state.
func (s *GossipStateProviderImpl) AddPayload(payload *proto.Payload) error {
	blockingMode := blocking
	if viper.GetBool("peer.gossip.nonBlockingCommitMode") {
		blockingMode = false
	}
	return s.addPayload(payload, blockingMode)
}

// addPayload add new payload into state. It may (or may not) block according to the
// given parameter. If it gets a block while in blocking mode - it would wait until
// the block is sent into the payloads buffer.
// Else - it may drop the block, if the payload buffer is too full.
func (s *GossipStateProviderImpl) addPayload(payload *proto.Payload, blockingMode bool) error {
	if payload == nil {
		return errors.New("Given payload is nil")
	}
	logger.Debug("Adding new payload into the buffer, seqNum = ", payload.SeqNum)
	height, err := s.ledger.LedgerHeight()
	if err != nil {
		return errors.Wrap(err, "Failed obtaining ledger height")
	}

	if !blockingMode && payload.SeqNum-height >= defMaxBlockDistance {
		return errors.Errorf("Ledger height is at %d, cannot enqueue block with sequence of %d", height, payload.SeqNum)
	}

	for blockingMode && s.payloads.Size() > defMaxBlockDistance*2 {
		time.Sleep(enqueueRetryInterval)
	}

	s.payloads.Push(payload)
	return nil
}

func (s *GossipStateProviderImpl) commitBlock(block *common.Block, pvtData util.PvtDataCollections) error {

	// Commit block with available private transactions
	if err := s.ledger.StoreBlock(block, pvtData); err != nil {
		logger.Errorf("Got error while committing(%+v)", errors.WithStack(err))
		return err
	}

	// Update ledger level within node metadata
	nodeMetastate := common2.NewNodeMetastate(block.Header.Number)
	// Decode nodeMetastate to byte array
	b, err := nodeMetastate.Bytes()
	if err == nil {
		s.mediator.UpdateChannelMetadata(b, common2.ChainID(s.chainID))
	} else {

		logger.Errorf("Unable to serialize node meta nodeMetastate, error = %+v", errors.WithStack(err))
	}

	logger.Debugf("Channel [%s]: Created block [%d] with %d transaction(s)",
		s.chainID, block.Header.Number, len(block.Data.Data))

	return nil
}

func min(a uint64, b uint64) uint64 {
	return b ^ ((a ^ b) & (-(uint64(a-b) >> 63)))
}
