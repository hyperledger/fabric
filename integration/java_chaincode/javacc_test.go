/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"io/ioutil"
	"os"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
)

var _ = Describe("Java Chaincode EndToEnd Suite", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-javacc")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), testDir, client, 40000+1000*GinkgoParallelNode(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready()).Should(BeClosed())
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGKILL)
			Eventually(process.Wait(), time.Minute).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	PIt("supports java chaincode from a gradle project", func() {
		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "../chaincode/java/simple/gradle/",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
			Lang:    "java",
		}

		By("getting the orderer by name")
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		network.CreateAndJoinChannel(orderer, "testchannel")

		By("deploying the chaincode")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("getting the client peer by name")
		peer := network.Peer("Org1", "peer0")

		RunQueryInvokeQuery(network, orderer, peer)
	})

	PIt("supports private data in java chaincode", func() {
		chaincode = nwo.Chaincode{
			Name:              "mycc",
			Version:           "0.0",
			Path:              "../chaincode/java/simple_pvtdata/gradle/",
			Ctor:              `{"Args":["init","a","100","b","200"]}`,
			Policy:            `OR ('Org1MSP.member','Org2MSP.member')`,
			Lang:              "java",
			CollectionsConfig: "testdata/collection_config.json",
		}

		By("getting the orderer by name")
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		network.CreateAndJoinChannel(orderer, "testchannel")

		By("deploying the chaincode")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("getting the client peer by name")
		peer := network.Peer("Org1", "peer0")

		RunQueryInvokeQuery(network, orderer, peer)
	})
})

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) {
	By("invoking the chaincode - 1")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode - 1")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))

	By("invoking the chaincode - 2")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode - 2")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","b"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("220"))
}
