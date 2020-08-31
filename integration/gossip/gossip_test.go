/*
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package gossip

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Gossip Membership", func() {
	var (
		testDir       string
		network       *nwo.Network
		nwprocs       *networkProcesses
		orderer       *nwo.Orderer
		peerEndpoints map[string]string = map[string]string{}
		channelName   string
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gossip-membership")
		Expect(err).NotTo(HaveOccurred())

		dockerClient, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		channelName = "testchannel"
		network = nwo.New(nwo.BasicSolo(), testDir, dockerClient, 25000+1000*GinkgoParallelNode(), components)
		network.GenerateConfigTree()

		//  modify peer config
		for _, peer := range network.Peers {
			core := network.ReadPeerConfig(peer)
			core.Peer.Gossip.AliveTimeInterval = 1 * time.Second
			core.Peer.Gossip.AliveExpirationTimeout = 2 * core.Peer.Gossip.AliveTimeInterval
			core.Peer.Gossip.ReconnectInterval = 2 * time.Second
			core.Peer.Gossip.MsgExpirationFactor = 2
			core.Peer.Gossip.MaxConnectionAttempts = 10
			network.WritePeerConfig(peer, core)
			peerEndpoints[peer.ID()] = core.Peer.Address
		}

		network.Bootstrap()
		orderer = network.Orderer("orderer")
		nwprocs = &networkProcesses{
			network:       network,
			peerRunners:   map[string]*ginkgomon.Runner{},
			peerProcesses: map[string]ifrit.Process{},
			ordererRunner: network.OrdererRunner(orderer),
		}
		nwprocs.ordererProcess = ifrit.Invoke(nwprocs.ordererRunner)
		Eventually(nwprocs.ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if nwprocs != nil {
			nwprocs.terminateAll()
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("updates membership when peers in the same org are stopped and restarted", func() {
		peer0Org1 := network.Peer("Org1", "peer0")
		peer1Org1 := network.Peer("Org1", "peer1")

		By("bringing up all peers")
		startPeers(nwprocs, false, peer0Org1, peer1Org1)

		By("creating and joining a channel")
		network.CreateChannel(channelName, orderer, peer0Org1)
		network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1)
		network.UpdateChannelAnchors(orderer, channelName)

		By("verifying non-anchor peer (peer1Org1) discovers all the peers before testing membership change on it")
		Eventually(nwo.DiscoverPeers(network, peer1Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1),
			network.DiscoveredPeer(peer1Org1),
		))

		By("verifying membership change on non-anchor peer (peer1Org1) when an anchor peer in the same org is stopped and restarted")
		expectedMsgFromExpirationCallback := fmt.Sprintf("Do not remove bootstrap or anchor peer endpoint %s from membership", peerEndpoints[peer0Org1.ID()])
		assertPeerMembershipUpdate(network, peer1Org1, []*nwo.Peer{peer0Org1}, nwprocs, expectedMsgFromExpirationCallback)

		By("verifying anchor peer (peer0Org1) discovers all the peers before testing membership change on it")
		Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1),
			network.DiscoveredPeer(peer1Org1),
		))

		By("verifying membership change on anchor peer (peer0Org1) when a non-anchor peer in the same org is stopped and restarted")
		expectedMsgFromExpirationCallback = fmt.Sprintf("Removing member: Endpoint: %s", peerEndpoints[peer1Org1.ID()])
		assertPeerMembershipUpdate(network, peer0Org1, []*nwo.Peer{peer1Org1}, nwprocs, expectedMsgFromExpirationCallback)
	})

	It("updates peer membership when peers in another org are stopped and restarted", func() {
		peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
		peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

		By("bringing up all peers")
		startPeers(nwprocs, false, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

		By("creating and joining a channel")
		network.CreateChannel(channelName, orderer, peer0Org1)
		network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)
		network.UpdateChannelAnchors(orderer, channelName)

		By("verifying membership on peer1Org1 before testing membership change on it")
		Eventually(nwo.DiscoverPeers(network, peer1Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1),
			network.DiscoveredPeer(peer1Org1),
			network.DiscoveredPeer(peer0Org2),
			network.DiscoveredPeer(peer1Org2),
		))

		By("stopping anchor peer peer0Org1 to have only one peer in org1")
		stopPeers(nwprocs, peer0Org1)

		By("verifying peer membership update when peers in another org are stopped and restarted")
		expectedMsgFromExpirationCallback := fmt.Sprintf("Do not remove bootstrap or anchor peer endpoint %s from membership", peerEndpoints[peer0Org2.ID()])
		assertPeerMembershipUpdate(network, peer1Org1, []*nwo.Peer{peer0Org2, peer1Org2}, nwprocs, expectedMsgFromExpirationCallback)
	})
})

// networkProcesses holds references to the network, its runners, and processes.
type networkProcesses struct {
	network *nwo.Network

	ordererRunner  *ginkgomon.Runner
	ordererProcess ifrit.Process

	peerRunners   map[string]*ginkgomon.Runner
	peerProcesses map[string]ifrit.Process
}

func (n *networkProcesses) terminateAll() {
	if n.ordererProcess != nil {
		n.ordererProcess.Signal(syscall.SIGTERM)
		Eventually(n.ordererProcess.Wait(), n.network.EventuallyTimeout).Should(Receive())
	}
	for _, process := range n.peerProcesses {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), n.network.EventuallyTimeout).Should(Receive())
	}
}

func startPeers(n *networkProcesses, forceStateTransfer bool, peersToStart ...*nwo.Peer) {
	env := []string{"FABRIC_LOGGING_SPEC=info:gossip.state=debug:gossip.discovery=debug"}

	// Setting CORE_PEER_GOSSIP_STATE_CHECKINTERVAL to 200ms (from default of 10s) will ensure that state transfer happens quickly,
	// before blocks are gossipped through normal mechanisms
	if forceStateTransfer {
		env = append(env, "CORE_PEER_GOSSIP_STATE_CHECKINTERVAL=200ms")
	}

	for _, peer := range peersToStart {
		runner := n.network.PeerRunner(peer, env...)
		process := ifrit.Invoke(runner)
		Eventually(process.Ready(), n.network.EventuallyTimeout).Should(BeClosed())

		n.peerProcesses[peer.ID()] = process
		n.peerRunners[peer.ID()] = runner
	}
}

func stopPeers(n *networkProcesses, peersToStop ...*nwo.Peer) {
	for _, peer := range peersToStop {
		id := peer.ID()
		proc := n.peerProcesses[id]
		proc.Signal(syscall.SIGTERM)
		Eventually(proc.Wait(), n.network.EventuallyTimeout).Should(Receive())
		delete(n.peerProcesses, id)
	}
}

// assertPeerMembershipUpdate stops and restart peersToRestart and verify peer membership
func assertPeerMembershipUpdate(network *nwo.Network, peer *nwo.Peer, peersToRestart []*nwo.Peer, nwprocs *networkProcesses, expectedMsgFromExpirationCallback string) {
	stopPeers(nwprocs, peersToRestart...)

	// timeout is the same amount of time as it takes to remove a message from the aliveMsgStore, and add a second as buffer
	core := network.ReadPeerConfig(peer)
	timeout := core.Peer.Gossip.AliveExpirationTimeout*time.Duration(core.Peer.Gossip.MsgExpirationFactor) + time.Second
	By("verifying peer membership after all other peers are stopped")
	Eventually(nwo.DiscoverPeers(network, peer, "User1", "testchannel"), timeout, 100*time.Millisecond).Should(ConsistOf(
		network.DiscoveredPeer(peer),
	))

	By("verifying expected log message from expiration callback")
	runner := nwprocs.peerRunners[peer.ID()]
	Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsgFromExpirationCallback))

	By("restarting peers")
	startPeers(nwprocs, false, peersToRestart...)

	By("verifying peer membership, expected to discover restarted peers")
	expectedPeers := make([]nwo.DiscoveredPeer, len(peersToRestart)+1)
	expectedPeers[0] = network.DiscoveredPeer(peer)
	for i, p := range peersToRestart {
		expectedPeers[i+1] = network.DiscoveredPeer(p)
	}
	timeout = 3 * core.Peer.Gossip.ReconnectInterval
	Eventually(nwo.DiscoverPeers(network, peer, "User1", "testchannel"), timeout, 100*time.Millisecond).Should(ConsistOf(expectedPeers))
}
