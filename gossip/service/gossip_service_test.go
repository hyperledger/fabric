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

package service

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/gossip/api"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/gossip/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func init() {
	util.SetupTestLogging()
}

func TestInitGossipService(t *testing.T) {
	// Test whenever gossip service is indeed singleton
	grpcServer := grpc.NewServer()
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5611))
	assert.NoError(t, error)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	msptesttools.LoadMSPSetupForTesting()
	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()

	wg := sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			messageCryptoService := peergossip.NewMCS(&mocks.ChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())
			secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
			err := InitGossipService(identity, "localhost:5611", grpcServer, messageCryptoService,
				secAdv, nil)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	defer GetGossipService().Stop()
	gossip := GetGossipService()

	for i := 0; i < 10; i++ {
		go func(gossipInstance GossipService) {
			assert.Equal(t, gossip, GetGossipService())
		}(gossip)
	}

	time.Sleep(time.Second * 2)
}

// Make sure *joinChannelMessage implements the api.JoinChannelMessage
func TestJCMInterface(t *testing.T) {
	_ = api.JoinChannelMessage(&joinChannelMessage{})
}

func TestLeaderElectionWithDeliverClient(t *testing.T) {

	//Test check if leader election works with mock deliver service instance
	//Configuration set to use dynamic leader election
	//10 peers started, added to channel and at the end we check if only for one peer
	//mockDeliverService.StartDeliverForChannel was invoked

	viper.Set("peer.gossip.useLeaderElection", true)
	viper.Set("peer.gossip.orgLeader", false)

	n := 10
	gossips := startPeers(t, n, 20000)

	channelName := "chanA"
	peerIndexes := make([]int, n)
	for i := 0; i < n; i++ {
		peerIndexes[i] = i
	}
	addPeersToChannel(t, n, 20000, channelName, gossips, peerIndexes)

	waitForFullMembership(t, gossips, n, time.Second*20, time.Second*2)

	services := make([]*electionService, n)

	for i := 0; i < n; i++ {
		deliverServiceFactory := &mockDeliverServiceFactory{
			service: &mockDeliverService{
				running: make(map[string]bool),
			},
		}
		gossips[i].(*gossipServiceImpl).deliveryFactory = deliverServiceFactory
		deliverServiceFactory.service.running[channelName] = false

		gossips[i].InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:5005"})
		service, exist := gossips[i].(*gossipServiceImpl).leaderElection[channelName]
		assert.True(t, exist, "Leader election service should be created for peer %d and channel %s", i, channelName)
		services[i] = &electionService{nil, false, 0}
		services[i].LeaderElectionService = service
	}

	// Is single leader was elected.
	assert.True(t, waitForLeaderElection(t, services, time.Second*30, time.Second*2), "One leader should be selected")

	startsNum := 0
	for i := 0; i < n; i++ {
		// Is mockDeliverService.StartDeliverForChannel in current peer for the specific channel was invoked
		if gossips[i].(*gossipServiceImpl).deliveryService.(*mockDeliverService).running[channelName] {
			startsNum++
		}
	}

	assert.Equal(t, 1, startsNum, "Only for one peer delivery client should start")

	stopPeers(gossips)
}

func TestWithStaticDeliverClientLeader(t *testing.T) {

	//Tests check if static leader flag works ok.
	//Leader election flag set to false, and static leader flag set to true
	//Two gossip service instances (peers) created.
	//Each peer is added to channel and should run mock delivery client
	//After that each peer added to another client and it should run deliver client for this channel as well.

	viper.Set("peer.gossip.useLeaderElection", false)
	viper.Set("peer.gossip.orgLeader", true)

	n := 2
	gossips := startPeers(t, n, 20000)

	channelName := "chanA"
	peerIndexes := make([]int, n)
	for i := 0; i < n; i++ {
		peerIndexes[i] = i
	}

	addPeersToChannel(t, n, 20000, channelName, gossips, peerIndexes)

	waitForFullMembership(t, gossips, n, time.Second*30, time.Second*2)

	deliverServiceFactory := &mockDeliverServiceFactory{
		service: &mockDeliverService{
			running: make(map[string]bool),
		},
	}

	for i := 0; i < n; i++ {
		gossips[i].(*gossipServiceImpl).deliveryFactory = deliverServiceFactory
		deliverServiceFactory.service.running[channelName] = false
		gossips[i].InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:5005"})
	}

	for i := 0; i < n; i++ {
		assert.NotNil(t, gossips[i].(*gossipServiceImpl).deliveryService, "Delivery service not initiated in peer %d", i)
		assert.True(t, gossips[i].(*gossipServiceImpl).deliveryService.(*mockDeliverService).running[channelName], "Block deliverer not started for peer %d", i)
	}

	channelName = "chanB"
	for i := 0; i < n; i++ {
		deliverServiceFactory.service.running[channelName] = false
		gossips[i].InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:5005"})
	}

	for i := 0; i < n; i++ {
		assert.NotNil(t, gossips[i].(*gossipServiceImpl).deliveryService, "Delivery service not initiated in peer %d", i)
		assert.True(t, gossips[i].(*gossipServiceImpl).deliveryService.(*mockDeliverService).running[channelName], "Block deliverer not started for peer %d", i)
	}

	stopPeers(gossips)
}

func TestWithStaticDeliverClientNotLeader(t *testing.T) {
	viper.Set("peer.gossip.useLeaderElection", false)
	viper.Set("peer.gossip.orgLeader", false)

	n := 2
	gossips := startPeers(t, n, 20000)

	channelName := "chanA"
	peerIndexes := make([]int, n)
	for i := 0; i < n; i++ {
		peerIndexes[i] = i
	}

	addPeersToChannel(t, n, 20000, channelName, gossips, peerIndexes)

	waitForFullMembership(t, gossips, n, time.Second*30, time.Second*2)

	deliverServiceFactory := &mockDeliverServiceFactory{
		service: &mockDeliverService{
			running: make(map[string]bool),
		},
	}

	for i := 0; i < n; i++ {
		gossips[i].(*gossipServiceImpl).deliveryFactory = deliverServiceFactory
		deliverServiceFactory.service.running[channelName] = false
		gossips[i].InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:5005"})
	}

	for i := 0; i < n; i++ {
		assert.NotNil(t, gossips[i].(*gossipServiceImpl).deliveryService, "Delivery service not initiated in peer %d", i)
		assert.False(t, gossips[i].(*gossipServiceImpl).deliveryService.(*mockDeliverService).running[channelName], "Block deliverer should not be started for peer %d", i)
	}

	stopPeers(gossips)
}

func TestWithStaticDeliverClientBothStaticAndLeaderElection(t *testing.T) {
	viper.Set("peer.gossip.useLeaderElection", true)
	viper.Set("peer.gossip.orgLeader", true)

	n := 2
	gossips := startPeers(t, n, 20000)

	channelName := "chanA"
	peerIndexes := make([]int, n)
	for i := 0; i < n; i++ {
		peerIndexes[i] = i
	}

	addPeersToChannel(t, n, 20000, channelName, gossips, peerIndexes)

	waitForFullMembership(t, gossips, n, time.Second*30, time.Second*2)

	deliverServiceFactory := &mockDeliverServiceFactory{
		service: &mockDeliverService{
			running: make(map[string]bool),
		},
	}

	for i := 0; i < n; i++ {
		gossips[i].(*gossipServiceImpl).deliveryFactory = deliverServiceFactory
		assert.Panics(t, func() {
			gossips[i].InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:5005"})
		}, "Dynamic leader lection based and static connection to ordering service can't exist simultaniosly")
	}

	stopPeers(gossips)
}

type mockDeliverServiceFactory struct {
	service *mockDeliverService
}

func (mf *mockDeliverServiceFactory) Service(g GossipService, endpoints []string, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	return mf.service, nil
}

type mockDeliverService struct {
	running map[string]bool
}

func (ds *mockDeliverService) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error {
	ds.running[chainID] = true
	return nil
}

func (ds *mockDeliverService) StopDeliverForChannel(chainID string) error {
	ds.running[chainID] = false
	return nil
}

func (ds *mockDeliverService) Stop() {
}

type mockLedgerInfo struct {
	Height uint64
}

// LedgerHeight returns mocked value to the ledger height
func (li *mockLedgerInfo) LedgerHeight() (uint64, error) {
	return li.Height, nil
}

// Commit block to the ledger
func (li *mockLedgerInfo) Commit(block *common.Block) error {
	return nil
}

// Gets blocks with sequence numbers provided in the slice
func (li *mockLedgerInfo) GetBlocks(blockSeqs []uint64) []*common.Block {
	return make([]*common.Block, 0)
}

// Closes committing service
func (li *mockLedgerInfo) Close() {
}

func TestLeaderElectionWithRealGossip(t *testing.T) {

	// Spawn 10 gossip instances with single channel and inside same organization
	// Run leader election on top of each gossip instance and check that only one leader chosen
	// Create another channel includes sub-set of peers over same gossip instances {1,3,5,7}
	// Run additional leader election services for new channel
	// Check correct leader still exist for first channel and new correct leader chosen in second channel
	// Stop gossip instances of leader peers for both channels and see that new leader chosen for both

	// Creating gossip service instances for peers
	n := 10
	gossips := startPeers(t, n, 20000)

	// Joining all peers to first channel
	channelName := "chanA"
	peerIndexes := make([]int, n)
	for i := 0; i < n; i++ {
		peerIndexes[i] = i
	}
	addPeersToChannel(t, n, 20000, channelName, gossips, peerIndexes)

	waitForFullMembership(t, gossips, n, time.Second*30, time.Second*2)

	logger.Warning("Starting leader election services")

	//Starting leader election services
	services := make([]*electionService, n)

	for i := 0; i < n; i++ {
		services[i] = &electionService{nil, false, 0}
		services[i].LeaderElectionService = gossips[i].(*gossipServiceImpl).newLeaderElectionComponent(channelName, services[i].callback)
	}

	logger.Warning("Waiting for leader election")

	assert.True(t, waitForLeaderElection(t, services, time.Second*30, time.Second*2), "One leader should be selected")

	startsNum := 0
	for i := 0; i < n; i++ {
		// Is callback function was invoked by this leader election service instance
		if services[i].callbackInvokeRes {
			startsNum++
		}
	}
	//Only leader should invoke callback function, so it is double check that only one leader exists
	assert.Equal(t, 1, startsNum, "Only for one peer callback function should be called - chanA")

	// Adding some peers to new channel and creating leader election services for peers in new channel
	// Expecting peer 1 (first in list of election services) to become leader of second channel
	secondChannelPeerIndexes := []int{1, 3, 5, 7}
	secondChannelName := "chanB"
	secondChannelServices := make([]*electionService, len(secondChannelPeerIndexes))
	addPeersToChannel(t, n, 20000, secondChannelName, gossips, secondChannelPeerIndexes)

	for idx, i := range secondChannelPeerIndexes {
		secondChannelServices[idx] = &electionService{nil, false, 0}
		secondChannelServices[idx].LeaderElectionService = gossips[i].(*gossipServiceImpl).newLeaderElectionComponent(secondChannelName, secondChannelServices[idx].callback)
	}

	assert.True(t, waitForLeaderElection(t, secondChannelServices, time.Second*30, time.Second*2), "One leader should be selected for chanB")
	assert.True(t, waitForLeaderElection(t, services, time.Second*30, time.Second*2), "One leader should be selected for chanA")

	startsNum = 0
	for i := 0; i < n; i++ {
		if services[i].callbackInvokeRes {
			startsNum++
		}
	}
	assert.Equal(t, 1, startsNum, "Only for one peer callback function should be called - chanA")

	startsNum = 0
	for i := 0; i < len(secondChannelServices); i++ {
		if secondChannelServices[i].callbackInvokeRes {
			startsNum++
		}
	}
	assert.Equal(t, 1, startsNum, "Only for one peer callback function should be called - chanB")

	//Stopping 2 gossip instances(peer 0 and peer 1), should init re-election
	//Now peer 2 become leader for first channel and peer 3 for second channel

	logger.Warning("Killing 2 peers, initiation new leader election")

	stopPeers(gossips[:2])

	waitForFullMembership(t, gossips[2:], n-2, time.Second*30, time.Second*2)

	assert.True(t, waitForLeaderElection(t, services[2:], time.Second*30, time.Second*2), "One leader should be selected after re-election - chanA")
	assert.True(t, waitForLeaderElection(t, secondChannelServices[1:], time.Second*30, time.Second*2), "One leader should be selected after re-election - chanB")

	startsNum = 0
	for i := 2; i < n; i++ {
		if services[i].callbackInvokeRes {
			startsNum++
		}
	}
	assert.Equal(t, 1, startsNum, "Only for one peer callback function should be called after re-election - chanA")

	startsNum = 0
	for i := 1; i < len(secondChannelServices); i++ {
		if secondChannelServices[i].callbackInvokeRes {
			startsNum++
		}
	}
	assert.Equal(t, 1, startsNum, "Only for one peer callback function should be called after re-election - chanB")

	stopServices(secondChannelServices)
	stopServices(services)
	stopPeers(gossips[2:])
}

type electionService struct {
	election.LeaderElectionService
	callbackInvokeRes   bool
	callbackInvokeCount int
}

func (es *electionService) callback(isLeader bool) {
	es.callbackInvokeRes = isLeader
	es.callbackInvokeCount = es.callbackInvokeCount + 1
}

type joinChanMsg struct {
}

// SequenceNumber returns the sequence number of the block this joinChanMsg
// is derived from
func (jmc *joinChanMsg) SequenceNumber() uint64 {
	return uint64(time.Now().UnixNano())
}

// Members returns the organizations of the channel
func (jmc *joinChanMsg) Members() []api.OrgIdentityType {
	return []api.OrgIdentityType{orgInChannelA}
}

// AnchorPeersOf returns the anchor peers of the given organization
func (jmc *joinChanMsg) AnchorPeersOf(org api.OrgIdentityType) []api.AnchorPeer {
	return []api.AnchorPeer{}
}

func waitForFullMembership(t *testing.T, gossips []GossipService, peersNum int, timeout time.Duration, testPollInterval time.Duration) bool {
	end := time.Now().Add(timeout)
	var correctPeers int
	for time.Now().Before(end) {
		correctPeers = 0
		for _, g := range gossips {
			if len(g.Peers()) == (peersNum - 1) {
				correctPeers++
			}
		}
		if correctPeers == peersNum {
			return true
		}
		time.Sleep(testPollInterval)
	}
	logger.Warningf("Only %d peers have full membership", correctPeers)
	return false
}

func waitForMultipleLeadersElection(t *testing.T, services []*electionService, leadersNum int, timeout time.Duration, testPollInterval time.Duration) bool {
	logger.Warning("Waiting for", leadersNum, "leaders")
	end := time.Now().Add(timeout)
	correctNumberOfLeadersFound := false
	leaders := 0
	for time.Now().Before(end) {
		leaders = 0
		for _, s := range services {
			if s.IsLeader() {
				leaders++
			}
		}
		if leaders == leadersNum {
			if correctNumberOfLeadersFound {
				return true
			}
			correctNumberOfLeadersFound = true
		} else {
			correctNumberOfLeadersFound = false
		}
		time.Sleep(testPollInterval)
	}
	logger.Warning("Incorrect number of leaders", leaders)
	for i, s := range services {
		logger.Warning("Peer at index", i, "is leader", s.IsLeader())
	}
	return false
}

func waitForLeaderElection(t *testing.T, services []*electionService, timeout time.Duration, testPollInterval time.Duration) bool {
	return waitForMultipleLeadersElection(t, services, 1, timeout, testPollInterval)
}

func waitUntilOrFailBlocking(t *testing.T, f func(), timeout time.Duration) {
	successChan := make(chan struct{}, 1)
	go func() {
		f()
		successChan <- struct{}{}
	}()
	select {
	case <-time.NewTimer(timeout).C:
		break
	case <-successChan:
		return
	}
	util.PrintStackTrace()
	assert.Fail(t, "Timeout expired!")
}

func stopServices(services []*electionService) {
	stoppingWg := sync.WaitGroup{}
	stoppingWg.Add(len(services))
	for i, sI := range services {
		go func(i int, s_i election.LeaderElectionService) {
			defer stoppingWg.Done()
			s_i.Stop()
		}(i, sI)
	}
	stoppingWg.Wait()
	time.Sleep(time.Second * time.Duration(2))
}

func stopPeers(peers []GossipService) {
	stoppingWg := sync.WaitGroup{}
	stoppingWg.Add(len(peers))
	for i, pI := range peers {
		go func(i int, p_i GossipService) {
			defer stoppingWg.Done()
			p_i.Stop()
		}(i, pI)
	}
	stoppingWg.Wait()
	time.Sleep(time.Second * time.Duration(2))
}

func addPeersToChannel(t *testing.T, n int, portPrefix int, channel string, peers []GossipService, peerIndexes []int) {
	jcm := &joinChanMsg{}

	wg := sync.WaitGroup{}
	for _, i := range peerIndexes {
		wg.Add(1)
		go func(i int) {
			peers[i].JoinChan(jcm, gossipCommon.ChainID(channel))
			peers[i].UpdateChannelMetadata([]byte("bla bla"), gossipCommon.ChainID(channel))
			wg.Done()
		}(i)
	}
	waitUntilOrFailBlocking(t, wg.Wait, time.Second*10)
}

func startPeers(t *testing.T, n int, portPrefix int) []GossipService {

	peers := make([]GossipService, n)
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {

			peers[i] = newGossipInstance(portPrefix, i, 100, 0, 1, 2, 3, 4, 5)
			wg.Done()
		}(i)
	}
	waitUntilOrFailBlocking(t, wg.Wait, time.Second*10)

	return peers
}

func newGossipInstance(portPrefix int, id int, maxMsgCount int, boot ...int) GossipService {
	port := id + portPrefix
	conf := &gossip.Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(portPrefix, boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(500) * time.Millisecond,
		MaxPropagationBurstSize:    20,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(2) * time.Second,
		PullPeerNum:                5,
		InternalEndpoint:           fmt.Sprintf("localhost:%d", port),
		ExternalEndpoint:           fmt.Sprintf("1.2.3.4:%d", port),
		PublishCertPeriod:          time.Duration(4) * time.Second,
		PublishStateInfoInterval:   time.Duration(1) * time.Second,
		RequestStateInfoInterval:   time.Duration(1) * time.Second,
	}
	selfId := api.PeerIdentityType(conf.InternalEndpoint)
	cryptoService := &naiveCryptoService{}
	idMapper := identity.NewIdentityMapper(cryptoService, selfId)

	gossip := gossip.NewGossipServiceWithServer(conf, &orgCryptoService{}, cryptoService,
		idMapper, selfId, nil)

	gossipService := &gossipServiceImpl{
		gossipSvc:       gossip,
		chains:          make(map[string]state.GossipStateProvider),
		leaderElection:  make(map[string]election.LeaderElectionService),
		deliveryFactory: &deliveryFactoryImpl{},
		idMapper:        idMapper,
		peerIdentity:    api.PeerIdentityType(conf.InternalEndpoint),
	}

	return gossipService
}

func bootPeers(portPrefix int, ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", id+portPrefix))
	}
	return peers
}

type naiveCryptoService struct {
}

type orgCryptoService struct {
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity
func (*orgCryptoService) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return orgInChannelA
}

// Verify verifies a JoinChanMessage, returns nil on success,
// and an error on failure
func (*orgCryptoService) Verify(joinChanMsg api.JoinChannelMessage) error {
	return nil
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (*naiveCryptoService) VerifyByChannel(_ gossipCommon.ChainID, _ api.PeerIdentityType, _, _ []byte) error {
	return nil
}

func (*naiveCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) gossipCommon.PKIidType {
	return gossipCommon.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveCryptoService) VerifyBlock(chainID gossipCommon.ChainID, seqNum uint64, signedBlock []byte) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*naiveCryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong signature:%v, %v", signature, message)
	}
	return nil
}

var orgInChannelA = api.OrgIdentityType("ORG1")

func TestInvalidInitialization(t *testing.T) {
	// Test whenever gossip service is indeed singleton
	grpcServer := grpc.NewServer()
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 7611))
	assert.NoError(t, error)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	err := InitGossipService(api.PeerIdentityType("IDENTITY"), "localhost:7611", grpcServer,
		&naiveCryptoService{}, secAdv, nil)
	assert.NoError(t, err)
	gService := GetGossipService().(*gossipServiceImpl)
	defer gService.Stop()

	dc, err := gService.deliveryFactory.Service(gService, []string{}, &naiveCryptoService{})
	assert.Nil(t, dc)
	assert.Error(t, err)

	dc, err = gService.deliveryFactory.Service(gService, []string{"localhost:1984"}, &naiveCryptoService{})
	assert.NotNil(t, dc)
	assert.NoError(t, err)
}

func TestChannelConfig(t *testing.T) {
	// Test whenever gossip service is indeed singleton
	grpcServer := grpc.NewServer()
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 6611))
	assert.NoError(t, error)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	error = InitGossipService(api.PeerIdentityType("IDENTITY"), "localhost:6611", grpcServer,
		&naiveCryptoService{}, secAdv, nil)
	assert.NoError(t, error)
	gService := GetGossipService().(*gossipServiceImpl)
	defer gService.Stop()

	jcm := &joinChannelMessage{seqNum: 1, members2AnchorPeers: map[string][]api.AnchorPeer{
		"A": {{Host: "host", Port: 5000}},
	}}

	assert.Equal(t, uint64(1), jcm.SequenceNumber())

	mc := &mockConfig{
		sequence: 1,
		orgs: map[string]config.ApplicationOrg{
			string(orgInChannelA): &appGrp{
				mspID:       string(orgInChannelA),
				anchorPeers: []*peer.AnchorPeer{},
			},
		},
	}
	gService.JoinChan(jcm, gossipCommon.ChainID("A"))
	gService.configUpdated(mc)
	assert.True(t, gService.amIinChannel(string(orgInChannelA), mc))
}
