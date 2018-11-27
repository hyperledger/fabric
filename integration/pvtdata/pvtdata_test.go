/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"gopkg.in/yaml.v2"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
)

var _ bool = Describe("PrivateData", func() {
	// at the beginning of each test under this block, we have 2 collections defined:
	// 1. collectionMarbles - Org1 and Org2 are have access to this collection
	// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
	// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles[1]
	// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails[2]
	Describe("reconciliation", func() {
		var (
			testDir       string
			client        *docker.Client
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			var err error
			testDir, err = ioutil.TempDir("", "e2e-pvtdata")
			Expect(err).NotTo(HaveOccurred())

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			configBytes, err := ioutil.ReadFile(filepath.Join("testdata", "network.yaml"))
			Expect(err).NotTo(HaveOccurred())

			var networkConfig *nwo.Config
			err = yaml.Unmarshal(configBytes, &networkConfig)
			Expect(err).NotTo(HaveOccurred())

			network = nwo.New(networkConfig, testDir, client, 35000+1000*GinkgoParallelNode(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			orderer = network.Orderer("orderer")
			network.CreateAndJoinChannel(orderer, "testchannel")
			network.UpdateChannelAnchors(orderer, "testchannel")

			expectedPeers = []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0"),
			}

			By("verifying membership")
			verifyMembership(network, expectedPeers, "testchannel")

			By("installing and instantiating chaincode on all peers")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json")}
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("invoking initMarble function of the chaincode")
			invokeChaincode(network, "org1", "peer0", "marblesp", `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, "testchannel", orderer)

			By("waiting for block to propagate")
			waitUntilAllPeersSameLedgerHeight(network, expectedPeers, "testchannel", getLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"))
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

		It("verify private data reconciliation when adding a new org to collection config", func() {
			// after the upgrade the collections will be updated as follows:
			// 1. collectionMarbles - Org1, Org2 and Org3 have access to this collection
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
			// the change - org3 was added to collectionMarbles
			By("verify access of initial setup")
			verifyAccessInitialSetup(network)

			By("upgrading chaincode in order to update collections config")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "2.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config2.json")}
			nwo.UpgradeChaincode(network, "testchannel", orderer, chaincode)

			By("invoking initMarble function of the chaincode")
			invokeChaincode(network, "org2", "peer0", "marblesp", `{"Args":["initMarble","marble2","yellow","53","jerry","22"]}`, "testchannel", orderer)

			By("waiting for block to propagate")
			waitUntilAllPeersSameLedgerHeight(network, expectedPeers, "testchannel", getLedgerHeight(network, network.Peer("org2", "peer0"), "testchannel"))

			By("verifying access as defined in collection config")
			peerList := []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0")}
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarble","marble2"]}`},
				peerList,
				`{"docType":"marble","name":"marble2","color":"yellow","size":53,"owner":"jerry"}`)

			peerList = []*nwo.Peer{
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0")}
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble2"]}`},
				peerList,
				`{"docType":"marblePrivateDetails","name":"marble2","price":22}`)

			By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble2"]}`},
				network.Peer("org1", "peer0"),
				"private data matching public hash version is not available")

			By("querying collectionMarbles by peer0.org3, make sure marble1 that was created before adding peer0.org3 to the config was reconciled")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarble","marble1"]}`},
				[]*nwo.Peer{network.Peer("org3", "peer0")},
				`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)
		})

		It("verify private data reconciliation when joining a new peer in an org that belongs to collection config", func() {
			By("verify access of initial setup")
			verifyAccessInitialSetup(network)

			By("peer1.org2 joins the channel")
			org2peer1 := network.Peer("org2", "peer1")
			network.JoinChannel("testchannel", orderer, org2peer1)
			org2peer1.Channels = append(org2peer1.Channels, &nwo.PeerChannel{Name: "testchannel", Anchor: false})

			ledgerHeight := getLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel")

			By("fetch latest blocks to peer1.org2")
			sess, err := network.PeerAdminSession(org2peer1, commands.ChannelFetch{
				Block:      "newest",
				ChannelID:  "testchannel",
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				OutputFile: filepath.Join(testDir, "newest_block.pb")})

			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

			By("install chaincode on peer1.org2 to be able to query it")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json")}

			nwo.InstallChaincode(network, chaincode, org2peer1)

			expectedPeers = []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org2", "peer1"),
				network.Peer("org3", "peer0")}

			By("verifying membership")
			verifyMembership(network, expectedPeers, "testchannel", "marblesp")

			By("make sure all peers have the same ledger height")
			waitUntilAllPeersSameLedgerHeight(network, expectedPeers, "testchannel", getLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"))

			By("verify peer1.org2 got the private data that was created historically")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarble","marble1"]}`},
				[]*nwo.Peer{org2peer1},
				`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)

			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				[]*nwo.Peer{org2peer1},
				`{"docType":"marblePrivateDetails","name":"marble1","price":99}`)
		})
	})
})

func verifyAccessInitialSetup(network *nwo.Network) {
	By("verifying access as defined in collection config")
	peerList := []*nwo.Peer{
		network.Peer("org1", "peer0"),
		network.Peer("org2", "peer0")}
	verifyAccess(
		network,
		commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "marblesp",
			Ctor:      `{"Args":["readMarble","marble1"]}`},
		peerList,
		`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)

	peerList = []*nwo.Peer{
		network.Peer("org2", "peer0"),
		network.Peer("org3", "peer0")}
	verifyAccess(
		network,
		commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "marblesp",
			Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
		peerList,
		`{"docType":"marblePrivateDetails","name":"marble1","price":99}`)

	By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
	verifyAccessFailed(
		network,
		commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "marblesp",
			Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
		network.Peer("org1", "peer0"),
		"private data matching public hash version is not available")

	By("querying collectionMarbles by peer0.org3, shouldn't have access")
	verifyAccessFailed(
		network,
		commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "marblesp",
			Ctor:      `{"Args":["readMarble","marble1"]}`,
		},
		network.Peer("org3", "peer0"),
		"Failed to get state for marble1")
}

func invokeChaincode(n *nwo.Network, org string, peer string, ccname string, args string, channel string, orderer *nwo.Orderer) {
	sess, err := n.PeerUserSession(n.Peer(org, peer), "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      ccname,
		Ctor:      args,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer(org, peer), nwo.ListenPort),
		},

		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func getLedgerHeight(n *nwo.Network, peer *nwo.Peer, channelName string) int {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChannelInfo{
		ChannelID: channelName,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
	var channelInfo = common.BlockchainInfo{}
	json.Unmarshal([]byte(channelInfoStr), &channelInfo)
	return int(channelInfo.Height)
}

func waitUntilAllPeersSameLedgerHeight(n *nwo.Network, peers []*nwo.Peer, channelName string, height int) {
	for _, peer := range peers {
		EventuallyWithOffset(1, func() int {
			return getLedgerHeight(n, peer, channelName)
		}, n.EventuallyTimeout*3).Should(Equal(height))
	}
}

// this function checks that each peer discovered all other peers in the network
func verifyMembership(n *nwo.Network, expectedPeers []*nwo.Peer, channelName string, chaincodes ...string) {
	expectedDiscoveredPeers := make([]nwo.DiscoveredPeer, 0, len(expectedPeers))
	for _, peer := range expectedPeers {
		expectedDiscoveredPeers = append(expectedDiscoveredPeers, n.DiscoveredPeer(peer, chaincodes...))
	}
	for _, peer := range expectedPeers {
		Eventually(nwo.DiscoverPeers(n, peer, "User1", channelName), time.Minute).Should(ConsistOf(expectedDiscoveredPeers))
	}
}

func verifyAccess(n *nwo.Network, chaincodeQueryCmd commands.ChaincodeQuery, peers []*nwo.Peer, expected string) {
	for _, peer := range peers {
		sess, err := n.PeerUserSession(peer, "User1", chaincodeQueryCmd)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expected))
	}
}

func verifyAccessFailed(n *nwo.Network, chaincodeQueryCmd commands.ChaincodeQuery, peer *nwo.Peer, expectedFailureMessage string) {
	sess, err := n.PeerUserSession(peer, "User1", chaincodeQueryCmd)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say(expectedFailureMessage))
}
