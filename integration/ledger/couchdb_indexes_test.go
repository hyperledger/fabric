/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

const (
	chaincodePathWithNoIndex = "github.com/hyperledger/fabric/integration/chaincode/marbles/cmd"
	chaincodePathWithIndex   = "github.com/hyperledger/fabric/integration/chaincode/marbles/cmdwithindexspec"
	chaincodePathWithIndexes = "github.com/hyperledger/fabric/integration/chaincode/marbles/cmdwithindexspecs"
)

var (
	filesWithIndex = map[string]string{
		"../chaincode/marbles/cmdwithindexspec/META-INF/statedb/couchdb/indexes/indexSizeSortDoc.json": "metadata/statedb/couchdb/indexes/indexSizeSortDoc.json",
	}

	filesWithIndices = map[string]string{
		"../chaincode/marbles/cmdwithindexspecs/META-INF/statedb/couchdb/indexes/indexSizeSortDoc.json":  "metadata/statedb/couchdb/indexes/indexSizeSortDoc.json",
		"../chaincode/marbles/cmdwithindexspecs/META-INF/statedb/couchdb/indexes/indexColorSortDoc.json": "metadata/statedb/couchdb/indexes/indexColorSortDoc.json",
	}
)

var _ = Describe("CouchDB indexes", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		orderer                     *nwo.Orderer
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process

		couchAddr    string
		couchDB      *runner.CouchDB
		couchProcess ifrit.Process

		chaincode nwo.Chaincode
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "ledger")
		Expect(err).NotTo(HaveOccurred())
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.FullEtcdRaft(), testDir, client, StartPort(), components)

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		network.ExternalBuilders = append(network.ExternalBuilders, fabricconfig.ExternalBuilder{
			Path:                 filepath.Join(cwd, "..", "externalbuilders", "golang"),
			Name:                 "external-golang",
			PropagateEnvironment: []string{"GOPATH", "GOCACHE", "GOPROXY", "HOME", "PATH"},
		})

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

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		By("setting up the channel")
		orderer = network.Orderer("orderer")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

		chaincode = nwo.Chaincode{
			Name:            "marbles",
			Version:         "0.0",
			Path:            components.Build(chaincodePathWithIndex),
			Lang:            "binary",
			CodeFiles:       filesWithIndex,
			PackageFile:     filepath.Join(testDir, "marbles.tar.gz"),
			SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			Label:           "marbles",
		}
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

		couchProcess.Signal(syscall.SIGTERM)
		Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		network.Cleanup()
		os.RemoveAll(testDir)
	})

	When("chaincode is deployed via new lifecycle (using the docker chaincode build) ", func() {
		BeforeEach(func() {
			chaincode.Path = chaincodePathWithIndex
			chaincode.Lang = "golang"
		})

		It("creates indexes", func() {
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode, network.Peers...)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_indexed")
			verifySizeIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
		})
	})

	When("chaincode is defined and installed via new lifecycle and then upgraded with an additional index", func() {
		It("creates indexes", func() {
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode, network.Peers...)
			initMarble(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles", "marble_indexed")
			verifySizeIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
			verifyColorIndexDoesNotExist(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")

			By("upgrading the chaincode to include an additional index")
			chaincode.Sequence = "2"
			chaincode.CodeFiles = filesWithIndices
			chaincode.PackageFile = filepath.Join(testDir, "marbles-two-indexes.tar.gz")
			chaincode.Label = "marbles-two-indexes"

			nwo.PackageChaincodeBinary(chaincode)
			nwo.InstallChaincode(network, chaincode, network.Peers...)
			nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.Peers...)
			nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), network.Peers...)
			nwo.CommitChaincode(network, "testchannel", orderer, chaincode, network.Peers[0], network.Peers...)

			verifySizeIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
			verifyColorIndexExists(network, "testchannel", orderer, network.Peer("Org1", "peer0"), "marbles")
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

func verifySizeIndexExists(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifySizeIndexPresence(n, channel, orderer, peer, ccName, true)
}

func verifySizeIndexDoesNotExist(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifySizeIndexPresence(n, channel, orderer, peer, ccName, false)
}

func verifySizeIndexPresence(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string, expectIndexPresent bool) {
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
	verifyIndexPresence(n, channel, orderer, peer, ccName, expectIndexPresent, query)
}

func verifyColorIndexExists(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifyColorIndexPresence(n, channel, orderer, peer, ccName, true)
}

func verifyColorIndexDoesNotExist(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string) {
	verifyColorIndexPresence(n, channel, orderer, peer, ccName, false)
}

func verifyColorIndexPresence(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string, expectIndexPresent bool) {
	query := `{
		"selector":{
			"docType":{
				"$eq":"marble"
			},
			"owner":{
				"$eq":"tom"
			},
			"color":{
				"$eq":"blue"
			}
		},
		"fields":["docType","owner","size"],
		"sort":[{"color":"desc"}],
		"use_index":"_design/indexColorSortDoc"
	}`
	verifyIndexPresence(n, channel, orderer, peer, ccName, expectIndexPresent, query)
}

func verifyIndexPresence(n *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer, ccName string, expectIndexPresent bool, indexQuery string) {
	By("invoking queryMarbles function with a user constructed query that requires an index due to a sort")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID:     channel,
		Name:          ccName,
		Ctor:          prepareChaincodeInvokeArgs("queryMarbles", indexQuery),
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		PeerAddresses: []string{n.PeerAddress(peer, nwo.ListenPort)},
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
