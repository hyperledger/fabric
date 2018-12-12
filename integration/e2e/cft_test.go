/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("EndToEnd Crash Fault Tolerance", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		peer      *nwo.Peer

		peerProc, ordererProc, o1Proc ifrit.Process
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
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
		}
	})

	AfterEach(func() {
		if o1Proc != nil {
			o1Proc.Signal(syscall.SIGTERM)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if ordererProc != nil {
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if peerProc != nil {
			peerProc.Signal(syscall.SIGTERM)
			Eventually(peerProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	fetchLatestBlock := func(targetOrderer *nwo.Orderer, blockFile string) {
		c := commands.ChannelFetch{
			ChannelID:  "testchannel",
			Block:      "newest",
			OutputFile: blockFile,
		}
		if targetOrderer != nil {
			c.Orderer = network.OrdererAddress(targetOrderer, nwo.ListenPort)
		}
		sess, err := network.PeerAdminSession(peer, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
	}

	When("orderer stops and restarts", func() {
		It("keeps network up and running", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer1")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")

			network.GenerateConfigTree()
			network.Bootstrap()

			o1Runner := network.OrdererRunner(o1)
			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)
			peerGroup := network.PeerGroupRunner()

			o1Proc = ifrit.Invoke(o1Runner)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(ordererProc.Ready()).Should(BeClosed())
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			By("performing operation with orderer1")
			network.CreateAndJoinChannel(o1, "testchannel")

			By("killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			By("performing operations with running orderer")
			nwo.DeployChaincode(network, "testchannel", o2, chaincode)

			By("restarting orderer1")
			o1Runner = network.OrdererRunner(o1)
			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready()).Should(BeClosed())

			By("executing transaction with restarted orderer")
			RunQueryInvokeQuery(network, o1, peer, "testchannel")

			fetchLatestBlock(o1, blockFile1)
			fetchLatestBlock(o2, blockFile2)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			Expect(b1.Header.Bytes()).To(Equal(b2.Header.Bytes()))
		})
	})

	When("an orderer is behind the latest snapshot on leader", func() {
		It("catches up using the block stored in snapshot", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")

			peer = network.Peer("Org1", "peer1")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")

			network.GenerateConfigTree()
			network.Bootstrap()

			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)
			peerGroup := network.PeerGroupRunner()

			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			network.CreateAndJoinChannel(o2, "testchannel")
			nwo.DeployChaincode(network, "testchannel", o2, chaincode)

			for i := 1; i <= 6; i++ {
				RunInvoke(network, o2, peer, "testchannel")
				RunQuery(network, o2, peer, "testchannel", 100-i*10)
			}

			o2SnapDir := path.Join(network.RootDir, "orderers", o2.ID(), "etcdraft", "snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o2SnapDir, "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}).Should(Equal(1))

			ordererProc.Signal(syscall.SIGKILL)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			o1Runner := network.OrdererRunner(o1)
			orderers = grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())

			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready()).Should(BeClosed())

			o1SnapDir := path.Join(network.RootDir, "orderers", o1.ID(), "etcdraft", "snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(o1SnapDir)
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(2))
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o1SnapDir, "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(1))

			RunInvoke(network, o1, peer, "testchannel")
			RunQuery(network, o1, peer, "testchannel", 30)

			fetchLatestBlock(o1, blockFile1)
			fetchLatestBlock(o2, blockFile2)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			Expect(b1.Header.Bytes()).To(Equal(b2.Header.Bytes()))
		})
	})
})

func RunInvoke(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func RunQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string, expect int) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(strconv.Itoa(expect)))
}
