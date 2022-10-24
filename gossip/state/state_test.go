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
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	pcomm "github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	tspb "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/configtx/test"
	errors2 "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
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
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/privdata"
	capabilitymock "github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	gossiputil "github.com/hyperledger/fabric/gossip/util"
	corecomm "github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	orgID = []byte("ORG1")

	noopPeerIdentityAcceptor = func(identity api.PeerIdentityType) error {
		return nil
	}
)

type peerIdentityAcceptor func(identity api.PeerIdentityType) error

type joinChanMsg struct{}

func init() {
	gossiputil.SetupTestLogging()
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

type orgCryptoService struct{}

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
func (*cryptoServiceMock) VerifyBlock(channelID common.ChannelID, seqNum uint64, signedBlock *pcomm.Block) error {
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
func (cs *cryptoServiceMock) VerifyByChannel(channelID common.ChannelID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return cs.acceptor(peerIdentity)
}

func (*cryptoServiceMock) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

func bootPeersWithPorts(ports ...int) []string {
	var peers []string
	for _, port := range ports {
		peers = append(peers, fmt.Sprintf("127.0.0.1:%d", port))
	}
	return peers
}

type peerNodeGossipSupport interface {
	GossipAdapter
	Stop()
	JoinChan(joinMsg api.JoinChannelMessage, channelID common.ChannelID)
}

// Simple presentation of peer which includes only
// communication module, gossip and state transfer
type peerNode struct {
	port   int
	g      peerNodeGossipSupport
	s      *GossipStateProviderImpl
	cs     *cryptoServiceMock
	commit committer.Committer
	grpc   *corecomm.GRPCServer
}

// Shutting down all modules used
func (node *peerNode) shutdown() {
	node.s.Stop()
	node.g.Stop()
	node.grpc.Stop()
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

func (mc *mockCommitter) CommitLegacy(blockAndPvtData *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()
	m.Called(blockAndPvtData.Block)
	return nil
}

func (mc *mockCommitter) GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error) {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()

	args := m.Called(seqNum)
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

func (mc *mockCommitter) DoesPvtDataInfoExistInLedger(blkNum uint64) (bool, error) {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()
	args := m.Called(blkNum)
	return args.Get(0).(bool), args.Error(1)
}

func (mc *mockCommitter) GetBlocks(blockSeqs []uint64) []*pcomm.Block {
	mc.Lock()
	m := mc.Mock
	mc.Unlock()

	if m.Called(blockSeqs).Get(0) == nil {
		return nil
	}
	return m.Called(blockSeqs).Get(0).([]*pcomm.Block)
}

func (*mockCommitter) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	panic("implement me")
}

func (*mockCommitter) CommitPvtDataOfOldBlocks(
	reconciledPvtdata []*ledger.ReconciledPvtdata,
	unreconciled ledger.MissingPvtDataInfo,
) ([]*ledger.PvtdataHashMismatch, error) {
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

func (mock *ramLedger) CommitPvtDataOfOldBlocks(
	reconciledPvtdata []*ledger.ReconciledPvtdata,
	unreconciled ledger.MissingPvtDataInfo,
) ([]*ledger.PvtdataHashMismatch, error) {
	panic("implement me")
}

func (mock *ramLedger) GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error) {
	panic("implement me")
}

func (mock *ramLedger) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	mock.RLock()
	defer mock.RUnlock()

	if block, ok := mock.ledger[blockNum]; !ok {
		return nil, fmt.Errorf("no block with seq = %d found", blockNum)
	} else {
		return block, nil
	}
}

func (mock *ramLedger) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	panic("implement me")
}

func (mock *ramLedger) CommitLegacy(blockAndPvtdata *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error {
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
		CurrentBlockHash:  protoutil.BlockHeaderHash(currentBlock.Header),
		PreviousBlockHash: currentBlock.Header.PreviousHash,
	}, nil
}

func (mock *ramLedger) DoesPvtDataInfoExist(blkNum uint64) (bool, error) {
	return false, nil
}

func (mock *ramLedger) GetBlockByNumber(blockNumber uint64) (*pcomm.Block, error) {
	mock.RLock()
	defer mock.RUnlock()

	if blockAndPvtData, ok := mock.ledger[blockNumber]; !ok {
		return nil, fmt.Errorf("no block with seq = %d found", blockNumber)
	} else {
		return blockAndPvtData.Block, nil
	}
}

func (mock *ramLedger) Close() {
}

// Create new instance of KVLedger to be used for testing
func newCommitter() committer.Committer {
	cb, _ := test.MakeGenesisBlock("testChain")
	ldgr := &ramLedger{
		ledger: make(map[uint64]*ledger.BlockAndPvtData),
	}
	ldgr.CommitLegacy(&ledger.BlockAndPvtData{Block: cb}, &ledger.CommitOptions{})
	return committer.NewLedgerCommitter(ldgr)
}

func newPeerNodeWithGossip(id int, committer committer.Committer,
	acceptor peerIdentityAcceptor, g peerNodeGossipSupport, bootPorts ...int) *peerNode {
	logger := flogging.MustGetLogger(gossiputil.StateLogger)
	return newPeerNodeWithGossipWithValidator(logger, id, committer, acceptor, g, &validator.MockValidator{}, bootPorts...)
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNodeWithGossipWithValidatorWithMetrics(logger gossiputil.Logger, id int, committer committer.Committer,
	acceptor peerIdentityAcceptor, g peerNodeGossipSupport, v txvalidator.Validator,
	gossipMetrics *metrics.GossipMetrics, bootPorts ...int) (node *peerNode, port int) {
	cs := &cryptoServiceMock{acceptor: acceptor}
	port, gRPCServer, certs, secureDialOpts, _ := gossiputil.CreateGRPCLayer()

	if g == nil {
		config := &gossip.Config{
			BindPort:                     port,
			BootstrapPeers:               bootPeersWithPorts(bootPorts...),
			ID:                           fmt.Sprintf("p%d", id),
			MaxBlockCountToStore:         0,
			MaxPropagationBurstLatency:   time.Duration(10) * time.Millisecond,
			MaxPropagationBurstSize:      10,
			PropagateIterations:          1,
			PropagatePeerNum:             3,
			PullInterval:                 time.Duration(4) * time.Second,
			PullPeerNum:                  5,
			InternalEndpoint:             fmt.Sprintf("127.0.0.1:%d", port),
			PublishCertPeriod:            10 * time.Second,
			RequestStateInfoInterval:     4 * time.Second,
			PublishStateInfoInterval:     4 * time.Second,
			TimeForMembershipTracker:     5 * time.Second,
			TLSCerts:                     certs,
			DigestWaitTime:               algo.DefDigestWaitTime,
			RequestWaitTime:              algo.DefRequestWaitTime,
			ResponseWaitTime:             algo.DefResponseWaitTime,
			DialTimeout:                  comm.DefDialTimeout,
			ConnTimeout:                  comm.DefConnTimeout,
			RecvBuffSize:                 comm.DefRecvBuffSize,
			SendBuffSize:                 comm.DefSendBuffSize,
			MsgExpirationTimeout:         channel.DefMsgExpirationTimeout,
			AliveTimeInterval:            discovery.DefAliveTimeInterval,
			AliveExpirationTimeout:       discovery.DefAliveExpirationTimeout,
			AliveExpirationCheckInterval: discovery.DefAliveExpirationCheckInterval,
			ReconnectInterval:            discovery.DefReconnectInterval,
			MaxConnectionAttempts:        discovery.DefMaxConnectionAttempts,
			MsgExpirationFactor:          discovery.DefMsgExpirationFactor,
		}

		selfID := api.PeerIdentityType(config.InternalEndpoint)
		mcs := &cryptoServiceMock{acceptor: noopPeerIdentityAcceptor}
		g = gossip.New(config, gRPCServer.Server(), &orgCryptoService{}, mcs, selfID, secureDialOpts, gossipMetrics, nil)
	}

	g.JoinChan(&joinChanMsg{}, common.ChannelID("testchannelid"))

	go func() {
		gRPCServer.Start()
	}()

	// Initialize pseudo peer simulator, which has only three
	// basic parts

	servicesAdapater := &ServicesMediator{GossipAdapter: g, MCSAdapter: cs}
	coordConfig := privdata.CoordinatorConfig{
		PullRetryThreshold:             0,
		TransientBlockRetention:        1000,
		SkipPullingInvalidTransactions: false,
	}

	mspID := "Org1MSP"
	capabilityProvider := &capabilitymock.CapabilityProvider{}
	appCapability := &capabilitymock.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coord := privdata.NewCoordinator(mspID, privdata.Support{
		Validator:          v,
		Committer:          committer,
		CapabilityProvider: capabilityProvider,
	}, &transientstore.Store{}, protoutil.SignedData{}, gossipMetrics.PrivdataMetrics, coordConfig, nil)
	stateConfig := &StateConfig{
		StateCheckInterval:   DefStateCheckInterval,
		StateResponseTimeout: DefStateResponseTimeout,
		StateBatchSize:       DefStateBatchSize,
		StateMaxRetries:      DefStateMaxRetries,
		StateBlockBufferSize: DefStateBlockBufferSize,
		StateChannelSize:     DefStateChannelSize,
		StateEnabled:         true,
	}
	sp := NewGossipStateProvider(logger, "testchannelid", servicesAdapater, coord, gossipMetrics.StateMetrics, blocking, stateConfig)
	if sp == nil {
		gRPCServer.Stop()
		return nil, port
	}

	return &peerNode{
		port:   port,
		g:      g,
		s:      sp.(*GossipStateProviderImpl),
		commit: committer,
		cs:     cs,
		grpc:   gRPCServer,
	}, port
}

// add metrics provider for metrics testing
func newPeerNodeWithGossipWithMetrics(id int, committer committer.Committer,
	acceptor peerIdentityAcceptor, g peerNodeGossipSupport, gossipMetrics *metrics.GossipMetrics) *peerNode {
	logger := flogging.MustGetLogger(gossiputil.StateLogger)
	node, _ := newPeerNodeWithGossipWithValidatorWithMetrics(logger, id, committer, acceptor, g,
		&validator.MockValidator{}, gossipMetrics)
	return node
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNodeWithGossipWithValidator(logger gossiputil.Logger, id int, committer committer.Committer,
	acceptor peerIdentityAcceptor, g peerNodeGossipSupport, v txvalidator.Validator, bootPorts ...int) *peerNode {
	gossipMetrics := metrics.NewGossipMetrics(&disabled.Provider{})
	node, _ := newPeerNodeWithGossipWithValidatorWithMetrics(logger, id, committer, acceptor, g, v, gossipMetrics, bootPorts...)
	return node
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNode(id int, committer committer.Committer, acceptor peerIdentityAcceptor, bootPorts ...int) *peerNode {
	return newPeerNodeWithGossip(id, committer, acceptor, nil, bootPorts...)
}

// Constructing pseudo boot node, simulating only gossip and state transfer part, return port
func newBootNode(id int, committer committer.Committer, acceptor peerIdentityAcceptor) (node *peerNode, port int) {
	v := &validator.MockValidator{}
	gossipMetrics := metrics.NewGossipMetrics(&disabled.Provider{})
	logger := flogging.MustGetLogger(gossiputil.StateLogger)
	return newPeerNodeWithGossipWithValidatorWithMetrics(logger, id, committer, acceptor, nil, v, gossipMetrics)
}

func TestStraggler(t *testing.T) {
	for _, testCase := range []struct {
		stateEnabled   bool
		orgLeader      bool
		leaderElection bool
		height         uint64
		receivedSeq    uint64
		expected       bool
	}{
		{
			height:         100,
			receivedSeq:    300,
			leaderElection: true,
			expected:       true,
		},
		{
			height:      100,
			receivedSeq: 300,
			expected:    true,
		},
		{
			height:      100,
			receivedSeq: 300,
			orgLeader:   true,
		},
		{
			height:         100,
			receivedSeq:    105,
			leaderElection: true,
		},
		{
			height:         100,
			receivedSeq:    300,
			leaderElection: true,
			stateEnabled:   true,
		},
	} {
		description := fmt.Sprintf("%+v", testCase)
		t.Run(description, func(t *testing.T) {
			s := &GossipStateProviderImpl{
				config: &StateConfig{
					StateEnabled:      testCase.stateEnabled,
					OrgLeader:         testCase.orgLeader,
					UseLeaderElection: testCase.leaderElection,
				},
			}

			s.straggler(testCase.height, &proto.Payload{
				SeqNum: testCase.receivedSeq,
			})
		})
	}
}

func TestNilDirectMsg(t *testing.T) {
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	p.s.handleStateRequest(nil)
	p.s.directMessage(nil)
	sMsg, _ := protoext.NoopSign(p.s.stateRequestMessage(uint64(10), uint64(8)))
	req := &comm.ReceivedMessageImpl{
		SignedGossipMessage: sMsg,
	}
	p.s.directMessage(req)
}

func TestNilAddPayload(t *testing.T) {
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	err := p.s.AddPayload(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestAddPayloadLedgerUnavailable(t *testing.T) {
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	// Simulate a problem in the ledger
	failedLedger := mock.Mock{}
	failedLedger.On("LedgerHeight", mock.Anything).Return(uint64(0), errors.New("cannot query ledger"))
	mc.Lock()
	mc.Mock = &failedLedger
	mc.Unlock()

	rawblock := protoutil.NewBlock(uint64(1), []byte{})
	b, _ := pb.Marshal(rawblock)
	err := p.s.AddPayload(&proto.Payload{
		SeqNum: uint64(1),
		Data:   b,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed obtaining ledger height")
	require.Contains(t, err.Error(), "cannot query ledger")
}

func TestLargeBlockGap(t *testing.T) {
	// Scenario: the peer knows of a peer who has a ledger height much higher
	// than itself (500 blocks higher).
	// The peer needs to ask blocks in a way such that the size of the payload buffer
	// never rises above a certain threshold.
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 200)
	mc.On("CommitLegacy", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	msgsFromPeer := make(chan protoext.ReceivedMessage)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	mc.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	g := &mocks.GossipMock{}
	membership := []discovery.NetworkMember{
		{
			PKIid:    common.PKIidType("a"),
			Endpoint: "a",
			Properties: &proto.Properties{
				LedgerHeight: 500,
			},
		},
	}
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
			Channel: []byte("testchannelid"),
			Content: &proto.GossipMessage_StateResponse{
				StateResponse: &proto.RemoteStateResponse{},
			},
		}
		// Populate the response with payloads according to what the peer asked
		for seq := req.StartSeqNum; seq <= req.EndSeqNum; seq++ {
			rawblock := protoutil.NewBlock(seq, []byte{})
			b, _ := pb.Marshal(rawblock)
			payload := &proto.Payload{
				SeqNum: seq,
				Data:   b,
			}
			res.GetStateResponse().Payloads = append(res.GetStateResponse().Payloads, payload)
		}
		// Finally, send the response down the channel the peer expects to receive it from
		sMsg, _ := protoext.NoopSign(res)
		msgsFromPeer <- &comm.ReceivedMessageImpl{
			SignedGossipMessage: sMsg,
		}
	})
	p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()

	// Process blocks at a speed of 20 Millisecond for each block.
	// The imaginative peer that responds to state
	// If the payload buffer expands above defMaxBlockDistance*2 + defAntiEntropyBatchSize blocks, fail the test
	blockProcessingTime := 20 * time.Millisecond // 10 seconds for total 500 blocks
	expectedSequence := 1
	for expectedSequence < 500 {
		blockSeq := <-blocksPassedToLedger
		require.Equal(t, expectedSequence, int(blockSeq))
		// Ensure payload buffer isn't over-populated
		require.True(t, p.s.payloads.Size() <= defMaxBlockDistance*2+defAntiEntropyBatchSize, "payload buffer size is %d", p.s.payloads.Size())
		expectedSequence++
		time.Sleep(blockProcessingTime)
	}
}

func TestOverPopulation(t *testing.T) {
	// Scenario: Add to the state provider blocks
	// with a gap in between, and ensure that the payload buffer
	// rejects blocks starting if the distance between the ledger height to the latest
	// block it contains is bigger than defMaxBlockDistance.
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 10)
	mc.On("CommitLegacy", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	mc.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	p := newPeerNode(0, mc, noopPeerIdentityAcceptor)
	defer p.shutdown()

	// Add some blocks in a sequential manner and make sure it works
	for i := 1; i <= 4; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		require.NoError(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Add payloads from 10 to defMaxBlockDistance, while we're missing blocks [5,9]
	// Should succeed
	for i := 10; i <= defMaxBlockDistance; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		require.NoError(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Add payloads from defMaxBlockDistance + 2 to defMaxBlockDistance * 10
	// Should fail.
	for i := defMaxBlockDistance + 1; i <= defMaxBlockDistance*10; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
		b, _ := pb.Marshal(rawblock)
		require.Error(t, p.s.addPayload(&proto.Payload{
			SeqNum: uint64(i),
			Data:   b,
		}, nonBlocking))
	}

	// Ensure only blocks 1-4 were passed to the ledger
	close(blocksPassedToLedger)
	i := 1
	for seq := range blocksPassedToLedger {
		require.Equal(t, uint64(i), seq)
		i++
	}
	require.Equal(t, 5, i)

	// Ensure we don't store too many blocks in memory
	sp := p.s
	require.True(t, sp.payloads.Size() < defMaxBlockDistance)
}

func TestBlockingEnqueue(t *testing.T) {
	// Scenario: In parallel, get blocks from gossip and from the orderer.
	// The blocks from the orderer we get are X2 times the amount of blocks from gossip.
	// The blocks we get from gossip are random indices, to maximize disruption.
	mc := &mockCommitter{Mock: &mock.Mock{}}
	blocksPassedToLedger := make(chan uint64, 10)
	mc.On("CommitLegacy", mock.Anything).Run(func(arg mock.Arguments) {
		blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	mc.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	p := newPeerNode(0, mc, noopPeerIdentityAcceptor)
	defer p.shutdown()

	numBlocksReceived := 500
	receivedBlockCount := 0
	// Get a block from the orderer every 1ms
	go func() {
		for i := 1; i <= numBlocksReceived; i++ {
			rawblock := protoutil.NewBlock(uint64(i), []byte{})
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
			rawblock := protoutil.NewBlock(uint64(blockSeq), []byte{})
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
		m.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
		m.On("CommitLegacy", mock.Anything).Run(func(arg mock.Arguments) {
			blocksPassedToLedger <- arg.Get(0).(*pcomm.Block).Header.Number
		})
		mc.Lock()
		mc.Mock = m
		mc.Unlock()
		require.Equal(t, receivedBlock, uint64(receivedBlockCount))
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
			Channel: []byte("testchannelid"),
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

	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("CommitLegacy", mock.Anything)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	g := &mocks.GossipMock{}
	gossipMsgs := make(chan *proto.GossipMessage)

	g.On("Accept", mock.Anything, false).Return(gossipChannel(gossipMsgs), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})

	v := &validator.MockValidator{}
	v.On("Validate").Return(&errors2.VSCCExecutionFailureError{
		Err: errors.New("foobar"),
	}).Once()

	buf := gbytes.NewBuffer()

	logger := flogging.MustGetLogger(gossiputil.StateLogger).WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		buf.Write([]byte(entry.Message))
		buf.Write([]byte("\n"))
		return nil
	}))
	peerNode := newPeerNodeWithGossipWithValidator(logger, 0, mc, noopPeerIdentityAcceptor, g, v)
	defer peerNode.shutdown()
	gossipMsgs <- newBlockMsg(1)

	gom := gomega.NewGomegaWithT(t)
	gom.Eventually(buf, time.Minute).Should(gbytes.Say("Failed executing VSCC due to foobar"))
	gom.Eventually(buf, time.Minute).Should(gbytes.Say("Aborting chain processing"))
}

func TestFailures(t *testing.T) {
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(0), nil)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	require.Panics(t, func() {
		newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	})
	// Reprogram mock
	mc.Mock = &mock.Mock{}
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), errors.New("Failed accessing ledger"))
	require.Nil(t, newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g))
}

func TestGossipReception(t *testing.T) {
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

	newMsg := func(channel string) *proto.GossipMessage {
		{
			return &proto.GossipMessage{
				Channel: []byte(channel),
				Content: &proto.GossipMessage_DataMsg{
					DataMsg: &proto.DataMessage{
						Payload: &proto.Payload{
							SeqNum: 1,
							Data:   b,
						},
					},
				},
			}
		}
	}

	createChan := func(signalChan chan struct{}) <-chan *proto.GossipMessage {
		c := make(chan *proto.GossipMessage)

		go func(c chan *proto.GossipMessage) {
			// Wait for Accept() to be called
			<-signalChan
			// Simulate a message reception from the gossip component with an invalid channel
			c <- newMsg("AAA")
			// Simulate a message reception from the gossip component
			c <- newMsg("testchannelid")
		}(c)
		return c
	}

	g := &mocks.GossipMock{}
	rmc := createChan(signalChan)
	g.On("Accept", mock.Anything, false).Return(rmc, nil).Run(func(_ mock.Arguments) {
		signalChan <- struct{}{}
	})
	g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	mc := &mockCommitter{Mock: &mock.Mock{}}
	receivedChan := make(chan struct{})
	mc.On("CommitLegacy", mock.Anything).Run(func(arguments mock.Arguments) {
		block := arguments.Get(0).(*pcomm.Block)
		require.Equal(t, uint64(1), block.Header.Number)
		receivedChan <- struct{}{}
	})
	mc.On("LedgerHeight", mock.Anything).Return(uint64(1), nil)
	mc.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
	defer p.shutdown()
	select {
	case <-receivedChan:
	case <-time.After(time.Second * 15):
		require.Fail(t, "Didn't commit a block within a timely manner")
	}
}

func TestLedgerHeightFromProperties(t *testing.T) {
	// Scenario: For each test, spawn a peer and supply it
	// with a specific mock of PeersOfChannel from peers that
	// either set both metadata properly, or only the properties, or none, or both.
	// Ensure the logic handles all of the 4 possible cases as needed

	// Returns whether the given networkMember was selected or not
	wasNetworkMemberSelected := func(t *testing.T, networkMember discovery.NetworkMember) bool {
		var wasGivenNetworkMemberSelected int32
		finChan := make(chan struct{})
		g := &mocks.GossipMock{}
		g.On("Send", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
			msg := arguments.Get(0).(*proto.GossipMessage)
			require.NotNil(t, msg.GetStateRequest())
			peer := arguments.Get(1).([]*comm.RemotePeer)[0]
			if bytes.Equal(networkMember.PKIid, peer.PKIID) {
				atomic.StoreInt32(&wasGivenNetworkMemberSelected, 1)
			}
			finChan <- struct{}{}
		})
		g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
		g.On("Accept", mock.Anything, true).Return(nil, make(chan protoext.ReceivedMessage))
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
		p := newPeerNodeWithGossip(0, mc, noopPeerIdentityAcceptor, g)
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

	for _, tst := range tests {
		require.Equal(t, tst.shouldGivenBeSelected, wasNetworkMemberSelected(t, tst.member))
	}
}

func TestAccessControl(t *testing.T) {
	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)

	authorizedPeersSize := 4
	var listeners []net.Listener
	var endpoints []string

	for i := 0; i < authorizedPeersSize; i++ {
		ll, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners = append(listeners, ll)
		endpoint := ll.Addr().String()
		endpoints = append(endpoints, endpoint)
	}

	defer func() {
		for _, ll := range listeners {
			ll.Close()
		}
	}()

	authorizedPeers := map[string]struct{}{
		endpoints[0]: {},
		endpoints[1]: {},
		endpoints[2]: {},
		endpoints[3]: {},
	}

	blockPullPolicy := func(identity api.PeerIdentityType) error {
		if _, isAuthorized := authorizedPeers[string(identity)]; isAuthorized {
			return nil
		}
		return errors.New("Not authorized")
	}

	var bootPorts []int

	for i := 0; i < bootstrapSetSize; i++ {
		commit := newCommitter()
		bootPeer, bootPort := newBootNode(i, commit, blockPullPolicy)
		bootstrapSet = append(bootstrapSet, bootPeer)
		bootPorts = append(bootPorts, bootPort)
	}

	defer func() {
		for _, p := range bootstrapSet {
			p.shutdown()
		}
	}()

	msgCount := 5

	for i := 1; i <= msgCount; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
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
		peersSet = append(peersSet, newPeerNode(bootstrapSetSize+i, commit, blockPullPolicy, bootPorts...))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.PeersOfChannel(common.ChannelID("testchannelid"))) != bootstrapSetSize+standardPeerSetSize-1 {
				t.Log("Peer discovery has not finished yet")
				return false
			}
		}
		t.Log("All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	t.Log("Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		t.Log("Trying to see all authorized peers get all blocks, and all non-authorized didn't")
		for _, p := range peersSet {
			height, err := p.commit.LedgerHeight()
			id := fmt.Sprintf("127.0.0.1:%d", p.port)
			if _, isAuthorized := authorizedPeers[id]; isAuthorized {
				if height != uint64(msgCount+1) || err != nil {
					return false
				}
			} else {
				if err == nil && height > 1 {
					require.Fail(t, "Peer", id, "got message but isn't authorized! Height:", height)
				}
			}
		}
		t.Log("All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

func TestNewGossipStateProvider_SendingManyMessages(t *testing.T) {
	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)

	var bootPorts []int

	for i := 0; i < bootstrapSetSize; i++ {
		commit := newCommitter()
		bootPeer, bootPort := newBootNode(i, commit, noopPeerIdentityAcceptor)
		bootstrapSet = append(bootstrapSet, bootPeer)
		bootPorts = append(bootPorts, bootPort)
	}

	defer func() {
		for _, p := range bootstrapSet {
			p.shutdown()
		}
	}()

	msgCount := 10

	for i := 1; i <= msgCount; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
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
		peersSet = append(peersSet, newPeerNode(bootstrapSetSize+i, commit, noopPeerIdentityAcceptor, bootPorts...))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.PeersOfChannel(common.ChannelID("testchannelid"))) != bootstrapSetSize+standartPeersSize-1 {
				t.Log("Peer discovery has not finished yet")
				return false
			}
		}
		t.Log("All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	t.Log("Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		t.Log("Trying to see all peers get all blocks")
		for _, p := range peersSet {
			height, err := p.commit.LedgerHeight()
			if height != uint64(msgCount+1) || err != nil {
				return false
			}
		}
		t.Log("All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

// Start one bootstrap peer and submit defAntiEntropyBatchSize + 5 messages into
// local ledger, next spawning a new peer waiting for anti-entropy procedure to
// complete missing blocks. Since state transfer messages now batched, it is expected
// to see _exactly_ two messages with state transfer response.
func TestNewGossipStateProvider_BatchingOfStateRequest(t *testing.T) {
	bootPeer, bootPort := newBootNode(0, newCommitter(), noopPeerIdentityAcceptor)
	defer bootPeer.shutdown()

	msgCount := defAntiEntropyBatchSize + 5
	expectedMessagesCnt := 2

	for i := 1; i <= msgCount; i++ {
		rawblock := protoutil.NewBlock(uint64(i), []byte{})
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

	peer := newPeerNode(1, newCommitter(), noopPeerIdentityAcceptor, bootPort)
	defer peer.shutdown()

	naiveStateMsgPredicate := func(message interface{}) bool {
		return protoext.IsRemoteStateMessage(message.(protoext.ReceivedMessage).GetGossipMessage().GossipMessage)
	}
	_, peerCh := peer.g.Accept(naiveStateMsgPredicate, true)

	wg := sync.WaitGroup{}
	wg.Add(expectedMessagesCnt)

	// Number of submitted messages is defAntiEntropyBatchSize + 5, therefore
	// expected number of batches is expectedMessagesCnt = 2. Following go routine
	// makes sure it receives expected amount of messages and sends signal of success
	// to continue the test
	go func() {
		for count := 0; count < expectedMessagesCnt; count++ {
			<-peerCh
			wg.Done()
		}
	}()

	// Once we got message which indicate of two batches being received,
	// making sure messages indeed committed.
	waitUntilTrueOrTimeout(t, func() bool {
		if len(peer.g.PeersOfChannel(common.ChannelID("testchannelid"))) != 1 {
			t.Log("Peer discovery has not finished yet")
			return false
		}
		t.Log("All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	// Waits for message which indicates that expected number of message batches received
	// otherwise timeouts after 2 * defAntiEntropyInterval + 1 seconds
	wg.Wait()

	t.Log("Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		t.Log("Trying to see all peers get all blocks")
		height, err := peer.commit.LedgerHeight()
		if height != uint64(msgCount+1) || err != nil {
			return false
		}
		t.Log("All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

// coordinatorMock mocking structure to capture mock interface for
// coord to simulate coord flow during the test
type coordinatorMock struct {
	committer.Committer
	mock.Mock
}

func (mock *coordinatorMock) GetPvtDataAndBlockByNum(seqNum uint64, _ protoutil.SignedData) (*pcomm.Block, gossiputil.PvtDataCollections, error) {
	args := mock.Called(seqNum)
	return args.Get(0).(*pcomm.Block), args.Get(1).(gossiputil.PvtDataCollections), args.Error(2)
}

func (mock *coordinatorMock) GetBlockByNum(seqNum uint64) (*pcomm.Block, error) {
	args := mock.Called(seqNum)
	return args.Get(0).(*pcomm.Block), args.Error(1)
}

func (mock *coordinatorMock) StoreBlock(block *pcomm.Block, data gossiputil.PvtDataCollections) error {
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

// StorePvtData used to persist private data into transient store
func (mock *coordinatorMock) StorePvtData(txid string, privData *tspb.TxPvtReadWriteSetWithConfigInfo, blkHeight uint64) error {
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

func (mock *receivedMessageMock) GetGossipMessage() *protoext.SignedGossipMessage {
	args := mock.Called()
	return args.Get(0).(*protoext.SignedGossipMessage)
}

func (mock *receivedMessageMock) GetSourceEnvelope() *proto.Envelope {
	args := mock.Called()
	return args.Get(0).(*proto.Envelope)
}

func (mock *receivedMessageMock) GetConnectionInfo() *protoext.ConnectionInfo {
	args := mock.Called()
	return args.Get(0).(*protoext.ConnectionInfo)
}

type testData struct {
	block   *pcomm.Block
	pvtData gossiputil.PvtDataCollections
}

func TestTransferOfPrivateRWSet(t *testing.T) {
	chainID := "testChainID"

	// First gossip instance
	g := &mocks.GossipMock{}
	coord1 := new(coordinatorMock)

	gossipChannel := make(chan *proto.GossipMessage)
	commChannel := make(chan protoext.ReceivedMessage)

	gossipChannelFactory := func(ch chan *proto.GossipMessage) <-chan *proto.GossipMessage {
		return ch
	}

	g.On("Accept", mock.Anything, false).Return(gossipChannelFactory(gossipChannel), nil)
	g.On("Accept", mock.Anything, true).Return(nil, commChannel)

	g.On("UpdateChannelMetadata", mock.Anything, mock.Anything)
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	g.On("Close")

	coord1.On("LedgerHeight", mock.Anything).Return(uint64(5), nil)

	data := map[uint64]*testData{
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
			pvtData: gossiputil.PvtDataCollections{
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
			pvtData: gossiputil.PvtDataCollections{
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
	stateMetrics := metrics.NewGossipMetrics(&disabled.Provider{}).StateMetrics
	stateConfig := &StateConfig{
		StateCheckInterval:   DefStateCheckInterval,
		StateResponseTimeout: DefStateResponseTimeout,
		StateBatchSize:       DefStateBatchSize,
		StateMaxRetries:      DefStateMaxRetries,
		StateBlockBufferSize: DefStateBlockBufferSize,
		StateChannelSize:     DefStateChannelSize,
		StateEnabled:         true,
	}
	logger := flogging.MustGetLogger(gossiputil.StateLogger)
	st := NewGossipStateProvider(logger, chainID, servicesAdapater, coord1, stateMetrics, blocking, stateConfig)
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

	msg, _ := protoext.NoopSign(requestGossipMsg)

	requestMsg.On("GetGossipMessage").Return(msg)
	requestMsg.On("GetConnectionInfo").Return(&protoext.ConnectionInfo{
		Auth: &protoext.AuthInfo{},
	})

	// Channel to send responses back
	responseChannel := make(chan protoext.ReceivedMessage)
	defer close(responseChannel)

	requestMsg.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		// Get gossip response to respond back on state request
		response := args.Get(0).(*proto.GossipMessage)
		// Wrap it up into received response
		receivedMsg := new(receivedMessageMock)
		// Create sign response
		msg, _ := protoext.NoopSign(response)
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

	assertion := require.New(t)
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
	commChannel   chan protoext.ReceivedMessage
	coord         *coordinatorMock
}

func (t testPeer) Gossip() <-chan *proto.GossipMessage {
	return t.gossipChannel
}

func (t testPeer) Comm() chan protoext.ReceivedMessage {
	return t.commChannel
}

var peers = map[string]testPeer{
	"peer1": {
		id:            "peer1",
		gossipChannel: make(chan *proto.GossipMessage),
		commChannel:   make(chan protoext.ReceivedMessage),
		GossipMock:    &mocks.GossipMock{},
		coord:         new(coordinatorMock),
	},
	"peer2": {
		id:            "peer2",
		gossipChannel: make(chan *proto.GossipMessage),
		commChannel:   make(chan protoext.ReceivedMessage),
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
	chainID := "testChainID"

	// Initialize peer
	for _, peer := range peers {
		peer.On("Accept", mock.Anything, false).Return(peer.Gossip(), nil)

		peer.On("Accept", mock.Anything, true).
			Return(nil, peer.Comm()).
			Once().
			On("Accept", mock.Anything, true).
			Return(nil, make(chan protoext.ReceivedMessage))

		peer.On("UpdateChannelMetadata", mock.Anything, mock.Anything)
		peer.coord.On("Close")
		peer.On("Close")
	}

	// First peer going to have more advanced ledger
	peers["peer1"].coord.On("LedgerHeight", mock.Anything).Return(uint64(3), nil)

	// Second peer has a gap of one block, hence it will have to replicate it from previous
	peers["peer2"].coord.On("LedgerHeight", mock.Anything).Return(uint64(2), nil)

	peers["peer1"].coord.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	peers["peer2"].coord.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	peers["peer1"].coord.On("GetPvtDataAndBlockByNum", uint64(2)).Return(&pcomm.Block{
		Header: &pcomm.BlockHeader{
			Number:       2,
			DataHash:     []byte{0, 0, 0, 1},
			PreviousHash: []byte{0, 1, 1, 1},
		},
		Data: &pcomm.BlockData{
			Data: [][]byte{{4}, {5}, {6}},
		},
	}, gossiputil.PvtDataCollections{&ledger.TxPvtData{
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
		msg, _ := protoext.NoopSign(request)
		requestMsg.On("GetGossipMessage").Return(msg)
		requestMsg.On("GetConnectionInfo").Return(&protoext.ConnectionInfo{
			Auth: &protoext.AuthInfo{},
		})

		requestMsg.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
			response := args.Get(0).(*proto.GossipMessage)
			receivedMsg := new(receivedMessageMock)
			msg, _ := protoext.NoopSign(response)
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

	stateMetrics := metrics.NewGossipMetrics(&disabled.Provider{}).StateMetrics

	mediator := &ServicesMediator{GossipAdapter: peers["peer1"], MCSAdapter: cryptoService}
	stateConfig := &StateConfig{
		StateCheckInterval:   DefStateCheckInterval,
		StateResponseTimeout: DefStateResponseTimeout,
		StateBatchSize:       DefStateBatchSize,
		StateMaxRetries:      DefStateMaxRetries,
		StateBlockBufferSize: DefStateBlockBufferSize,
		StateChannelSize:     DefStateChannelSize,
		StateEnabled:         true,
	}
	logger := flogging.MustGetLogger(gossiputil.StateLogger)
	peer1State := NewGossipStateProvider(logger, chainID, mediator, peers["peer1"].coord, stateMetrics, blocking, stateConfig)
	defer peer1State.Stop()

	mediator = &ServicesMediator{GossipAdapter: peers["peer2"], MCSAdapter: cryptoService}
	logger = flogging.MustGetLogger(gossiputil.StateLogger)
	peer2State := NewGossipStateProvider(logger, chainID, mediator, peers["peer2"].coord, stateMetrics, blocking, stateConfig)
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

func TestStateRequestValidator(t *testing.T) {
	validator := &stateRequestValidator{}
	err := validator.validate(&proto.RemoteStateRequest{
		StartSeqNum: 10,
		EndSeqNum:   5,
	}, defAntiEntropyBatchSize)
	require.Contains(t, err.Error(), "Invalid sequence interval [10...5).")
	require.Error(t, err)

	err = validator.validate(&proto.RemoteStateRequest{
		StartSeqNum: 10,
		EndSeqNum:   30,
	}, defAntiEntropyBatchSize)
	require.Contains(t, err.Error(), "Requesting blocks range [10-30) greater than configured")
	require.Error(t, err)

	err = validator.validate(&proto.RemoteStateRequest{
		StartSeqNum: 10,
		EndSeqNum:   20,
	}, defAntiEntropyBatchSize)
	require.NoError(t, err)
}

func waitUntilTrueOrTimeout(t *testing.T, predicate func() bool, timeout time.Duration) {
	ch := make(chan struct{})
	t.Log("Started to spin off, until predicate will be satisfied.")

	go func() {
		t := time.NewTicker(time.Second)
		for !predicate() {
			select {
			case <-ch:
				t.Stop()
				return
			case <-t.C:
			}
		}
		t.Stop()
		close(ch)
	}()

	select {
	case <-ch:
		t.Log("Done.")
		break
	case <-time.After(timeout):
		close(ch)
		t.Fatal("Timeout has expired")
	}
	t.Log("Stop waiting until timeout or true")
}
