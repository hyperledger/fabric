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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Gossip State Transfer", func() {
	var (
		testDir   string
		network   *nwo.Network
		nwprocs   *networkProcesses
		chaincode nwo.Chaincode
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gossip-statexfer")
		Expect(err).NotTo(HaveOccurred())

		dockerClient, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.FullSolo(), testDir, dockerClient, StartPort(), components)
		network.GenerateConfigTree()

		//  modify peer config
		//  Org1: leader election
		//  Org2: no leader election
		//      peer0: follower
		//      peer1: leader
		for _, peer := range network.Peers {
			if peer.Organization == "Org1" {
				if peer.Name == "peer1" {
					core := network.ReadPeerConfig(peer)
					core.Peer.Gossip.Bootstrap = fmt.Sprintf("127.0.0.1:%d", network.ReservePort())
					network.WritePeerConfig(peer, core)
				}
			}
			if peer.Organization == "Org2" {
				core := network.ReadPeerConfig(peer)
				core.Peer.Gossip.UseLeaderElection = false
				core.Peer.Gossip.OrgLeader = peer.Name == "peer1"
				network.WritePeerConfig(peer, core)
			}
		}

		network.Bootstrap()
		nwprocs = &networkProcesses{
			network:       network,
			peerRunners:   map[string]*ginkgomon.Runner{},
			peerProcesses: map[string]ifrit.Process{},
		}

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
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

	It("syncs blocks from the peer when no orderer is available", func() {
		orderer := network.Orderer("orderer")
		nwprocs.ordererRunner = network.OrdererRunner(orderer)
		nwprocs.ordererProcess = ifrit.Invoke(nwprocs.ordererRunner)
		Eventually(nwprocs.ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
		peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

		By("bringing up all four peers")
		startPeers(nwprocs, false, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

		channelName := "testchannel"
		network.CreateChannel(channelName, orderer, peer0Org1)
		By("joining all peers to channel")
		network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

		network.UpdateChannelAnchors(orderer, channelName)

		// base peer will be used for chaincode interactions
		basePeerForTransactions := peer0Org1
		nwo.DeployChaincodeLegacy(network, channelName, orderer, chaincode, basePeerForTransactions)

		By("STATE TRANSFER TEST 1: newly joined peers should receive blocks from the peers that are already up")

		// Note, a better test would be to bring orderer down before joining the two peers.
		// However, network.JoinChannel() requires orderer to be up so that genesis block can be fetched from orderer before joining peers.
		// Therefore, for now we've joined all four peers and stop the two peers that should be synced up.
		stopPeers(nwprocs, peer1Org1, peer1Org2)

		By("confirming peer0Org1 was elected to be a leader")
		expectedMsg := "Elected as a leader, starting delivery service for channel testchannel"
		Eventually(nwprocs.peerRunners[peer0Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

		sendTransactionsAndSyncUpPeers(nwprocs, orderer, basePeerForTransactions, channelName, peer1Org1, peer1Org2)

		By("STATE TRANSFER TEST 2: restarted peers should receive blocks from the peers that are already up")
		basePeerForTransactions = peer1Org1
		nwo.InstallChaincodeLegacy(network, chaincode, basePeerForTransactions)

		By("stopping peer0Org1 (currently elected leader in Org1) and peer1Org2 (static leader in Org2)")
		stopPeers(nwprocs, peer0Org1, peer1Org2)

		By("confirming peer1Org1 was elected to be a leader")
		Eventually(nwprocs.peerRunners[peer1Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

		// Note that with the static leader in Org2 down, the static follower peer0Org2 will also get blocks via state transfer
		// This effectively tests leader election as well, since the newly elected leader in Org1 (peer1Org1) will be the only peer
		// that receives blocks from orderer and will therefore serve as the provider of blocks to all other peers.
		sendTransactionsAndSyncUpPeers(nwprocs, orderer, basePeerForTransactions, channelName, peer0Org1, peer1Org2)
	})
})

func runTransactions(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, chaincodeName string, channelID string) {
	for i := 0; i < 5; i++ {
		sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
			ChannelID: channelID,
			Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
			Name:      chaincodeName,
			Ctor:      `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: []string{
				n.PeerAddress(peer, nwo.ListenPort),
			},
			WaitForEvent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
	}
}

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
	env := []string{"FABRIC_LOGGING_SPEC=info:gossip.state=debug"}

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

func assertPeersLedgerHeight(n *nwo.Network, peersToSyncUp []*nwo.Peer, expectedVal int, channelID string) {
	for _, peer := range peersToSyncUp {
		Eventually(func() int {
			return nwo.GetLedgerHeight(n, peer, channelID)
		}, n.EventuallyTimeout).Should(Equal(expectedVal))
	}
}

// send transactions, stop orderering server, then start peers to ensure they received blcoks via state transfer
func sendTransactionsAndSyncUpPeers(n *networkProcesses, orderer *nwo.Orderer, basePeer *nwo.Peer, channelName string, peersToSyncUp ...*nwo.Peer) {
	By("creating transactions")
	runTransactions(n.network, orderer, basePeer, "mycc", channelName)
	basePeerLedgerHeight := nwo.GetLedgerHeight(n.network, basePeer, channelName)

	By("stopping orderer")
	n.ordererProcess.Signal(syscall.SIGTERM)
	Eventually(n.ordererProcess.Wait(), n.network.EventuallyTimeout).Should(Receive())
	n.ordererProcess = nil

	By("starting the peers contained in the peersToSyncUp list")
	startPeers(n, true, peersToSyncUp...)

	By("ensuring the peers are synced up")
	assertPeersLedgerHeight(n.network, peersToSyncUp, basePeerLedgerHeight, channelName)

	By("restarting orderer")
	n.ordererRunner = n.network.OrdererRunner(orderer)
	n.ordererProcess = ifrit.Invoke(n.ordererRunner)
	Eventually(n.ordererProcess.Ready(), n.network.EventuallyTimeout).Should(BeClosed())
}
