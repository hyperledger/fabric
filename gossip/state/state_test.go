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
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/mocks/validator"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip"
	gossipUtil "github.com/hyperledger/fabric/gossip/util"
	pcomm "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

var (
	portPrefix = 5610
	logger     = gossipUtil.GetLogger(gossipUtil.LoggingStateModule, "")
)

var orgID = []byte("ORG1")
var anchorPeerIdentity = api.PeerIdentityType("identityInOrg1")

type joinChanMsg struct {
}

// SequenceNumber returns the sequence number of the block that the message
// is derived from
func (*joinChanMsg) SequenceNumber() uint64 {
	return uint64(time.Now().UnixNano())
}

// AnchorPeers returns all the anchor peers that are in the channel
func (*joinChanMsg) AnchorPeers() []api.AnchorPeer {
	return []api.AnchorPeer{{Cert: anchorPeerIdentity}}
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

type naiveCryptoService struct {
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveCryptoService) VerifyBlock(chainID common.ChainID, signedBlock api.SignedBlock) error {
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

// VerifyByChannel checks that signature is a valid signature of message
// under a peer's verification key, but also in the context of a specific channel.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerIdentity is nil, then the signature is verified against this peer's verification key.
func (*naiveCryptoService) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*naiveCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

func bootPeers(ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", id+portPrefix))
	}
	return peers
}

// Simple presentation of peer which includes only
// communication module, gossip and state transfer
type peerNode struct {
	g gossip.Gossip
	s GossipStateProvider

	commit committer.Committer
}

// Shutting down all modules used
func (node *peerNode) shutdown() {
	node.s.Stop()
	node.g.Stop()
}

// Default configuration to be used for gossip and communication modules
func newGossipConfig(id int, maxMsgCount int, boot ...int) *gossip.Config {
	port := id + portPrefix
	return &gossip.Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(10) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(4) * time.Second,
		PullPeerNum:                5,
		SelfEndpoint:               fmt.Sprintf("localhost:%d", port),
		PublishCertPeriod:          10 * time.Second,
		RequestStateInfoInterval:   4 * time.Second,
		PublishStateInfoInterval:   4 * time.Second,
	}
}

// Create gossip instance
func newGossipInstance(config *gossip.Config) gossip.Gossip {
	return gossip.NewGossipServiceWithServer(config, &orgCryptoService{}, &naiveCryptoService{}, []byte(config.SelfEndpoint))
}

// Create new instance of KVLedger to be used for testing
func newCommitter(id int) committer.Committer {
	ledger, _ := ledgermgmt.CreateLedger(strconv.Itoa(id))
	cb, _ := test.MakeGenesisBlock(util.GetTestChainID())
	ledger.Commit(cb)
	return committer.NewLedgerCommitter(ledger, &validator.MockValidator{})
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNode(config *gossip.Config, committer committer.Committer) *peerNode {

	// Gossip component based on configuration provided and communication module
	gossip := newGossipInstance(config)

	logger.Debug("Joinning channel", util.GetTestChainID())
	gossip.JoinChan(&joinChanMsg{}, common.ChainID(util.GetTestChainID()))

	// Initialize pseudo peer simulator, which has only three
	// basic parts
	return &peerNode{
		g: gossip,
		s: NewGossipStateProvider(util.GetTestChainID(), gossip, committer),

		commit: committer,
	}
}

/*// Simple scenario to start first booting node, gossip a message
// then start second node and verify second node also receives it
func TestNewGossipStateProvider_GossipingOneMessage(t *testing.T) {
	bootId := 0
	ledgerPath := "/tmp/tests/ledger/"
	defer os.RemoveAll(ledgerPath)

	bootNodeCommitter := newCommitter(bootId, ledgerPath + "node/")
	defer bootNodeCommitter.Close()

	bootNode := newPeerNode(newGossipConfig(bootId, 100), bootNodeCommitter)
	defer bootNode.shutdown()

	rawblock := &peer.Block2{}
	if err := pb.Unmarshal([]byte{}, rawblock); err != nil {
		t.Fail()
	}

	if bytes, err := pb.Marshal(rawblock); err == nil {
		payload := &proto.Payload{1, "", bytes}
		bootNode.s.AddPayload(payload)
	} else {
		t.Fail()
	}

	waitUntilTrueOrTimeout(t, func() bool {
		if block := bootNode.s.GetBlock(uint64(1)); block != nil {
			return true
		}
		return false
	}, 5 * time.Second)

	bootNode.g.Gossip(createDataMsg(uint64(1), []byte{}, ""))

	peerCommitter := newCommitter(1, ledgerPath + "node/")
	defer peerCommitter.Close()

	peer := newPeerNode(newGossipConfig(1, 100, bootId), peerCommitter)
	defer peer.shutdown()

	ready := make(chan interface{})

	go func(p *peerNode) {
		for len(p.g.GetPeers()) != 1 {
			time.Sleep(100 * time.Millisecond)
		}
		ready <- struct{}{}
	}(peer)

	select {
	case <-ready:
		{
			break
		}
	case <-time.After(1 * time.Second):
		{
			t.Fail()
		}
	}

	// Let sure anti-entropy will have a chance to bring missing block
	waitUntilTrueOrTimeout(t, func() bool {
		if block := peer.s.GetBlock(uint64(1)); block != nil {
			return true
		}
		return false
	}, 2 * defAntiEntropyInterval + 1 * time.Second)

	block := peer.s.GetBlock(uint64(1))

	assert.NotNil(t, block)
}

func TestNewGossipStateProvider_RepeatGossipingOneMessage(t *testing.T) {
	for i := 0; i < 10; i++ {
		TestNewGossipStateProvider_GossipingOneMessage(t)
	}
}*/

func TestNewGossipStateProvider_SendingManyMessages(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/tests/ledger/node")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)

	for i := 0; i < bootstrapSetSize; i++ {
		committer := newCommitter(i)
		bootstrapSet = append(bootstrapSet, newPeerNode(newGossipConfig(i, 100), committer))
	}

	defer func() {
		for _, p := range bootstrapSet {
			p.shutdown()
		}
	}()

	msgCount := 10

	for i := 1; i <= msgCount; i++ {
		rawblock := pcomm.NewBlock(uint64(i), []byte{})
		if bytes, err := pb.Marshal(rawblock); err == nil {
			payload := &proto.Payload{uint64(i), "", bytes}
			bootstrapSet[0].s.AddPayload(payload)
		} else {
			t.Fail()
		}
	}

	standartPeersSize := 10
	peersSet := make([]*peerNode, 0)

	for i := 0; i < standartPeersSize; i++ {
		committer := newCommitter(bootstrapSetSize + i)
		peersSet = append(peersSet, newPeerNode(newGossipConfig(bootstrapSetSize+i, 100, 0, 1, 2, 3, 4), committer))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.PeersOfChannel(common.ChainID(util.GetTestChainID()))) != bootstrapSetSize+standartPeersSize-1 {
				logger.Debug("[XXXXXXX]: Peer discovery has not finished yet")
				return false
			}
		}
		logger.Debug("[AAAAAA]: All peer discovered each other!!!")
		return true
	}, 30*time.Second)

	logger.Debug("[!!!!!]: Waiting for all blocks to arrive.")
	waitUntilTrueOrTimeout(t, func() bool {
		logger.Debug("[*****]: Trying to see all peers get all blocks")
		for _, p := range peersSet {
			height, err := p.commit.LedgerHeight()
			if height != uint64(msgCount+1) || err != nil {
				//logger.Debug("[XXXXXXX]: Ledger height is at: ", height)
				return false
			}
		}
		logger.Debug("[#####]: All peers have same ledger height!!!")
		return true
	}, 60*time.Second)
}

func TestGossipStateProvider_TestStateMessages(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/tests/ledger/node")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	bootPeer := newPeerNode(newGossipConfig(0, 100), newCommitter(0))
	defer bootPeer.shutdown()

	peer := newPeerNode(newGossipConfig(1, 100, 0), newCommitter(1))
	defer peer.shutdown()

	_, bootCh := bootPeer.g.Accept(remoteStateMsgFilter, true)
	_, peerCh := peer.g.Accept(remoteStateMsgFilter, true)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		msg := <-bootCh
		logger.Info("Bootstrap node got message, ", msg)
		assert.True(t, msg.GetGossipMessage().GetStateRequest() != nil)
		msg.Respond(&proto.GossipMessage{
			Content: &proto.GossipMessage_StateResponse{&proto.RemoteStateResponse{nil}},
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
		Content: &proto.GossipMessage_StateRequest{&proto.RemoteStateRequest{nil}},
	}, &comm.RemotePeer{peer.g.PeersOfChannel(chainID)[0].Endpoint, peer.g.PeersOfChannel(chainID)[0].PKIid})
	logger.Info("Waiting until peers exchange messages")

	select {
	case <-readyCh:
		{
			logger.Info("[XXX]: Done!!!")

		}
	case <-time.After(time.Duration(10) * time.Second):
		{
			t.Fail()
		}
	}
}

func waitUntilTrueOrTimeout(t *testing.T, predicate func() bool, timeout time.Duration) {
	ch := make(chan struct{})
	go func() {
		logger.Debug("[@@@@@]: Started to spin off, until predicate will be satisfied.")
		for !predicate() {
			time.Sleep(1 * time.Second)
		}
		ch <- struct{}{}
		logger.Debug("[@@@@@]: Done.")
	}()

	select {
	case <-ch:
		break
	case <-time.After(timeout):
		t.Fatal("Timeout has expired")
		break
	}
	logger.Debug("[>>>>>] Stop wainting until timeout or true")
}
