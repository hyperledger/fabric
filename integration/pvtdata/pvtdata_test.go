/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	yaml "gopkg.in/yaml.v2"
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
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			testDir, network, process, orderer, expectedPeers = initThreeOrgsSetup()

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
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"), expectedPeers...)
		})

		AfterEach(func() {
			testCleanup(testDir, network, process)
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
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org2", "peer0"), "testchannel"), expectedPeers...)

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

			ledgerHeight := nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel")

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
			network.VerifyMembership(expectedPeers, "testchannel", "marblesp")

			By("make sure all peers have the same ledger height")
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"), expectedPeers...)

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

	Describe("collection config BlockToLive is respected", func() {
		var (
			testDir string
			network *nwo.Network
			process ifrit.Process
			orderer *nwo.Orderer
		)
		BeforeEach(func() {
			testDir, network, process, orderer, _ = initThreeOrgsSetup()
		})

		AfterEach(func() {
			testCleanup(testDir, network, process)
		})

		It("verifies private data is purged after BTL has passed and new peer doesn't pull private data that was purged", func() {
			By("installing and instantiating chaincode on all peers")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "short_btl_config.json")}

			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			org2peer0 := network.Peer("org2", "peer0")
			initialLedgerHeight := nwo.GetLedgerHeight(network, org2peer0, "testchannel")

			By("invoking initMarble function of the chaincode to create private data")
			invokeChaincode(network, "org2", "peer0", "marblesp", `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, "testchannel", orderer)

			By("create a block, for private data existence")
			i := 2
			Eventually(func() int {
				invokeChaincode(network, "org2", "peer0", "marblesp", fmt.Sprintf(`{"Args":["initMarble","marble%d","blue%d","3%d","tom","9%d"]}`, i, i, i, i), "testchannel", orderer)
				i++
				return nwo.GetLedgerHeight(network, org2peer0, "testchannel")
			}, network.EventuallyTimeout).Should(BeNumerically(">", initialLedgerHeight))

			By("verify private data exist in peer0.org2")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				[]*nwo.Peer{org2peer0},
				`{"docType":"marblePrivateDetails","name":"marble1","price":99}`)

			By("create 4 more blocks to reach BTL threshold and have marble1 private data purged")
			Eventually(func() int {
				invokeChaincode(network, "org2", "peer0", "marblesp", fmt.Sprintf(`{"Args":["initMarble","marble%d","blue%d","3%d","tom","9%d"]}`, i, i, i, i), "testchannel", orderer)
				i++
				return nwo.GetLedgerHeight(network, org2peer0, "testchannel")
			}, network.EventuallyTimeout).Should(BeNumerically(">", initialLedgerHeight+4))

			By("querying collectionMarbles by peer0.org2, marble1 should still be available")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarble","marble1"]}`},
				[]*nwo.Peer{org2peer0},
				`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)

			By("querying collectionMarblePrivateDetails by peer0.org2, marble1 should have been purged")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				org2peer0,
				"Marble private details does not exist: marble1")

			By("peer1.org2 joins the channel")
			org2peer1 := network.Peer("org2", "peer1")
			network.JoinChannel("testchannel", orderer, org2peer1)
			org2peer1.Channels = append(org2peer1.Channels, &nwo.PeerChannel{Name: "testchannel", Anchor: false})

			By("install chaincode on peer1.org2 to be able to query it")
			nwo.InstallChaincode(network, chaincode, org2peer1)

			By("fetch latest blocks to peer1.org2")
			ledgerHeight := nwo.GetLedgerHeight(network, org2peer0, "testchannel")
			sess, err := network.PeerAdminSession(org2peer1, commands.ChannelFetch{
				Block:      "newest",
				ChannelID:  "testchannel",
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				OutputFile: filepath.Join(testDir, "newest_block.pb")})

			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

			By("wait until peer1.org2 ledger is updated with all txs")
			Eventually(func() int {
				return nwo.GetLedgerHeight(network, org2peer1, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(ledgerHeight))

			By("verify chaincode is instantiated on peer1.org2")
			nwo.EnsureInstantiated(network, "testchannel", "marblesp", "1.0", org2peer1)

			By("query peer1.org2, verify marble1 exist in collectionMarbles and private data doesn't exist")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarble","marble1"]}`},
				[]*nwo.Peer{org2peer1},
				`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)

			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				org2peer0,
				"Marble private details does not exist: marble1")
		})
	})

	Describe("network partition with respect of private data", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer

			peersProcesses map[string]ifrit.Process
			org1peer0      *nwo.Peer
			org2peer0      *nwo.Peer
		)
		BeforeEach(func() {
			peersProcesses = make(map[string]ifrit.Process)
			var err error
			testDir, err = ioutil.TempDir("", "e2e-pvtdata")
			Expect(err).NotTo(HaveOccurred())

			client, err := docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			configBytes, err := ioutil.ReadFile(filepath.Join("testdata", "network.yaml"))
			Expect(err).NotTo(HaveOccurred())

			var networkConfig *nwo.Config
			err = yaml.Unmarshal(configBytes, &networkConfig)
			Expect(err).NotTo(HaveOccurred())

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			members := grouper.Members{
				{Name: "brokers", Runner: network.BrokerGroupRunner()},
				{Name: "orderers", Runner: network.OrdererGroupRunner()},
			}
			networkRunner := grouper.NewOrdered(syscall.SIGTERM, members)
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			org1peer0 = network.Peer("org1", "peer0")
			org2peer0 = network.Peer("org2", "peer0")

			testPeers := []*nwo.Peer{org1peer0, org2peer0}
			for _, peer := range testPeers {
				pr := network.PeerRunner(peer)
				p := ifrit.Invoke(pr)
				peersProcesses[peer.ID()] = p
				Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			orderer = network.Orderer("orderer")
			network.CreateChannel("testchannel", orderer, testPeers[0])
			network.JoinChannel("testchannel", orderer, testPeers...)
			network.UpdateChannelAnchors(orderer, "testchannel")

			expectedPeers = []*nwo.Peer{org1peer0, org2peer0}

			By("verifying membership")
			network.VerifyMembership(expectedPeers, "testchannel")
		})

		AfterEach(func() {
			for _, peerProcess := range peersProcesses {
				if peerProcess != nil {
					peerProcess.Signal(syscall.SIGTERM)
					Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
			testCleanup(testDir, network, process)
		})

		It("verifies private data not distributed when there is network partition", func() {
			By("installing and instantiating chaincode on all peers")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config3.json")}

			nwo.DeployChaincode(network, "testchannel", orderer, chaincode, expectedPeers...)

			By("invoking initMarble function of the chaincode to create private data")
			invokeChaincode(network, "org1", "peer0", "marblesp", `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, "testchannel", orderer)

			// after the upgrade:
			// 1. collectionMarbles - Org1 and Org2 have access to this collection, using readMarble to read from it.
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection - using readMarblePrivateDetails to read from it.
			By("upgrading chaincode in order to update collections config, now org2 and org3 have access to private collection")
			chaincode.Version = "2.0"
			chaincode.CollectionsConfig = filepath.Join("testdata", "collection_configs", "collections_config1.json")
			nwo.UpgradeChaincode(network, "testchannel", orderer, chaincode, expectedPeers...)

			By("stop p0.org2 process")
			process := peersProcesses[org2peer0.ID()]
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
			delete(peersProcesses, org2peer0.ID())

			By("verifying membership")
			expectedPeers = []*nwo.Peer{org1peer0}
			network.VerifyMembership(expectedPeers, "testchannel", "marblesp")

			By("start p0.org3 process")
			org3peer0 := network.Peer("org3", "peer0")
			pr := network.PeerRunner(org3peer0)
			p := ifrit.Invoke(pr)
			peersProcesses[org3peer0.ID()] = p
			Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("join peer0.org3 to the channel")
			network.JoinChannel("testchannel", orderer, org3peer0)

			By("install the chaincode on p0.org3 in order to query it")
			nwo.InstallChaincode(network, chaincode, org3peer0)

			By("fetch latest blocks to peer0.org3")
			ledgerHeight := nwo.GetLedgerHeight(network, org1peer0, "testchannel")
			sess, err := network.PeerAdminSession(org3peer0, commands.ChannelFetch{
				Block:      "newest",
				ChannelID:  "testchannel",
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				OutputFile: filepath.Join(testDir, "newest_block.pb")})

			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

			By("wait until peer0.org3 ledger is updated with all txs")
			Eventually(func() int {
				return nwo.GetLedgerHeight(network, org3peer0, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(ledgerHeight))

			By("verify p0.org3 didn't get private data")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				org3peer0,
				"Failed to get private details for marble1")
		})
	})

	Describe("collection ACL while reading private data", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			testDir, network, process, orderer, expectedPeers = initThreeOrgsSetup()

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
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"), expectedPeers...)
		})

		AfterEach(func() {
			testCleanup(testDir, network, process)
		})

		It("verify that the private data is not readable by non-members", func() {
			// as the member_only_read is set to false, org1-user1 should be able to
			// read from both the collectionMarblePrivateDetails but cannot find the
			// private data.
			By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, should have read access but cannot find the pvtdata")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				network.Peer("org1", "peer0"),
				"private data matching public hash version is not available")

			// after the upgrade the collections will be updated as follows:
			// 1. collectionMarbles - member_only_read is set to true
			// 2. collectionMarblePrivateDetails - member_only_read is set to true
			// no change in the membership but org1-user1 cannot read from
			// collectionMarblePrivateDetails.
			By("upgrading chaincode in order to update collections config")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "2.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config4.json")}
			nwo.UpgradeChaincode(network, "testchannel", orderer, chaincode)

			By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, shouldn't have read access")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`},
				network.Peer("org1", "peer0"),
				"tx creator does not have read access permission")
		})
	})

	Describe("Organization removed from collection", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			testDir, network, process, orderer, expectedPeers = initThreeOrgsSetup()
		})

		AfterEach(func() {
			testCleanup(testDir, network, process)
		})

		It("verifies removed org does not get new data", func() {
			//
			// collection test using LSCC
			//
			By("installing and instantiating chaincode on all peers using LSCC. All three orgs are member of 'collectionMarbles'")
			chaincode := nwo.Chaincode{
				Name:              "marbles-pvtdata",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Label:             "marbles-private",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config2.json"),
			}
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("creating marble1")
			invokeChaincode(network, "org1", "peer0", "marbles-pvtdata", `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, "testchannel", orderer)

			By("waiting for the block to propagate")
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"), expectedPeers...)

			By("verifying that initially all three orgs have private data for marble1")
			peerAllThreeOrgs := []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0"),
			}
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble1"]}`,
				},
				peerAllThreeOrgs,
				`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`,
			)

			By("upgrading chaincode to remove org3 from collectionMarbles")
			chaincode = nwo.Chaincode{
				Name:              "marbles-pvtdata",
				Version:           "1.1",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Label:             "marbles-private",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
			}
			nwo.UpgradeChaincode(network, "testchannel", orderer, chaincode)

			By("creating marble2")
			invokeChaincode(network, "org2", "peer0", "marbles-pvtdata", `{"Args":["initMarble","marble2","yellow","53","jerry","22"]}`, "testchannel", orderer)

			By("waiting for the block to propagate")
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org2", "peer0"), "testchannel"), expectedPeers...)

			By("verifying availability of new private data (marble2) for org1 and org2")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble2"]}`,
				},
				[]*nwo.Peer{
					network.Peer("org1", "peer0"),
					network.Peer("org2", "peer0"),
				},
				`{"docType":"marble","name":"marble2","color":"yellow","size":53,"owner":"jerry"}`,
			)

			By("verifying that marble2 private data does not exist for org3")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble2"]}`,
				},
				network.Peer("org3", "peer0"),
				"Failed to get state for marble2",
			)

			//
			// collection test using _lifecycle
			//
			By("enabling V2_0 capabilities on the channel")
			nwo.EnableV2_0Capabilities(network, "testchannel", orderer, peerAllThreeOrgs...)

			By("deploying chaincode on all peers using _lifecycle. All three orgs are members of 'collectionMarbles'")
			chaincode = nwo.Chaincode{
				Name:              "marbles-pvtdata",
				Version:           "2.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Lang:              "golang",
				PackageFile:       filepath.Join(testDir, "marbles-pvtdata.tar.gz"),
				Label:             "marbles-private-2.0",
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config2.json"),
				Sequence:          "1",
			}
			nwo.DeployChaincodeNewLifecycle(network, "testchannel", orderer, chaincode)

			By("creating marble3")
			invokeChaincode(network, "org1", "peer0", "marbles-pvtdata", `{"Args":["initMarble","marble3","green","75","tom","99"]}`, "testchannel", orderer)

			By("waiting for the block to propagate")
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel"), expectedPeers...)

			By("verifying that initially all three orgs have private data for marble3")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble3"]}`,
				},
				peerAllThreeOrgs,
				`{"docType":"marble","name":"marble3","color":"green","size":75,"owner":"tom"}`,
			)

			By("upgrading chaincode to remove org3 from collectionMarbles")
			chaincode.Sequence = "2"
			chaincode.Policy = `OR ('Org1MSP.member','Org2MSP.member')`
			chaincode.CollectionsConfig = filepath.Join("testdata", "collection_configs", "collections_config1.json")

			maxLedgerHeight := nwo.GetMaxLedgerHeight(network, "testchannel", peerAllThreeOrgs...)
			for _, org := range network.PeerOrgs() {
				nwo.ApproveChaincodeForMyOrgNewLifecycle(network, "testchannel", orderer, chaincode, network.PeersInOrg(org.Name)...)
			}
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", maxLedgerHeight+len(network.PeerOrgs()), peerAllThreeOrgs...)
			nwo.CommitChaincodeNewLifecycle(network, "testchannel", orderer, chaincode, peerAllThreeOrgs[0], peerAllThreeOrgs...)

			By("creating marble4")
			invokeChaincode(network, "org2", "peer0", "marbles-pvtdata", `{"Args":["initMarble","marble4","white","20","jerry","22"]}`, "testchannel", orderer)

			By("waiting for the block to propagate")
			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", nwo.GetLedgerHeight(network, network.Peer("org2", "peer0"), "testchannel"), expectedPeers...)

			By("verifying availability of new private data (marble4) for org1 and org2")
			verifyAccess(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble4"]}`,
				},
				[]*nwo.Peer{
					network.Peer("org1", "peer0"),
					network.Peer("org2", "peer0"),
				},
				`{"docType":"marble","name":"marble4","color":"white","size":20,"owner":"jerry"}`,
			)

			By("verifying that marble4 private data does not exist for org3")
			verifyAccessFailed(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marbles-pvtdata",
					Ctor:      `{"Args":["readMarble","marble4"]}`,
				},
				network.Peer("org3", "peer0"),
				"Failed to get state for marble4",
			)
		})
	})
})

func initThreeOrgsSetup() (string, *nwo.Network, ifrit.Process, *nwo.Orderer, []*nwo.Peer) {
	var err error
	testDir, err := ioutil.TempDir("", "e2e-pvtdata")
	Expect(err).NotTo(HaveOccurred())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := ioutil.ReadFile(filepath.Join("testdata", "network.yaml"))
	Expect(err).NotTo(HaveOccurred())

	var networkConfig *nwo.Config
	err = yaml.Unmarshal(configBytes, &networkConfig)
	Expect(err).NotTo(HaveOccurred())

	n := nwo.New(networkConfig, testDir, client, StartPort(), components)
	n.GenerateConfigTree()
	n.Bootstrap()

	networkRunner := n.NetworkGroupRunner()
	process := ifrit.Invoke(networkRunner)
	Eventually(process.Ready()).Should(BeClosed())

	orderer := n.Orderer("orderer")
	n.CreateAndJoinChannel(orderer, "testchannel")
	n.UpdateChannelAnchors(orderer, "testchannel")

	expectedPeers := []*nwo.Peer{
		n.Peer("org1", "peer0"),
		n.Peer("org2", "peer0"),
		n.Peer("org3", "peer0"),
	}

	By("verifying membership")
	n.VerifyMembership(expectedPeers, "testchannel")

	return testDir, n, process, orderer, expectedPeers
}

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

func testCleanup(testDir string, network *nwo.Network, process ifrit.Process) {
	if process != nil {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
	}
	if network != nil {
		network.Cleanup()
	}
	os.RemoveAll(testDir)
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
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
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
