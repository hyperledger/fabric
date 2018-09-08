/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
)

var _ = Describe("SBE_E2E", func() {
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
			Path:    "github.com/hyperledger/fabric/integration/chaincode/keylevelep/cmd",
			Ctor:    `{"Args":["init"]}`,
		}
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), time.Minute).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("basic solo network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicSolo(), testDir, client, 30000, components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())
		})

		It("executes a basic solo network with 2 orgs and SBE checks", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("deploy first chaincode")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("deploy second chaincode")
			chaincode.Name = "mycc2"
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("getting the client peer by name")

			RunSBE(network, orderer)
		})
	})
})

func RunSBE(n *nwo.Network, orderer *nwo.Orderer) {
	peerOrg1 := n.Peer("Org1", "peer0")
	peerOrg2 := n.Peer("Org2", "peer1")
	By("org1 adds org1 to the state-based ep of a key")
	sess, err := n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["addorgs", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("checking that the modification succeeded through listing the orgs in the ep")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("Org1MSP"))

	By("org1 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "val1"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org1 checks that setting the value was successful by reading it")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	By("org2 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "val2"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org2 checks that setting the value was not succesful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	By("org1 adds org2 to the ep of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["addorgs", "Org2MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org1 lists the orgs of the ep to check that both org1 and org2 are there")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	orgs := [2]string{"Org1MSP", "Org2MSP"}
	orgsList, err := json.Marshal(orgs)
	Expect(err).NotTo(HaveOccurred())
	Expect(sess).To(gbytes.Say(string(orgsList)))

	By("org2 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "val3"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org2 checks that seting the value was not successful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	By("org1 and org2 set the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "val4"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org1 checks that setting the value was successful by reading it")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val4"))

	By("org2 deletes org1 from the ep of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["delorgs", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org2 lists the orgs of the key to check that deleting org1 did not succeed")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(string(orgsList)))

	By("org1 and org2 delete org1 from the ep of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["delorgs", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org2 lists the orgs of the key's ep to check that removing org1 from the ep was successful")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("Org2MSP"))

	By("org2 uses cc2cc invocation to set the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc2",
		Ctor:      `{"Args":["cc2cc", "testchannel", "mycc", "setval", "cc2cc_org2"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org2 reads the value of the key to check that setting it was successful")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("cc2cc_org2"))

	By("org1 uses cc2cc to set the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc2",
		Ctor:      `{"Args":["cc2cc", "testchannel", "mycc", "setval", "cc2cc_org1"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	By("org1 reads the value of the key to check that setting it was not successful")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("cc2cc_org2"))
}
