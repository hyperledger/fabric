/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sbe

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("SBE_E2E", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		chaincode                   nwo.Chaincode
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
		tempDir                     string
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e_sbe")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:              "mycc",
			Version:           "0.0",
			Path:              "github.com/hyperledger/fabric/integration/chaincode/keylevelep/cmd",
			Ctor:              `{"Args":["init"]}`,
			CollectionsConfig: "testdata/collection_config.json",
		}

		tempDir, err = ioutil.TempDir("", "sbe")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(tempDir)
		os.RemoveAll(testDir)
	})

	Describe("basic etcdraft network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
		})

		It("executes a basic etcdraft network with 2 orgs and SBE checks", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

			By("deploying the chaincode")
			nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)

			By("deploying a second instance of the chaincode")
			chaincode.Name = "mycc2"
			nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)

			RunSBE(network, orderer, "pub")
			RunSBE(network, orderer, "priv")
		})

		It("executes a basic etcdraft network with 2 orgs and SBE checks with _lifecycle", func() {
			chaincode = nwo.Chaincode{
				Name:              "mycc",
				Version:           "0.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/keylevelep/cmd",
				Lang:              "golang",
				PackageFile:       filepath.Join(tempDir, "simplecc.tar.gz"),
				Ctor:              `{"Args":["init"]}`,
				SignaturePolicy:   `OR('Org1MSP.member','Org2MSP.member')`,
				Sequence:          "1",
				InitRequired:      true,
				Label:             "my_simple_chaincode",
				CollectionsConfig: "testdata/collection_config.json",
			}

			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

			By("enabling 2.0 application capabilities")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

			By("deploying the chaincode")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("deploying a second instance of the chaincode")
			chaincode.Name = "mycc2"
			chaincode.PackageFile = filepath.Join(tempDir, "simplecc2.tar.gz")
			chaincode.Label = "my_other_simple_chaincode"
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			RunSBE(network, orderer, "pub")
			RunSBE(network, orderer, "priv")
		})
	})
})

func RunSBE(n *nwo.Network, orderer *nwo.Orderer, mode string) {
	peerOrg1 := n.Peer("Org1", "peer0")
	peerOrg2 := n.Peer("Org2", "peer0")

	By("org1 initializes the key")
	sess, err := n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "` + mode + `", "foo"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	syncLedgerHeights(n, peerOrg1, peerOrg2)

	By("org2 checks that setting the value was successful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("foo"))

	By("org1 adds org1 to the state-based ep of a key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["addorgs", "` + mode + `", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("checking that the modification succeeded through listing the orgs in the ep")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("Org1MSP"))

	By("org1 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "` + mode + `", "val1"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	syncLedgerHeights(n, peerOrg1, peerOrg2)

	By("org2 checks that setting the value was successful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	By("org2 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "` + mode + `", "val2"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

	By("org2 checks that setting the value was not successful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	syncLedgerHeights(n, peerOrg2, peerOrg1)

	By("org1 adds org2 to the ep of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["addorgs", "` + mode + `", "Org2MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("org1 lists the orgs of the ep to check that both org1 and org2 are there")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	orgs := [2]string{"Org1MSP", "Org2MSP"}
	orgsList, err := json.Marshal(orgs)
	Expect(err).NotTo(HaveOccurred())
	Expect(sess).To(gbytes.Say(string(orgsList)))

	syncLedgerHeights(n, peerOrg1, peerOrg2)

	By("org2 sets the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "` + mode + `", "val3"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

	By("org2 checks that seting the value was not successful by reading it")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val1"))

	syncLedgerHeights(n, peerOrg2, peerOrg1)

	By("org1 and org2 set the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["setval", "` + mode + `", "val4"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("org1 checks that setting the value was successful by reading it")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("val4"))

	syncLedgerHeights(n, peerOrg1, peerOrg2)

	By("org2 deletes org1 from the ep of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["delorgs", "` + mode + `", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

	By("org2 lists the orgs of the key to check that deleting org1 did not succeed")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(string(orgsList)))

	syncLedgerHeights(n, peerOrg2, peerOrg1)

	By("org1 and org2 delete org1 from the ep of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["delorgs", "` + mode + `", "Org1MSP"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("org2 lists the orgs of the key's ep to check that removing org1 from the ep was successful")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["listorgs", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("Org2MSP"))

	By("org2 uses cc2cc invocation to set the value of the key")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc2",
		Ctor:      `{"Args":["cc2cc", "testchannel", "mycc", "setval", "` + mode + `", "cc2cc_org2"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg2, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("org2 reads the value of the key to check that setting it was successful")
	sess, err = n.PeerUserSession(peerOrg2, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("cc2cc_org2"))

	syncLedgerHeights(n, peerOrg2, peerOrg1)

	By("org1 uses cc2cc to set the value of the key")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc2",
		Ctor:      `{"Args":["cc2cc", "testchannel", "mycc", "setval", "` + mode + `", "cc2cc_org1"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peerOrg1, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

	By("org1 reads the value of the key to check that setting it was not successful")
	sess, err = n.PeerUserSession(peerOrg1, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["getval", "` + mode + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("cc2cc_org2"))
}

func getLedgerHeight(n *nwo.Network, peer *nwo.Peer, channelName string) int {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChannelInfo{
		ChannelID: channelName,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
	channelInfo := common.BlockchainInfo{}
	err = json.Unmarshal([]byte(channelInfoStr), &channelInfo)
	Expect(err).NotTo(HaveOccurred())
	return int(channelInfo.Height)
}

func syncLedgerHeights(n *nwo.Network, peer1 *nwo.Peer, peer2 *nwo.Peer) {
	// get height from peer1
	height := getLedgerHeight(n, peer1, "testchannel")
	// wait for same height on peer2
	Eventually(func() int {
		return getLedgerHeight(n, peer2, "testchannel")
	}, n.EventuallyTimeout).Should(Equal(height))
}
