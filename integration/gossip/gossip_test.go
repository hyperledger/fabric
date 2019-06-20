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
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Gossip Test", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process
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
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	PDescribe("State transfer test", func() {
		var (
			ordererProcess ifrit.Process
			peerProcesses  = map[string]ifrit.Process{}
		)

		BeforeEach(func() {
			network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)

			network.GenerateConfigTree()
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

		It("solo network with 2 orgs, 2 peers each, should sync from the peer if no orderer available", func() {
			orderer := network.Orderer("orderer")
			ordererRunner := network.OrdererRunner(orderer)
			ordererProcess = ifrit.Invoke(ordererRunner)

			peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
			peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

			for _, peer := range []*nwo.Peer{peer0Org1, peer1Org1, peer0Org2, peer1Org2} {
				runner := network.PeerRunner(peer)
				peerProcesses[peer.ID()] = ifrit.Invoke(runner)
			}

			channelName := "testchannel"
			network.CreateChannel(channelName, orderer, peer0Org1)
			network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

			nwo.DeployChaincodeLegacy(network, channelName, orderer, chaincode, peer0Org1)
			network.UpdateChannelAnchors(orderer, channelName)

			for _, peer := range []*nwo.Peer{peer0Org1, peer1Org1, peer0Org2, peer1Org2} {
				Eventually(func() int {
					return nwo.GetLedgerHeight(network, peer, channelName)
				}).Should(BeNumerically(">=", 2))
			}

			By("stop peers to make sure they cannot get blocks from orderer")
			for id, proc := range peerProcesses {
				if id == peer0Org1.ID() {
					continue
				}
				proc.Signal(syscall.SIGTERM)
				Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
				delete(peerProcesses, id)
			}

			By("create transactions")
			for i := 0; i < 10; i++ {
				sess, err := network.PeerUserSession(peer0Org1, "User1", commands.ChaincodeInvoke{
					ChannelID: channelName,
					Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
					Name:      "mycc",
					Ctor:      `{"Args":["invoke","a","b","10"]}`,
					PeerAddresses: []string{
						network.PeerAddress(peer0Org1, nwo.ListenPort),
					},
					WaitForEvent: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
			}

			peer0LedgerHeight := nwo.GetLedgerHeight(network, peer0Org1, channelName)

			By("turning down ordering service")
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			ordererProcess = nil

			By("wait for rest of the peers to join and startup")
			for _, peer := range []*nwo.Peer{peer1Org1, peer0Org2, peer1Org2} {
				runner := network.PeerRunner(peer, fmt.Sprint("CORE_PEER_GOSSIP_STATE_CHECKINTERVAL=200ms"),
					fmt.Sprint("FABRIC_LOGGING_SPEC=info:gossip.state=debug"),
				)
				peerProcesses[peer.ID()] = ifrit.Invoke(runner)
			}

			By("Make sure peers are synced up")
			for _, peer := range []*nwo.Peer{peer1Org1, peer0Org2, peer1Org2} {
				Eventually(func() int {
					return nwo.GetLedgerHeight(network, peer, channelName)
				}, time.Second*10).Should(Equal(peer0LedgerHeight))
			}
		})
	})
})
