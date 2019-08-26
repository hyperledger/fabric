/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

const (
	chaincodePathWithNoIndex = "github.com/hyperledger/fabric/integration/chaincode/marbles/cmd"
	chaincodePathWithIndex   = "github.com/hyperledger/fabric/integration/chaincode/marbles/cmdwithindexspec"
)

var _ = Describe("CouchDB indexes", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		orderer *nwo.Orderer
		process ifrit.Process

		couchAddr    string
		couchDB      *runner.CouchDB
		couchProcess ifrit.Process

		legacyChaincode       nwo.Chaincode
		newlifecycleChaincode nwo.Chaincode
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "ledger")
		Expect(err).NotTo(HaveOccurred())
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()

		// configure only one of four peers (Org1, peer0) to use couchdb.
		// Note that we do not support a channel with mixed DBs.
		// However, for testing, it would be fine to use couchdb for one
		// peer and sending all the couchdb related test queries to this peer
		couchDB = &runner.CouchDB{}
		couchProcess = ifrit.Invoke(couchDB)
		Eventually(couchProcess.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(couchProcess.Wait()).ShouldNot(Receive())
		couchAddr = couchDB.Address()
		peer := network.Peer("Org1", "peer0")
		core := network.ReadPeerConfig(peer)
		core.Ledger.State.StateDatabase = "CouchDB"
		core.Ledger.State.CouchDBConfig.CouchDBAddress = couchAddr
		network.WritePeerConfig(peer, core)

		// start the network
		network.Bootstrap()
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		orderer = network.Orderer("orderer")
		network.CreateAndJoinChannel(orderer, "testchannel")
		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

		legacyChaincode = nwo.Chaincode{
			Name:        "marbles",
			Version:     "0.0",
			Path:        chaincodePathWithIndex,
			Ctor:        `{"Args":[]}`,
			Policy:      `OR ('Org1MSP.member','Org2MSP.member')`,
			PackageFile: filepath.Join(testDir, "marbles_legacy.tar.gz"),
		}

		newlifecycleChaincode = nwo.Chaincode{
			Name:            "marbles",
			Version:         "0.0",
			Path:            chaincodePathWithIndex,
			Lang:            "golang",
			PackageFile:     filepath.Join(testDir, "marbles.tar.gz"),
			SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			Label:           "marbles",
		}
	})

	AfterEach(func() {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		couchProcess.Signal(syscall.SIGTERM)
		Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		network.Cleanup()
		os.RemoveAll(testDir)
	})

	When("chaincode is installed and instantiated via legacy lifecycle", func() {
		It("creates indexes", func() {
			nwo.PackageChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))
			nwo.InstallChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))
			nwo.InstantiateChaincodeLegacy(network, "testchannel", orderer, legacyChaincode, network.Peer("Org1", "peer0"), network.Peers...)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_indexed")
			verifyIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})

	When("chaincode is defined and installed via new lifecycle", func() {
		It("creates indexes", func() {
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, newlifecycleChaincode, network.Peers...)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_indexed")
			verifyIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})

	When("chaincode is installed and instantiated via legacy lifecycle and then defined and installed via new lifecycle", func() {
		BeforeEach(func() {
			legacyChaincode.Path = chaincodePathWithNoIndex
			newlifecycleChaincode.Path = chaincodePathWithIndex
		})

		It("create indexes from the new lifecycle package", func() {
			By("instantiating and installing legacy chaincode")
			nwo.PackageChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))
			nwo.InstallChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))
			nwo.InstantiateChaincodeLegacy(network, "testchannel", orderer, legacyChaincode, network.Peer("Org1", "peer0"), network.Peers...)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_not_indexed")
			verifyIndexDoesNotExist(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")

			By("installing and defining chaincode using new lifecycle")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, newlifecycleChaincode)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_indexed")
			verifyIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})

	When("chaincode is instantiated via legacy lifecycle, then defined and installed via new lifecycle and, finally installed via legacy lifecycle", func() {
		BeforeEach(func() {
			legacyChaincode.Path = chaincodePathWithIndex
			newlifecycleChaincode.Path = chaincodePathWithNoIndex
		})

		It("does not create indexes upon final installation of legacy chaincode", func() {
			By("instantiating legacy chaincode")
			// lscc requires the chaincode to be installed before a instantiate transaction can be simulated
			// doing so in Org1.peer1 so that chaincode is not installed on "Org1.peer0" i.e., only instantiated
			// via legacy lifecycle
			nwo.PackageChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer1"))
			nwo.InstallChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer1"))
			nwo.InstantiateChaincodeLegacy(network, "testchannel", orderer, legacyChaincode, network.Peer("Org1", "peer1"), network.Peers...)

			By("installing and defining chaincode using new lifecycle")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, newlifecycleChaincode)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_not_indexed")

			By("installing legacy chaincode on Org1.peer0")
			nwo.InstallChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))

			By("verifying that the index should not have been created on (Org1, peer0) - though the legacy package contains indexes")
			verifyIndexDoesNotExist(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})

	When("chaincode is installed using legacy lifecycle, then defined and installed using new lifecycle", func() {
		BeforeEach(func() {
			legacyChaincode.Path = chaincodePathWithIndex
			newlifecycleChaincode.Path = chaincodePathWithNoIndex
		})

		It("does not use legacy package to create indexes", func() {
			By("installing legacy chaincode (with an index included)")
			nwo.PackageChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))
			nwo.InstallChaincodeLegacy(network, legacyChaincode, network.Peer("Org1", "peer0"))

			By("installing and defining chaincode (without an index included) using new lifecycle")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, newlifecycleChaincode)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_not_indexed")

			By("verifying that the index should not have been created - though the legacy package contains indexes")
			verifyIndexDoesNotExist(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})
})

func initMarble(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName, marbleName string) {
	By("invoking initMarble function of the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      ccName,
		Ctor:      prepareChaincodeInvokeArgs("initMarble", marbleName, "blue", "35", "tom"),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func prepareChaincodeInvokeArgs(args ...string) string {
	m, err := json.Marshal(map[string][]string{
		"Args": args,
	})
	Expect(err).NotTo(HaveOccurred())
	return string(m)
}

func verifyIndexExists(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifyIndexPresence(n, channel, orderer, peer, ccName, true)
}

func verifyIndexDoesNotExist(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifyIndexPresence(n, channel, orderer, peer, ccName, false)
}

func verifyIndexPresence(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string, expectIndexPresent bool) {
	By("invoking queryMarbles function with a user constructed query that requires an index due to a sort")
	query := `{
		"selector":{
			"docType":{
				"$eq":"marble"
			},
			"owner":{
				"$eq":"tom"
			},
			"size":{
				"$gt":0
			}
		},
		"fields":["docType","owner","size"],
		"sort":[{"size":"desc"}],
		"use_index":"_design/indexSizeSortDoc"
	}`
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Name:      ccName,
		Ctor:      prepareChaincodeInvokeArgs("queryMarbles", query),
	})
	Expect(err).NotTo(HaveOccurred())
	if expectIndexPresent {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say("Error:no_usable_index"))
	}
}
