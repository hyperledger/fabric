/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/configtx/test"
	errors2 "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/mocks/validator"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/privdata"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	gutil "github.com/hyperledger/fabric/gossip/util"
	pcomm "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	transientstore2 "github.com/hyperledger/fabric/protos/transientstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	portStartRange = 5610

	orgID = []byte("ORG1")

	noopPeerIdentityAcceptor = func(identity api.PeerIdentityType) error {
		return nil
	}
)

type peerIdentityAcceptor func(identity api.PeerIdentityType) error

type joinChanMsg struct {
}

func init() {
	gutil.SetupTestLogging()
	factory.InitFactories(nil)
}

// SequenceNumber returns the sequence number of the block that the message
// is derived from
func (*joinChanMsg) SequenceNumber() uint64 {
	return uint64(time.Now().UnixNano())
}

// Members returns the organizations of the channel
func (jcm *joinChanMsg) Members() []api.OrgIdentityType {
	return []api.OrgIdentityType{orgID}
}

// AnchorPeersOf returns the anchor peers of the given organization
func (jcm *joinChanMsg) AnchorPeersOf(org api.OrgIdentityType) []api.AnchorPeer {
	return []api.AnchorPeer{}
}

type orgCryptoService struct {
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity
func (*orgCryptoService) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return orgID
}

// Verify verifies a JoinChannelMessage, returns nil on success,
// and an error on failure
func (*orgCryptoService) Verify(joinChanMsg api.JoinChannelMessage) error {
	return nil
}

type cryptoServiceMock struct {
	acceptor peerIdentityAcceptor
}

func (cryptoServiceMock) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*cryptoServiceMock) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*cryptoServiceMock) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*cryptoServiceMock) Sign(msg []byte) ([]byte, error) {
	clone := make([]byte, len(msg))
	copy(clone, msg)
	return clone, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*cryptoServiceMock) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong signature:%v, %v", signature, message)
	}
	return nil
}

// VerifyByChannel checks that signature is a valid signature of message
// under a peer's verification key, but also in the context of a specific channel.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerIdentity is nil, then the signature is verified against this peer's verification key.
func (cs *cryptoServiceMock) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return cs.acceptor(peerIdentity)
}

func (*cryptoServiceMock) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

func bootPeers(portPrefix int, ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", id+portPrefix))
	}
	return peers
}

// Simple presentation of peer which includes only
// communication module, gossip and state transfer
type peerNode struct {
	port   int
	g      gossip.Gossip
	s      *GossipStateProviderImpl
	cs     *cryptoServiceMock
	commit committer.Committer
}

// Shutting down all modules used
func (node *peerNode) shutdown() {
	node.s.Stop()
	node.g.Stop()
}

type mockTransientStore struct {
}

func (*mockTransientStore) PurgeByHeight(maxBlockNumToRetain uint64) error {
	return nil
}

func (*mockTransientStore) Persist(txid string, blockHeight uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error {
	panic("implement me")
}

func (*mockTransientStore) PersistWithConfig(txid string, blockHeight uint64, privateSimulationResultsWithConfig *transientstore2.TxPvtReadWriteSetWithConfigInfo) error {
	panic("implement me")
}

func (mockTransientStore) GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (transientstore.RWSetScanner, error) {
	panic("implement me")
}

func (*mockTransientStore) PurgeByTxids(txids []string) error {
	panic("implement me")
}

type mockCommitter struct {
	*mock.Mock
	sync.Mutex
}

func (mc *mockCommitter) GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error) {
	args := mc.Called()
	return args.Get(0).(ledger.ConfigHistoryRetriever), args.Error(1)
}

func (mc *mockCommitter) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	args := mc.Called(blockNum, filter)
	return args.Get(0).([]*ledger.TxPvtData), args.Error(1)
}

func (mc *mockCommitter) CommitWithPvtData(blockAndPvtData *ledger.BlockAndPvtData) error {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()
	m.Called(blockAndPvtData.Block)
	return nil
}

func (mc *mockCommitter) GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error) {
	args := mc.Called(seqNum)
	return args.Get(0).(*ledger.BlockAndPvtData), args.Error(1)
}

func (mc *mockCommitter) LedgerHeight() (uint64, error) {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()
	args := m.Called()
	if args.Get(1) == nil {
		return args.Get(0).(uint64), nil
	}
	return args.Get(0).(uint64), args.Get(1).(error)
}

func (mc *mockCommitter) GetBlocks(blockSeqs []uint64) []*pcomm.Block {
	if mc.Called(blockSeqs).Get(0) == nil {
		return nil
	}
	return mc.Called(blockSeqs).Get(0).([]*pcomm.Block)
}

func (*mockCommitter) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	panic("implement me")
}

func (*mockCommitter) CommitPvtData(blockPvtData []*ledger.BlockPvtData) ([]*ledger.PvtdataHashMismatch, error) {
	panic("implement me")
}

func (*mockCommitter) Close() {
}

type ramLedger struct {
	ledger map[uint64]*ledger.BlockAndPvtData
	sync.RWMutex
}

func (mock *ramLedger) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	panic("implement me")
}

func (mock *ramLedger) CommitPvtData(blockPvtData []*ledger.BlockPvtData) ([]*ledger.PvtdataHashMismatch, error) {
	panic("implement me")
}

func (mock *ramLedger) GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error) {
	panic("implement me")
}

func (mock *ramLedger) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	mock.RLock()
	defer mock.RUnlock()

	if block, ok := mock.ledger[blockNum]; !ok {
		return nil, errors.New(fmt.Sprintf("no block with seq = %d found", blockNum))
	} else {
		return block, nil
	}
}

func (mock *ramLedger) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	panic("implement me")
}

func (mock *ramLedger) CommitWithPvtData(blockAndPvtdata *ledger.BlockAndPvtData) error {
	mock.Lock()
	defer mock.Unlock()

	if blockAndPvtdata != nil && blockAndPvtdata.Block != nil {
		mock.ledger[blockAndPvtdata.Block.Header.Number] = blockAndPvtdata
		return nil
	}
	return errors.New("invalid input parameters for block and private data param")
}

func (mock *ramLedger) GetBlockchainInfo() (*pcomm.BlockchainInfo, error) {
	mock.RLock()
	defer mock.RUnlock()

	currentBlock := mock.ledger[uint64(len(mock.ledger)-1)].Block
	return &pcomm.BlockchainInfo{
		Height:            currentBlock.Header.Number + 1,
		CurrentBlockHash:  currentBlock.Header.Hash(),
		PreviousBlockHash: currentBlock.Header.PreviousHash,
	}, nil
}

func (mock *ramLedger) GetBlockByNumber(blockNumber uint64) (*pcomm.Block, error) {
	mock.RLock()
	defer mock.RUnlock()

	if blockAndPvtData, ok := mock.ledger[blockNumber]; !ok {
		return nil, errors.New(fmt.Sprintf("no block with seq = %d found", blockNumber))
	} else {
		return blockAndPvtData.Block, nil
	}
}

func (mock *ramLedger) Close() {

}

// Default configuration to be used for gossip and communication modules
func newGossipConfig(portPrefix, id int, boot ...int) *gossip.Config {
	port := id + portPrefix
	return &gossip.Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(portPrefix, boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       0,
		MaxPropagationBurstLatency: time.Duration(10) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(4) * time.Second,
		PullPeerNum:                5,
		InternalEndpoint:           fmt.Sprintf("localhost:%d", port),
		PublishCertPeriod:          10 * time.Second,
		RequestStateInfoInterval:   4 * time.Second,
		PublishStateInfoInterval:   4 * time.Second,
	}
}

// Create gossip instance
func newGossipInstance(config *gossip.Config, mcs api.MessageCryptoService) gossip.Gossip {
	id := api.PeerIdentityType(config.InternalEndpoint)
	return gossip.NewGossipServiceWithServer(config, &orgCryptoService{}, mcs,
		id, nil)
}

// Create new instance of KVLedger to be used for testing
func newCommitter() committer.Committer {
	cb, _ := test.MakeGenesisBlock("testChain")
	ldgr := &ramLedger{
		ledger: make(map[uint64]*ledger.BlockAndPvtData),
	}
	ldgr.CommitWithPvtData(&ledger.BlockAndPvtData{
		Block: cb,
	})
	return committer.NewLedgerCommitter(ldgr)
}

func newPeerNodeWithGossip(config *gossip.Config, committer committer.Committer, acceptor peerIdentityAcceptor, g gossip.Gossip) *peerNode {
	return newPeerNodeWithGossipWithValidator(config, committer, acceptor, g, &validator.MockValidator{})
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNodeWithGossipWithValidator(config *gossip.Config, committer committer.Committer, acceptor peerIdentityAcceptor, g gossip.Gossip, v txvalidator.Validator) *peerNode {
	cs := &cryptoServiceMock{acceptor: acceptor}
	// Gossip component based on configuration provided and communication module
	if g == nil {
		g = newGossipInstance(config, &cryptoServiceMock{acceptor: noopPeerIdentityAcceptor})
	}

	logger.Debug("Joinning channel", util.GetTestChainID())
	g.JoinChan(&joinChanMsg{}, common.ChainID(util.GetTestChainID()))

	// Initialize pseudo peer simulator, which has only three
	// basic parts

	servicesAdapater := &ServicesMediator{GossipAdapter: g, MCSAdapter: cs}
	coord := privdata.NewCoordinator(privdata.Support{
		Validator:      v,
		TransientStore: &mockTransientStore{},
		Committer:      committer,
	}, pcomm.SignedData{})
	sp := NewGossipStateProvider(util.GetTestChainID(), servicesAdapater, coord)
	if sp == nil {
		return nil
	}

	return &peerNode{
		port:   config.BindPort,
		g:      g,
		s:      sp.(*GossipStateProviderImpl),
		commit: committer,
		cs:     cs,
	}
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNode(config *gossip.Config, committer committer.Committer, acceptor peerIdentityAcceptor) *peerNode {
	return newPeerNodeWithGossip(config, committer, acceptor, nil)
}

func TestNilDirectMsg(t *testing.T) {
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	portPrefix := portStartRange + 50
	p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	p.s.handleStateRequest(nil)
	p.s.directMessage(nil)
	sMsg, _ := p.s.stateRequestMessage(uint64(10), uint64(8)).NoopSign()
	req := &comm.ReceivedMessageImpl{
		SignedGossipMessage: sMsg,
	}
	p.s.directMessage(req)
}

func TestNilAddPayload(t *testing.T) {
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	portPrefix := portStartRange + 100
	p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	err := p.s.AddPayload(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestAddPayloadLedgerUnavailable(t *testing.T) {
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	portPrefix := portStartRange + 150
	p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	// Simulate a problem in the ledger
	failedLedger := mock.Mock{}
	failedLedger.On("LedgerHeight", mock.Anything).Return(uint64(0), errors.New("cannot query ledger"))
	mc.Lock()
	mc.Mock = &failedLedger
	mc.Unlock()

	rawblock := pcomm.NewBlock(uint64(1), []byte{})
	b, _ := pb.Marshal(rawblock)
	err := p.s.AddPayload(&proto.Payload{
		SeqNum: uint64(1),
		Data:   b,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed obtaining ledger height")
	assert.Contains(t, err.Error(), "cannot query ledger")
}

func TestLargeBlockGap(t *testing.T) {
	// Scenario: the peer knows of a peer who has a ledger height much higher
	// than itself (500 blocks higher).
	// The peer needs to ask blocks in a way such that the size of the payload buffer
	// never rises above a certain threshold.
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 200)
	mc.On("CommitWithPvtData", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	msgsFromPeer := make(chan proto.ReceivedMessage)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	membership := []discovery.NetworkMember{
		{
			PKIid:    common.PKIidType("a"),
			Endpoint: "a",
			Properties: &proto.Properties{
				LedgerHeight: 500,
			},
		}}
	g.On("PeersOfChannel", mock.Anything).Return(membership)
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, msgsFromPeer)
	g.On("Send", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
		msg := arguments.Get(0).(*proto.GossipMessage)
		// The peer requested a state request
		req := msg.GetStateRequest()
		// Construct a skeleton for the response
		res := &proto.GossipMessage{
			Nonce:   msg.Nonce,
			Channel: []byte(util.GetTestChainID()),
			Content: &proto.GossipMessage_StateResponse{
				StateResponse: &proto.RemoteStateResponse{},
			},
		}
		// Populate the response with payloads according to what the peer asked
		for seq := req.StartSeqNum; seq <= req.EndSeqNum; seq++ {
			rawblock := pcomm.NewBlock(seq, []byte{})
			b, _ := pb.Marshal(rawblock)
			payload := &proto.Payload{
				SeqNum: seq,
				Data:   b,
			}
			res.GetStateResponse().Payloads = append(res.GetStateResponse().Payloads, payload)
		}
		// Finally, send the response down the channel the peer expects to receive it from
		sMsg, _ := res.NoopSign()
		msgsFromPeer <- &comm.ReceivedMessageImpl{
			SignedGossipMessage: sMsg,
		}
	})
	portPrefix := portStartRange + 200
	p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()

	// Process blocks at a speed of 20 Millisecond for each block.
	// The imaginative peer that responds to state
	// If the payload buffer expands above defMaxBlockDistance*2 + defAntiEntropyBatchSize blocks, fail the test
	blockProcessingTime := 20 * time.Millisecond // 10 seconds for total 500 blocks
	expectedSequence := 1
	for expectedSequence < 500 {
		blockSeq := <-blocksPassedToLedger
		assert.Equal(t, expectedSequence, int(blockSeq))
		// Ensure payload buffer isn't over-populated
		assert.True(t, p.s.payloads.Size() <= defMaxBlockDistance*2+defAntiEntropyBatchSize, "payload buffer size is %d", p.s.payloads.Size())
		expectedSequence++
		time.Sleep(blockProcessingTime)
	}
}

func TestOverPopulation(t *testing.T) {
	// Scenario: Add to the state provider blocks
	// with a gap in between, and ensure that the payload buffer
	// rejects blocks starting if the distance between the ledger height to the latest
	// block it contains is bigger than defMaxBlockDistance.
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 10)
	mc.On("CommitWithPvtData", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	portPrefix := portStartRange + 250
	p := newPeerNode(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor)
	defer p.shutdown()

	// Add some blocks in a sequential manner and make sure it works
	for i := 1; i <= 4; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		assert.NoError(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Add payloads from 10 to defMaxBlockDistance, while we're missing blocks [5,9]
	// Should succeed
	for i := 10; i <= defMaxBlockDistance; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		assert.NoError(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Add payloads from defMaxBlockDistance + 2 to defMaxBlockDistance * 10
	// Should fail.
	for i := defMaxBlockDistance + 1; i <= defMaxBlockDistance*10; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		assert.Error(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Ensure only blocks 1-4 were passed to the ledger
	close(blocksPassedToLedger)
	i := 1
	for seq := range blocksPassedToLedger {
		assert.Equal(t, uint64(i), seq)
		i++
	}
	assert.Equal(t, 5, i)

	// Ensure we don't store too many blocks in memory
	sp := p.s
	assert.True(t, sp.payloads.Size() < defMaxBlockDistance)
}

func TestBlockingEnqueue(t *testing.T) {
	// Scenario: In parallel, get blocks from gossip and from the orderer.
	// The blocks from the orderer we get are X2 times the amount of blocks from gossip.
	// The blocks we get from gossip are random indices, to maximize disruption.
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 10)
	mc.On("CommitWithPvtData", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	portPrefix := portStartRange + 300
	p := newPeerNode(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor)
	defer p.shutdown()

	numBlocksReceived := 500
	receivedBlockCount := 0
	// Get a block from the orderer every 1ms
	go func() {
		for i := 1; i <= numBlocksReceived; i++ {
			rawblock := pcomm.NewBlock(uint64(i), []byte{})
			b, _ := pb.Marshal(rawblock)
			block := &proto.Payload{
				SeqNum: uint64(i),
				Data:   b,
			}
			p.s.AddPayload(block)
			time.Sleep(time.Millisecond)
		}
	}()

	// Get a block from gossip every 1ms too
	go func() {
		rand.Seed(time.Now().UnixNano())
		for i := 1; i <= numBlocksReceived/2; i++ {
			blockSeq := rand.Intn(numBlocksReceived)
			rawblock := pcomm.NewBlock(uint64(blockSeq), []byte{})
			b, _ := pb.Marshal(rawblock)
			block := &proto.Payload{
				SeqNum: uint64(blockSeq),
				Data:   b,
			}
			p.s.addPayload(block, nonBlocking)
			time.Sleep(time.Millisecond)
		}
	}()

	for {
		receivedBlock := <-blocksPassedToLedger
		receivedBlockCount++
		m := &mock.Mock{}
		m.On("LedgerHeight", mock.Anything).Return(receivedBlock, nil)
		m.On("CommitWithPvtData", mock.Anything).Run(func(arg mock.Arguments) {
			blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
		})
		mc.Lock()
		mc.Mock = m
		mc.Unlock()
		assert.Equal(t, receivedBlock, uint64(receivedBlockCount))
		if int(receivedBlockCount) == numBlocksReceived {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func TestHaltChainProcessing(t *testing.T) {
	gossipChannel := func(c chan *proto.GossipMessage) <-chan *proto.GossipMessage {
		return c
	}
	makeBlock := func(seq int) []byte {
		b := &pcomm.Block{
			Header: &pcomm.BlockHeader{
				Number: uint64(seq),
			},
			Data: &pcomm.BlockData{
				Data: [][]byte{},
			},
			Metadata: &pcomm.BlockMetadata{
				Metadata: [][]byte{
					{}, {}, {}, {},
				},
			},
		}
		data, _ := pb.Marshal(b)
		return data
	}
	newBlockMsg := func(i int) *proto.GossipMessage {
		return &proto.GossipMessage{
			Channel: []byte("testchainid"),
			Content: &proto.GossipMessage_DataMsg{
				DataMsg: &proto.DataMessage{
					Payload: &proto.Payload{
						SeqNum: uint64(i),
						Data:   makeBlock(i),
					},
				},
			},
		}
	}

	oldLogger := logger
	defer func() { logger = oldLogger }()
	l, recorder := floggingtest.NewTestLogger(t)
	logger = l

	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("CommitWithPvtData", mock.Anything)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	gossipMsgs := make(chan *proto.GossipMessage)

	g.On("Accept", mock.Anything, false).Return(gossipChannel(gossipMsgs), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})

	v := &validator.MockValidator{}
	v.On("Validate").Return(&errors2.VSCCExecutionFailureError{
		Err: errors.New("foobar"),
	}).Once()
	portPrefix := portStartRange + 350
	newPeerNodeWithGossipWithValidator(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g, v)
	gossipMsgs <- newBlockMsg(1)
	assertLogged(t, recorder, "Got error while committing")
	assertLogged(t, recorder, "Aborting chain processing")
	assertLogged(t, recorder, "foobar")
}

func TestFailures(t *testing.T) {
	t.Parallel()
	portPrefix := portStartRange + 400
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(0), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	assert.Panics(t, func() {
		newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	})
	// Reprogram mock
	mc.Mock = &mock.Mock{}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), errors.New("Failed accessing ledger"))
	assert.Nil(t, newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g))
}

func TestGossipReception(t *testing.T) {
	t.Parallel()
	signalChan := make(chan struct{})
	rawblock := &pcomm.Block{
		Header: &pcomm.BlockHeader{
			Number: uint64(1),
		},
		Data: &pcomm.BlockData{
			Data: [][]byte{},
		},
		Metadata: &pcomm.BlockMetadata{
			Metadata: [][]byte{
				{}, {}, {}, {},
			},
		},
	}
	b, _ := pb.Marshal(rawblock)

	createChan := func(signalChan chan struct{}) <-chan *proto.GossipMessage {
		c := make(chan *proto.GossipMessage)
		gMsg := &proto.GossipMessage{
			Channel: []byte("AAA"),
			Content: &proto.GossipMessage_DataMsg{
				DataMsg: &proto.DataMessage{
					Payload: &proto.Payload{
						SeqNum: 1,
						Data:   b,
					},
				},
			},
		}
		go func(c chan *proto.GossipMessage) {
			// Wait for Accept() to be called
			<-signalChan
			// Simulate a message reception from the gossip component with an invalid channel
			c <- gMsg
			gMsg.Channel = []byte(util.GetTestChainID())
			// Simulate a message reception from the gossip component
			c <- gMsg
		}(c)
		return c
	}

	g := &mocks.GossipMock{}
	rmc := createChan(signalChan)
	g.On("Accept", mock.Anything, false).Return(rmc, nil).Run(func(_ mock.Arguments) {
		signalChan <- struct{}{}
	})
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	mc := &mockCommitter{Mock: &mock.Mock{}}
	receivedChan := make(chan struct{})
	mc.On("CommitWithPvtData", mock.Anything).Run(func(arguments mock.Arguments) {
		block := arguments.Get(0).(*pcomm.Block)
		assert.Equal(t, uint64(1), block.Header.Number)
		receivedChan <- struct{}{}
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	portPrefix := portStartRange + 450
	p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	select {
	case <-receivedChan:
	case <-time.After(time.Second * 15):
		assert.Fail(t, "Didn't commit a block within a timely manner")
	}
}

func TestLedgerHeightFromProperties(t *testing.T) {
	// Scenario: For each test, spawn a peer and supply it
	// with a specific mock of PeersOfChannel from peers that
	// either set both metadata properly, or only the properties, or none, or both.
	// Ensure the logic handles all of the 4 possible cases as needed

	t.Parallel()
	// Returns whether the given networkMember was selected or not
	wasNetworkMemberSelected := func(t *testing.T, networkMember discovery.NetworkMember, wg *sync.WaitGroup) bool {
		var wasGivenNetworkMemberSelected int32
		finChan := make(chan struct{})
		g := &mocks.GossipMock{}
		g.On("Send", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
			defer wg.Done()
			msg := arguments.Get(0).(*proto.GossipMessage)
			assert.NotNil(t, msg.GetStateRequest())
			peer := arguments.Get(1).([]*comm.RemotePeer)[0]
			if bytes.Equal(networkMember.PKIid, peer.PKIID) {
				atomic.StoreInt32(&wasGivenNetworkMemberSelected, 1)
			}
			finChan <- struct{}{}
		})
		g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
		g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))
		defaultPeer := discovery.NetworkMember{
			InternalEndpoint: "b",
			PKIid:            common.PKIidType("b"),
			Properties: &proto.Properties{
				LedgerHeight: 5,
			},
		}
		g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{
			defaultPeer,
			networkMember,
		})
		mc := &mockCommitter{Mock: &mock.Mock{}}
		mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
		portPrefix := portStartRange + 500
		p := newPeerNodeWithGossip(newGossipConfig(portPrefix, 0), mc, noopPeerIdentityAcceptor, g)
		defer p.shutdown()
		select {
		case <-time.After(time.Second * 20):
			t.Fatal("Didn't send a request within a timely manner")
		case <-finChan:
		}
		return atomic.LoadInt32(&wasGivenNetworkMemberSelected) == 1
	}

	peerWithProperties := discovery.NetworkMember{
		PKIid: common.PKIidType("peerWithoutMetadata"),
		Properties: &proto.Properties{
			LedgerHeight: 10,
		},
		InternalEndpoint: "peerWithoutMetadata",
	}

	peerWithoutProperties := discovery.NetworkMember{
		PKIid:            common.PKIidType("peerWithoutProperties"),
		InternalEndpoint: "peerWithoutProperties",
	}

	tests := []struct {
		shouldGivenBeSelected bool
		member                discovery.NetworkMember
	}{
		{member: peerWithProperties, shouldGivenBeSelected: true},
		{member: peerWithoutProperties, shouldGivenBeSelected: false},
	}

	var wg sync.WaitGroup
	wg.Add(len(tests))
	for _, tst := range tests {
		go func(shouldGivenBeSelected bool, member discovery.NetworkMember) {
			assert.Equal(t, shouldGivenBeSelected, wasNetworkMemberSelected(t, member, &wg))
		}(tst.shouldGivenBeSelected, tst.member)
	}
	wg.Wait()
}

func TestAccessControl(t *testing.T) {
	t.Parallel()
	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)

	authorizedPeers := map[string]struct{}{
		"localhost:5610": {},
		"localhost:5615": {},
		"localhost:5618": {},
		"localhost:5621": {},
	}
	portPrefix := portStartRange + 600

	blockPullPolicy := func(identity api.PeerIdentityType) error {
		if _, isAuthorized := authorizedPeers[string(identity)]; isAuthorized {
			return nil
		}
		return errors.New("Not authorized")
	}
	for i := 0; i < bootstrapSetSize; i++ {
		commit := newCommitter()
		bootstrapSet = append(bootstrapSet, newPeerNode(newGossipConfig(portPrefix, i), commit, blockPullPolicy))
	}

	defer func() {
		for _, p := range bootstrapSet {
			p.shutdown()
		}
	}()

	msgCount := 5

	for i := 1; i <= msgCount; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		if b, err := pb.Marshal(rawblock); err == nil {
			payload := &proto.Payload{
				SeqNum: uint64(i),
				Data:   b,
			}
			bootstrapSet[0].s.AddPayload(payload)
		} else {
			t.Fail()
		}
	}

	standardPeerSetSize := 10
	peersSet := make([]*peerNode, 0)

	for i := 0; i < standardPeerSetSize; i++ {
		commit := newCommitter()
		peersSet = append(peersSet, newPeerNode(newGossipConfig(portPrefix, bootstrapSetSize+i, 0, 1, 2, 3, 4), commit, blockPullPolicy))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.PeersOfChannel(common.ChainID(util.GetTestChainID()))) != bootstrapSetSize+standardPeerSetSize-1 {
				logger.Debug("Peer discovery has not finished yet")
				return false
			}
		}
		logger.Debug("All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	logger.Debug("Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		logger.Debug("Trying to see all authorized peers get all blocks, and all non-authorized didn't")
		for _, p := range peersSet {
			height, err := p.commit.LedgerHeight()
			id := fmt.Sprintf("localhost:%d", p.port)
			if _, isAuthorized := authorizedPeers[id]; isAuthorized {
				if height != uint64(msgCount+1) || err != nil {
					return false
				}
			} else {
				if err == nil && height > 1 {
					assert.Fail(t, "Peer", id, "got message but isn't authorized! Height:", height)
				}
			}
		}
		logger.Debug("All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

func TestNewGossipStateProvider_SendingManyMessages(t *testing.T) {
	t.Parallel()
	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)
	portPrefix := portStartRange + 650

	for i := 0; i < bootstrapSetSize; i++ {
		commit := newCommitter()
		bootstrapSet = append(bootstrapSet, newPeerNode(newGossipConfig(portPrefix, i, 0, 1, 2, 3, 4), commit, noopPeerIdentityAcceptor))
	}

	defer func() {
		for _, p := range bootstrapSet {
			p.shutdown()
		}
	}()

	msgCount := 10

	for i := 1; i <= msgCount; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		if b, err := pb.Marshal(rawblock); err == nil {
			payload := &proto.Payload{
				SeqNum: uint64(i),
				Data:   b,
			}
			bootstrapSet[0].s.AddPayload(payload)
		} else {
			t.Fail()
		}
	}

	standartPeersSize := 10
	peersSet := make([]*peerNode, 0)

	for i := 0; i < standartPeersSize; i++ {
		commit := newCommitter()
		peersSet = append(peersSet, newPeerNode(newGossipConfig(portPrefix, bootstrapSetSize+i, 0, 1, 2, 3, 4), commit, noopPeerIdentityAcceptor))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.PeersOfChannel(common.ChainID(util.GetTestChainID()))) != bootstrapSetSize+standartPeersSize-1 {
				logger.Debug("Peer discovery has not finished yet")
				return false
			}
		}
		logger.Debug("All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	logger.Debug("Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		logger.Debug("Trying to see all peers get all blocks")
		for _, p := range peersSet {
			height, err := p.commit.LedgerHeight()
			if height != uint64(msgCount+1) || err != nil {
				return false
			}
		}
		logger.Debug("All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

func TestGossipStateProvider_TestStateMessages(t *testing.T) {
	t.Parallel()
	portPrefix := portStartRange + 700
	bootPeer := newPeerNode(newGossipConfig(portPrefix, 0), newCommitter(), noopPeerIdentityAcceptor)
	defer bootPeer.shutdown()

	peer := newPeerNode(newGossipConfig(portPrefix, 1, 0), newCommitter(), noopPeerIdentityAcceptor)
	defer peer.shutdown()

	naiveStateMsgPredicate := func(message interface{}) bool {
		return message.(proto.ReceivedMessage).GetGossipMessage().IsRemoteStateMessage()
	}

	_, bootCh := bootPeer.g.Accept(naiveStateMsgPredicate, true)
	_, peerCh := peer.g.Accept(naiveStateMsgPredicate, true)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		msg := <-bootCh
		logger.Info("Bootstrap node got message, ", msg)
		assert.True(t, msg.GetGossipMessage().GetStateRequest() != nil)
		msg.Respond(&proto.GossipMessage{
			Content: &proto.GossipMessage_StateResponse{StateResponse: &proto.RemoteStateResponse{Payloads: nil}},
		})
		wg.Done()
	}()

	go func() {
		msg := <-peerCh
		logger.Info("Peer node got an answer, ", msg)
		assert.True(t, msg.GetGossipMessage().GetStateResponse() != nil)
		wg.Done()

	}()

	readyCh := make(chan struct{})
	go func() {
		wg.Wait()
		readyCh <- struct{}{}
	}()

	time.Sleep(time.Duration(5) * time.Second)
	logger.Info("Sending gossip message with remote state request")

	chainID := common.ChainID(util.GetTestChainID())

	peer.g.Send(&proto.GossipMessage{
		Content: &proto.GossipMessage_StateRequest{StateRequest: &proto.RemoteStateRequest{StartSeqNum: 0, EndSeqNum: 1}},
	}, &comm.RemotePeer{Endpoint: peer.g.PeersOfChannel(chainID)[0].Endpoint, PKIID: peer.g.PeersOfChannel(chainID)[0].PKIid})
	logger.Info("Waiting until peers exchange messages")

	select {
	case <-readyCh:
		{
			logger.Info("Done!!!")

		}
	case <-time.After(time.Duration(10) * time.Second):
		{
			t.Fail()
		}
	}
}

// Start one bootstrap peer and submit defAntiEntropyBatchSize + 5 messages into
// local ledger, next spawning a new peer waiting for anti-entropy procedure to
// complete missing blocks. Since state transfer messages now batched, it is expected
// to see _exactly_ two messages with state transfer response.
func TestNewGossipStateProvider_BatchingOfStateRequest(t *testing.T) {
	t.Parallel()
	portPrefix := portStartRange + 750
	bootPeer := newPeerNode(newGossipConfig(portPrefix, 0), newCommitter(), noopPeerIdentityAcceptor)
	defer bootPeer.shutdown()

	msgCount := defAntiEntropyBatchSize + 5
	expectedMessagesCnt := 2

	for i := 1; i <= msgCount; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		if b, err := pb.Marshal(rawblock); err == nil {
			payload := &proto.Payload{
				SeqNum: uint64(i),
				Data:   b,
			}
			bootPeer.s.AddPayload(payload)
		} else {
			t.Fail()
		}
	}

	peer := newPeerNode(newGossipConfig(portPrefix, 1, 0), newCommitter(), noopPeerIdentityAcceptor)
	defer peer.shutdown()

	naiveStateMsgPredicate := func(message interface{}) bool {
		return message.(proto.ReceivedMessage).GetGossipMessage().IsRemoteStateMessage()
	}
	_, peerCh := peer.g.Accept(naiveStateMsgPredicate, true)

	messageCh := make(chan struct{})
	stopWaiting := make(chan struct{})

	// Number of submitted messages is defAntiEntropyBatchSize + 5, therefore
	// expected number of batches is expectedMessagesCnt = 2. Following go routine
	// makes sure it receives expected amount of messages and sends signal of success
	// to continue the test
	go func(expected int) {
		cnt := 0
		for cnt < expected {
			select {
			case <-peerCh:
				{
					cnt++
				}

			case <-stopWaiting:
				{
					return
				}
			}
		}

		messageCh <- struct{}{}
	}(expectedMessagesCnt)

	// Waits for message which indicates that expected number of message batches received
	// otherwise timeouts after 2 * defAntiEntropyInterval + 1 seconds
	select {
	case <-messageCh:
		{
			// Once we got message which indicate of two batches being received,
			// making sure messages indeed committed.
			waitUntilTrueOrTimeout(t, func() bool {
				if len(peer.g.PeersOfChannel(common.ChainID(util.GetTestChainID()))) != 1 {
					logger.Debug("Peer discovery has not finished yet")
					return false
				}
				logger.Debug("All peer discovered each other!!!")
				return true
			}, 30*time.Second)

			logger.Debug("Waiting for all blocks to arrive.")
			waitUntilTrueOrTimeout(t, func() bool {
				logger.Debug("Trying to see all peers get all blocks")
				height, err := peer.commit.LedgerHeight()
				if height != uint64(msgCount+1) || err != nil {
					return false
				}
				logger.Debug("All peers have same ledger height!!!")
				return true
			}, 60*time.Second)
		}
	case <-time.After(defAntiEntropyInterval*2 + time.Second*1):
		{
			close(stopWaiting)
			t.Fatal("Expected to receive two batches with missing payloads")
		}
	}
}

// coordinatorMock mocking structure to capture mock interface for
// coord to simulate coord flow during the test
type coordinatorMock struct {
	committer.Committer
	mock.Mock
}

func (mock *coordinatorMock) GetPvtDataAndBlockByNum(seqNum uint64, _ pcomm.SignedData) (*pcomm.Block, gutil.PvtDataCollections, error) {
	args := mock.Called(seqNum)
	return args.Get(0).(*pcomm.Block), args.Get(1).(gutil.PvtDataCollections), args.Error(2)
}

func (mock *coordinatorMock) GetBlockByNum(seqNum uint64) (*pcomm.Block, error) {
	args := mock.Called(seqNum)
	return args.Get(0).(*pcomm.Block), args.Error(1)
}

func (mock *coordinatorMock) StoreBlock(block *pcomm.Block, data gutil.PvtDataCollections) error {
	args := mock.Called(block, data)
	return args.Error(1)
}

func (mock *coordinatorMock) LedgerHeight() (uint64, error) {
	args := mock.Called()
	return args.Get(0).(uint64), args.Error(1)
}

func (mock *coordinatorMock) Close() {
	mock.Called()
}

// StorePvtData used to persist private date into transient store
func (mock *coordinatorMock) StorePvtData(txid string, privData *transientstore2.TxPvtReadWriteSetWithConfigInfo, blkHeight uint64) error {
	return mock.Called().Error(0)
}

type receivedMessageMock struct {
	mock.Mock
}

// Ack returns to the sender an acknowledgement for the message
func (mock *receivedMessageMock) Ack(err error) {

}

func (mock *receivedMessageMock) Respond(msg *proto.GossipMessage) {
	mock.Called(msg)
}

func (mock *receivedMessageMock) GetGossipMessage() *proto.SignedGossipMessage {
	args := mock.Called()
	return args.Get(0).(*proto.SignedGossipMessage)
}

func (mock *receivedMessageMock) GetSourceEnvelope() *proto.Envelope {
	args := mock.Called()
	return args.Get(0).(*proto.Envelope)
}

func (mock *receivedMessageMock) GetConnectionInfo() *proto.ConnectionInfo {
	args := mock.Called()
	return args.Get(0).(*proto.ConnectionInfo)
}

type testData struct {
	block   *pcomm.Block
	pvtData gutil.PvtDataCollections
}

func TestTransferOfPrivateRWSet(t *testing.T) {
	t.Parallel()
	chainID := "testChainID"

	// First gossip instance
	g := &mocks.GossipMock{}
	coord1 := new(coordinatorMock)

	gossipChannel := make(chan *proto.GossipMessage)
	commChannel := make(chan proto.ReceivedMessage)

	gossipChannelFactory := func(ch chan *proto.GossipMessage) <-chan *proto.GossipMessage {
		return ch
	}

	g.On("Accept", mock.Anything, false).Return(gossipChannelFactory(gossipChannel), nil)
	g.On("Accept", mock.Anything, true).Return(nil, commChannel)

	g.On("UpdateChannelMetadata", mock.Anything, mock.Anything)
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	g.On("Close")

	coord1.On("LedgerHeight", mock.Anything).Return(uint64(5), nil)

	var data = map[uint64]*testData{
		uint64(2): {
			block: &pcomm.Block{
				Header: &pcomm.BlockHeader{
					Number:       2,
					DataHash:     []byte{0, 1, 1, 1},
					PreviousHash: []byte{0, 0, 0, 1},
				},
				Data: &pcomm.BlockData{
					Data: [][]byte{{1}, {2}, {3}},
				},
			},
			pvtData: gutil.PvtDataCollections{
				{
					SeqInBlock: uint64(0),
					WriteSet: &rwset.TxPvtReadWriteSet{
						DataModel: rwset.TxReadWriteSet_KV,
						NsPvtRwset: []*rwset.NsPvtReadWriteSet{
							{
								Namespace: "myCC:v1",
								CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
									{
										CollectionName: "mysecrectCollection",
										Rwset:          []byte{1, 2, 3, 4, 5},
									},
								},
							},
						},
					},
				},
			},
		},

		uint64(3): {
			block: &pcomm.Block{
				Header: &pcomm.BlockHeader{
					Number:       3,
					DataHash:     []byte{1, 1, 1, 1},
					PreviousHash: []byte{0, 1, 1, 1},
				},
				Data: &pcomm.BlockData{
					Data: [][]byte{{4}, {5}, {6}},
				},
			},
			pvtData: gutil.PvtDataCollections{
				{
					SeqInBlock: uint64(2),
					WriteSet: &rwset.TxPvtReadWriteSet{
						DataModel: rwset.TxReadWriteSet_KV,
						NsPvtRwset: []*rwset.NsPvtReadWriteSet{
							{
								Namespace: "otherCC:v1",
								CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
									{
										CollectionName: "topClassified",
										Rwset:          []byte{0, 0, 0, 4, 2},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for seqNum, each := range data {
		coord1.On("GetPvtDataAndBlockByNum", seqNum).Return(each.block, each.pvtData, nil /* no error*/)
	}

	coord1.On("Close")

	servicesAdapater := &ServicesMediator{GossipAdapter: g, MCSAdapter: &cryptoServiceMock{acceptor: noopPeerIdentityAcceptor}}
	st := NewGossipStateProvider(chainID, servicesAdapater, coord1)
	defer st.Stop()

	// Mocked state request message
	requestMsg := new(receivedMessageMock)

	// Get state request message, blocks [2...3]
	requestGossipMsg := &proto.GossipMessage{
		// Copy nonce field from the request, so it will be possible to match response
		Nonce:   1,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Channel: []byte(chainID),
		Content: &proto.GossipMessage_StateRequest{StateRequest: &proto.RemoteStateRequest{
			StartSeqNum: 2,
			EndSeqNum:   3,
		}},
	}

	msg, _ := requestGossipMsg.NoopSign()

	requestMsg.On("GetGossipMessage").Return(msg)
	requestMsg.On("GetConnectionInfo").Return(&proto.ConnectionInfo{
		Auth: &proto.AuthInfo{},
	})

	// Channel to send responses back
	responseChannel := make(chan proto.ReceivedMessage)
	defer close(responseChannel)

	requestMsg.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		// Get gossip response to respond back on state request
		response := args.Get(0).(*proto.GossipMessage)
		// Wrap it up into received response
		receivedMsg := new(receivedMessageMock)
		// Create sign response
		msg, _ := response.NoopSign()
		// Mock to respond
		receivedMsg.On("GetGossipMessage").Return(msg)
		// Send response
		responseChannel <- receivedMsg
	})

	// Send request message via communication channel into state transfer
	commChannel <- requestMsg

	// State transfer request should result in state response back
	response := <-responseChannel

	// Start the assertion section
	stateResponse := response.GetGossipMessage().GetStateResponse()

	assertion := assert.New(t)
	// Nonce should be equal to Nonce of the request
	assertion.Equal(response.GetGossipMessage().Nonce, uint64(1))
	// Payload should not need be nil
	assertion.NotNil(stateResponse)
	assertion.NotNil(stateResponse.Payloads)
	// Exactly two messages expected
	assertion.Equal(len(stateResponse.Payloads), 2)

	// Assert we have all data and it's same as we expected it
	for _, each := range stateResponse.Payloads {
		block := &pcomm.Block{}
		err := pb.Unmarshal(each.Data, block)
		assertion.NoError(err)

		assertion.NotNil(block.Header)

		testBlock, ok := data[block.Header.Number]
		assertion.True(ok)

		for i, d := range testBlock.block.Data.Data {
			assertion.True(bytes.Equal(d, block.Data.Data[i]))
		}

		for i, p := range testBlock.pvtData {
			pvtDataPayload := &proto.PvtDataPayload{}
			err := pb.Unmarshal(each.PrivateData[i], pvtDataPayload)
			assertion.NoError(err)
			pvtRWSet := &rwset.TxPvtReadWriteSet{}
			err = pb.Unmarshal(pvtDataPayload.Payload, pvtRWSet)
			assertion.NoError(err)
			assertion.True(pb.Equal(p.WriteSet, pvtRWSet))
		}
	}
}

type testPeer struct {
	*mocks.GossipMock
	id            string
	gossipChannel chan *proto.GossipMessage
	commChannel   chan proto.ReceivedMessage
	coord         *coordinatorMock
}

func (t testPeer) Gossip() <-chan *proto.GossipMessage {
	return t.gossipChannel
}

func (t testPeer) Comm() chan proto.ReceivedMessage {
	return t.commChannel
}

var peers = map[string]testPeer{
	"peer1": {
		id:            "peer1",
		gossipChannel: make(chan *proto.GossipMessage),
		commChannel:   make(chan proto.ReceivedMessage),
		GossipMock:    &mocks.GossipMock{},
		coord:         new(coordinatorMock),
	},
	"peer2": {
		id:            "peer2",
		gossipChannel: make(chan *proto.GossipMessage),
		commChannel:   make(chan proto.ReceivedMessage),
		GossipMock:    &mocks.GossipMock{},
		coord:         new(coordinatorMock),
	},
}

func TestTransferOfPvtDataBetweenPeers(t *testing.T) {
	/*
	   This test covers pretty basic scenario, there are two peers: "peer1" and "peer2",
	   while peer2 missing a few blocks in the ledger therefore asking to replicate those
	   blocks from the first peers.

	   Test going to check that block from one peer will be replicated into second one and
	   have identical content.
	*/
	t.Parallel()
	chainID := "testChainID"

	// Initialize peer
	for _, peer := range peers {
		peer.On("Accept", mock.Anything, false).Return(peer.Gossip(), nil)

		peer.On("Accept", mock.Anything, true).
			Return(nil, peer.Comm()).
			Once().
			On("Accept", mock.Anything, true).
			Return(nil, make(chan proto.ReceivedMessage))

		peer.On("UpdateChannelMetadata", mock.Anything, mock.Anything)
		peer.coord.On("Close")
		peer.On("Close")
	}

	// First peer going to have more advanced ledger
	peers["peer1"].coord.On("LedgerHeight", mock.Anything).Return(uint64(3), nil)

	// Second peer has a gap of one block, hence it will have to replicate it from previous
	peers["peer2"].coord.On("LedgerHeight", mock.Anything).Return(uint64(2), nil)

	peers["peer1"].coord.On("GetPvtDataAndBlockByNum", uint64(2)).Return(&pcomm.Block{
		Header: &pcomm.BlockHeader{
			Number:       2,
			DataHash:     []byte{0, 0, 0, 1},
			PreviousHash: []byte{0, 1, 1, 1},
		},
		Data: &pcomm.BlockData{
			Data: [][]byte{{4}, {5}, {6}},
		},
	}, gutil.PvtDataCollections{&ledger.TxPvtData{
		SeqInBlock: uint64(1),
		WriteSet: &rwset.TxPvtReadWriteSet{
			DataModel: rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace: "myCC:v1",
					CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
						{
							CollectionName: "mysecrectCollection",
							Rwset:          []byte{1, 2, 3, 4, 5},
						},
					},
				},
			},
		},
	}}, nil)

	// Return membership of the peers
	member2 := discovery.NetworkMember{
		PKIid:            common.PKIidType([]byte{2}),
		Endpoint:         "peer2:7051",
		InternalEndpoint: "peer2:7051",
		Properties: &proto.Properties{
			LedgerHeight: 2,
		},
	}

	member1 := discovery.NetworkMember{
		PKIid:            common.PKIidType([]byte{1}),
		Endpoint:         "peer1:7051",
		InternalEndpoint: "peer1:7051",
		Properties: &proto.Properties{
			LedgerHeight: 3,
		},
	}

	peers["peer1"].On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{member2})
	peers["peer2"].On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{member1})

	peers["peer2"].On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		request := args.Get(0).(*proto.GossipMessage)
		requestMsg := new(receivedMessageMock)
		msg, _ := request.NoopSign()
		requestMsg.On("GetGossipMessage").Return(msg)
		requestMsg.On("GetConnectionInfo").Return(&proto.ConnectionInfo{
			Auth: &proto.AuthInfo{},
		})

		requestMsg.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
			response := args.Get(0).(*proto.GossipMessage)
			receivedMsg := new(receivedMessageMock)
			msg, _ := response.NoopSign()
			receivedMsg.On("GetGossipMessage").Return(msg)
			// Send response back to the peer
			peers["peer2"].commChannel <- receivedMsg
		})

		peers["peer1"].commChannel <- requestMsg
	})

	wg := sync.WaitGroup{}
	wg.Add(1)
	peers["peer2"].coord.On("StoreBlock", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		wg.Done() // Done once second peer hits commit of the block
	}).Return([]string{}, nil) // No pvt data to complete and no error

	cryptoService := &cryptoServiceMock{acceptor: noopPeerIdentityAcceptor}

	mediator := &ServicesMediator{GossipAdapter: peers["peer1"], MCSAdapter: cryptoService}
	peer1State := NewGossipStateProvider(chainID, mediator, peers["peer1"].coord)
	defer peer1State.Stop()

	mediator = &ServicesMediator{GossipAdapter: peers["peer2"], MCSAdapter: cryptoService}
	peer2State := NewGossipStateProvider(chainID, mediator, peers["peer2"].coord)
	defer peer2State.Stop()

	// Make sure state was replicated
	done := make(chan struct{})
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		break
	case <-time.After(30 * time.Second):
		t.Fail()
	}
}

func waitUntilTrueOrTimeout(t *testing.T, predicate func() bool, timeout time.Duration) {
	ch := make(chan struct{})
	go func() {
		logger.Debug("Started to spin off, until predicate will be satisfied.")
		for !predicate() {
			time.Sleep(1 * time.Second)
		}
		ch <- struct{}{}
		logger.Debug("Done.")
	}()

	select {
	case <-ch:
		break
	case <-time.After(timeout):
		t.Fatal("Timeout has expired")
		break
	}
	logger.Debug("Stop waiting until timeout or true")
}

func assertLogged(t *testing.T, r *floggingtest.Recorder, msg string) {
	observed := func() bool { return len(r.MessagesContaining(msg)) > 0 }
	waitUntilTrueOrTimeout(t, observed, 30*time.Second)
}
