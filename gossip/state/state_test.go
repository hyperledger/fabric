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
	"os"
	"strconv"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/proto"
	pcomm "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

var (
	portPrefix = 5610
	logger, _  = logging.GetLogger("GossipStateProviderTest")
)

type naiveCryptoService struct {
}

func (*naiveCryptoService) ValidateAliveMsg(am *proto.AliveMessage) bool {
	return true
}

func (*naiveCryptoService) SignMessage(am *proto.AliveMessage) *proto.AliveMessage {
	return am
}

func (*naiveCryptoService) IsEnabled() bool {
	return true
}

func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*naiveCryptoService) Verify(vkID, signature, message []byte) error {
	if bytes.Equal(signature, message) {
		return nil
	}
	return fmt.Errorf("Failed verifying")
}

func bootPeers(ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", (id+portPrefix)))
	}
	return peers
}

// Simple presentation of peer which includes only
// communication module, gossip and state transfer
type peerNode struct {
	c comm.Comm
	g gossip.Gossip
	s GossipStateProvider

	commit committer.Committer
}

// Shutting down all modules used
func (node *peerNode) shutdown() {
	node.s.Stop()
	node.c.Stop()
	node.g.Stop()
}

// Default configuration to be used for gossip and communication modules
func newGossipConfig(id int, maxMsgCount int, boot ...int) *gossip.Config {
	port := id + portPrefix
	return &gossip.Config{
		BindPort:       port,
		BootstrapPeers: bootPeers(boot...),
		ID:             fmt.Sprintf("p%d", id),
		MaxMessageCountToStore:     maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(10) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(4) * time.Second,
		PullPeerNum:                5,
		SelfEndpoint:               fmt.Sprintf("localhost:%d", port),
	}
}

// Create gossip instance
func newGossipInstance(config *gossip.Config, comm comm.Comm) gossip.Gossip {
	return gossip.NewGossipService(config, comm, &naiveCryptoService{})
}

// Setup and create basic communication module
// need to be used for peer-to-peer communication
// between peers and state transfer
func newCommInstance(config *gossip.Config) comm.Comm {
	comm, err := comm.NewCommInstanceWithServer(config.BindPort, &naiveCryptoService{}, []byte(config.SelfEndpoint))
	if err != nil {
		panic(err)
	}

	return comm
}

// Create new instance of KVLedger to be used for testing
func newCommitter(id int, basePath string) committer.Committer {
	conf := kvledger.NewConf(basePath+strconv.Itoa(id), 0)
	ledger, _ := kvledger.NewKVLedger(conf)
	return committer.NewLedgerCommitter(ledger)
}

// Constructing pseudo peer node, simulating only gossip and state transfer part
func newPeerNode(config *gossip.Config, committer committer.Committer) *peerNode {

	// Create communication module instance
	comm := newCommInstance(config)
	// Gossip component based on configuration provided and communication module
	gossip := newGossipInstance(config, comm)

	// Initialize pseudo peer simulator, which has only three
	// basic parts
	return &peerNode{
		c: comm,
		g: gossip,
		s: NewGossipStateProvider(gossip, comm, committer),

		commit: committer,
	}
}

func createDataMsg(seqnum uint64, data []byte, hash string) *proto.GossipMessage {
	return &proto.GossipMessage{
		Nonce: 0,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   data,
					Hash:   hash,
					SeqNum: seqnum,
				},
			},
		},
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
	ledgerPath := "/tmp/tests/ledger/"
	defer os.RemoveAll(ledgerPath)

	bootstrapSetSize := 5
	bootstrapSet := make([]*peerNode, 0)

	for i := 0; i < bootstrapSetSize; i++ {
		committer := newCommitter(i, ledgerPath+"node/")
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
		committer := newCommitter(standartPeersSize+i, ledgerPath+"node/")
		peersSet = append(peersSet, newPeerNode(newGossipConfig(standartPeersSize+i, 100, 0, 1, 2, 3, 4), committer))
	}

	defer func() {
		for _, p := range peersSet {
			p.shutdown()
		}
	}()

	waitUntilTrueOrTimeout(t, func() bool {
		for _, p := range peersSet {
			if len(p.g.GetPeers()) != bootstrapSetSize+standartPeersSize-1 {
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
			if height != uint64(msgCount) || err != nil {
				logger.Debug("[XXXXXXX]: Ledger height is at: ", height)
				return false
			}
		}
		logger.Debug("[#####]: All peers have same ledger height!!!")
		return true
	}, 60*time.Second)

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
