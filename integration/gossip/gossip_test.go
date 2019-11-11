/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
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

var _ = Describe("Gossip Test", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
	})

	AfterEach(func() {
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("Gossip state transfer test", func() {
		var (
			ordererProcess ifrit.Process
			peerProcesses  = map[string]ifrit.Process{}
			peerRunners    = map[string]*ginkgomon.Runner{}
		)

		BeforeEach(func() {
			network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)

			network.GenerateConfigTree()
			//  modify peer config
			//  Org1: leader election
			//  Org2: no leader election
			//      peer0: follower
			//      peer1: leader
			for _, peer := range network.Peers {
				if peer.Organization == "Org1" {
					core := network.ReadPeerConfig(peer)
					if peer.Name == "peer1" {
						core.Peer.Gossip.Bootstrap = "127.0.0.1:21004"
						network.WritePeerConfig(peer, core)
					}

				}
				if peer.Organization == "Org2" {
					core := network.ReadPeerConfig(peer)
					core.Peer.Gossip.UseLeaderElection = false
					if peer.Name == "peer1" {
						core.Peer.Gossip.OrgLeader = true
					} else {
						core.Peer.Gossip.OrgLeader = false
					}

					network.WritePeerConfig(peer, core)
				}
			}
			network.Bootstrap()
		})

		AfterEach(func() {
			if ordererProcess != nil {
				ordererProcess.Signal(syscall.SIGTERM)
				Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}

			for _, process := range peerProcesses {
				process.Signal(syscall.SIGTERM)
				Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("syncs blocks from the peer if no orderer is available, using solo network with 2 orgs, 2 peers each", func() {
			orderer := network.Orderer("orderer")
			ordererRunner := network.OrdererRunner(orderer)
			ordererProcess = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
			peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

			By("bring up all four peers")
			peersToBringUp := []*nwo.Peer{peer0Org1, peer1Org1, peer0Org2, peer1Org2}
			startPeers(network, peersToBringUp, peerProcesses, peerRunners, false)

			channelName := "testchannel"
			network.CreateChannel(channelName, orderer, peer0Org1)
			By("join all peers to channel")
			network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

			network.UpdateChannelAnchors(orderer, channelName)

			// base peer will be used for chaincode interactions
			basePeerForTransactions := peer0Org1
			nwo.DeployChaincodeLegacy(network, channelName, orderer, chaincode, basePeerForTransactions)

			By("STATE TRANSFER TEST 1: newly joined peers should receive blocks from the peers that are already up")

			// Note, a better test would be to bring orderer down before joining the two peers.
			// However, network.JoinChannel() requires orderer to be up so that genesis block can be fetched from orderer before joining peers.
			// Therefore, for now we've joined all four peers and stop the two peers that should be synced up.
			peersToStop := []*nwo.Peer{peer1Org1, peer1Org2}
			stopPeers(network, peersToStop, peerProcesses)

			By("confirm peer0Org1 elected to be a leader")
			expectedMsg := "Elected as a leader, starting delivery service for channel testchannel"
			Eventually(peerRunners[peer0Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

			peersToSyncUp := []*nwo.Peer{peer1Org1, peer1Org2}
			sendTransactionsAndSyncUpPeers(network, orderer, basePeerForTransactions, peersToSyncUp, channelName, &ordererProcess, ordererRunner, peerProcesses, peerRunners)

			By("STATE TRANSFER TEST 2: restarted peers should receive blocks from the peers that are already up")
			basePeerForTransactions = peer1Org1
			nwo.InstallChaincodeLegacy(network, chaincode, basePeerForTransactions)

			By("stop peer0Org1 (currently elected leader in Org1) and peer1Org2 (static leader in Org2)")
			peersToStop = []*nwo.Peer{peer0Org1, peer1Org2}
			stopPeers(network, peersToStop, peerProcesses)

			By("confirm peer1Org1 elected to be a leader")
			Eventually(peerRunners[peer1Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

			peersToSyncUp = []*nwo.Peer{peer0Org1, peer1Org2}
			// Note that with the static leader in Org2 down, the static follower peer0Org2 will also get blocks via state transfer
			// This effectively tests leader election as well, since the newly elected leader in Org1 (peer1Org1) will be the only peer
			// that receives blocks from orderer and will therefore serve as the provider of blocks to all other peers.
			sendTransactionsAndSyncUpPeers(network, orderer, basePeerForTransactions, peersToSyncUp, channelName, &ordererProcess, ordererRunner, peerProcesses, peerRunners)

		})

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

func startPeers(network *nwo.Network, peersToStart []*nwo.Peer, peerProc map[string]ifrit.Process, peerRun map[string]*ginkgomon.Runner, forceStateTransfer bool) {

	env := []string{fmt.Sprint("FABRIC_LOGGING_SPEC=info:gossip.state=debug")}

	// Setting CORE_PEER_GOSSIP_STATE_CHECKINTERVAL to 200ms (from default of 10s) will ensure that state transfer happens quickly,
	// before blocks are gossipped through normal mechanisms
	if forceStateTransfer {
		env = append(env, fmt.Sprint("CORE_PEER_GOSSIP_STATE_CHECKINTERVAL=200ms"))
	}

	for _, peer := range peersToStart {
		runner := network.PeerRunner(peer, env...)
		process := ifrit.Invoke(runner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peerProc[peer.ID()] = process
		peerRun[peer.ID()] = runner
	}
}

func stopPeers(network *nwo.Network, peersToStop []*nwo.Peer, peerProcesses map[string]ifrit.Process) {
	for _, peer := range peersToStop {
		id := peer.ID()
		proc := peerProcesses[id]
		proc.Signal(syscall.SIGTERM)
		Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
		delete(peerProcesses, id)
	}
}

func assertPeersLedgerHeight(n *nwo.Network, orderer *nwo.Orderer, peersToSyncUp []*nwo.Peer, expectedVal int, channelID string) {
	for _, peer := range peersToSyncUp {
		Eventually(func() int {
			return nwo.GetLedgerHeight(n, peer, channelID)
		}, n.EventuallyTimeout).Should(Equal(expectedVal))
	}
}

// send transactions, stop orderering server, then start peers to ensure they received blcoks via state transfer
func sendTransactionsAndSyncUpPeers(network *nwo.Network, orderer *nwo.Orderer, basePeer *nwo.Peer, peersToSyncUp []*nwo.Peer, channelName string,
	ordererProcess *ifrit.Process, ordererRunner *ginkgomon.Runner,
	peerProcesses map[string]ifrit.Process, peerRunners map[string]*ginkgomon.Runner) {

	By("create transactions")
	runTransactions(network, orderer, basePeer, "mycc", channelName)
	basePeerLedgerHeight := nwo.GetLedgerHeight(network, basePeer, channelName)

	By("stop orderer")
	(*ordererProcess).Signal(syscall.SIGTERM)
	Eventually((*ordererProcess).Wait(), network.EventuallyTimeout).Should(Receive())
	*ordererProcess = nil

	By("start the peers contained in the peersToSyncUp list")
	startPeers(network, peersToSyncUp, peerProcesses, peerRunners, true)

	By("ensure the peers are synced up")
	assertPeersLedgerHeight(network, orderer, peersToSyncUp, basePeerLedgerHeight, channelName)

	By("restart orderer")
	orderer = network.Orderer("orderer")
	ordererRunner = network.OrdererRunner(orderer)
	*ordererProcess = ifrit.Invoke(ordererRunner)
	Eventually((*ordererProcess).Ready(), network.EventuallyTimeout).Should(BeClosed())
}
