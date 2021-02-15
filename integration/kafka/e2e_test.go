/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("basic kafka network with 2 orgs", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "kafka-e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicKafka(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
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

	It("executes a basic kafka network with 2 orgs", func() {
		chaincodePath, err := filepath.Abs("../chaincode/module")
		Expect(err).NotTo(HaveOccurred())

		// use these two variants of the same chaincode to ensure we test
		// the golang docker build for both module and gopath chaincode
		chaincode := nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            chaincodePath,
			Lang:            "golang",
			PackageFile:     filepath.Join(testDir, "modulecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_module_chaincode",
		}

		network.CreateAndJoinChannel(network.Orderer("orderer"), "testchannel")
		nwo.EnableCapabilities(
			network,
			"testchannel",
			"Application", "V2_0",
			network.Orderer("orderer"),
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)
		nwo.DeployChaincode(
			network,
			"testchannel",
			network.Orderer("orderer"),
			chaincode,
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)
		RunQueryInvokeQuery(
			network,
			"testchannel",
			network.Orderer("orderer"),
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)
	})
})

func RunQueryInvokeQuery(n *nwo.Network, channel string, orderer *nwo.Orderer, peers ...*nwo.Peer) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peers[0], "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("100"))

	var peerAddrs []string
	for _, p := range peers {
		peerAddrs = append(peerAddrs, n.PeerAddress(p, nwo.ListenPort))
	}
	sess, err = n.PeerUserSession(peers[0], "User1", commands.ChaincodeInvoke{
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		Name:          "mycc",
		Ctor:          `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: peerAddrs,
		WaitForEvent:  true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	sess, err = n.PeerUserSession(peers[0], "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))
}
