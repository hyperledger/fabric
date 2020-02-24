/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	mspp "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"
)

var _ bool = Describe("PrivateData", func() {
	// at the beginning of each test under this block, we have 2 collections defined:
	// 1. collectionMarbles - Org1 and Org2 are have access to this collection
	// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
	// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles[1]
	// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails[2]
	Describe("dissemination", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			testDir, network = initThreeOrgsSetup()

			By("disabling pulling by setting pullRetryThreshold to 0")
			// set pull retry threshold to 0, this ensures private data can only be transfered via dissemination
			peers := []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0"),
			}
			for _, p := range peers {
				core := network.ReadPeerConfig(p)
				core.Peer.Gossip.PvtData.PullRetryThreshold = 0
				network.WritePeerConfig(p, core)
			}
		})

		JustBeforeEach(func() {
			process, orderer, expectedPeers = startNetwork(network)

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
			testCleanup(testDir, network, process)
		})

		It("verifies private data was disseminated (positive test) and verifies endorsement fails when dissemination RequiredPeerCount not met (negative test)", func() {

			By("verify access of initial setup (positive test)")
			verifyAccessInitialSetup(network)

			// Integration test structure refactored in future releases. This is a backported integration test from master,
			// hence it doesn't necessarily align with other integration tests in this file that have not yet been refactored.
			By("deploying chaincode with RequiredPeerCount greater than number of peers, endorsement will fail (negative test)")
			testChaincodeHighRequiredPeerCount := nwo.Chaincode{
				Name:              "marblespHighRequiredPeerCount",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config8_high_requiredPeerCount.json")}

			nwo.DeployChaincode(network, "testchannel", orderer, testChaincodeHighRequiredPeerCount)

			// attempt to add a marble with insufficient dissemination to meet RequiredPeerCount
			command := commands.ChaincodeInvoke{
				ChannelID: "testchannel",
				Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
				Name:      testChaincodeHighRequiredPeerCount.Name,
				Ctor:      fmt.Sprintf(`{"Args":["initMarble","marble1","blue","35","tom","99"]}`),
				PeerAddresses: []string{
					network.PeerAddress(network.Peer("org1", "peer0"), nwo.ListenPort),
				},
				WaitForEvent: true,
			}
			expectedErrMsg := `Error: endorsement failure during invoke. response: status:500 message:"failed to distribute private collection`
			invokeChaincodeExpectErr(network, network.Peer("org1", "peer0"), command, expectedErrMsg)

		})

	})

	Describe("reconciliation and pulling", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
			peerProcesses map[string]ifrit.Process
		)

		BeforeEach(func() {
			testDir, network = initThreeOrgsSetup()
		})

		JustBeforeEach(func() {
			By("starting the network")
			peerProcesses = make(map[string]ifrit.Process)
			network.Bootstrap()

			members := grouper.Members{
				{Name: "brokers", Runner: network.BrokerGroupRunner()},
				{Name: "orderers", Runner: network.OrdererGroupRunner()},
			}
			networkRunner := grouper.NewOrdered(syscall.SIGTERM, members)
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			org1peer0 := network.Peer("org1", "peer0")
			org2peer0 := network.Peer("org2", "peer0")
			org3peer0 := network.Peer("org3", "peer0")

			testPeers := []*nwo.Peer{org1peer0, org2peer0, org3peer0}
			for _, peer := range testPeers {
				pr := network.PeerRunner(peer)
				p := ifrit.Invoke(pr)
				peerProcesses[peer.ID()] = p
				Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			orderer = network.Orderer("orderer")
			network.CreateAndJoinChannel(orderer, "testchannel")
			network.UpdateChannelAnchors(orderer, "testchannel")

			expectedPeers = []*nwo.Peer{
				org1peer0,
				org2peer0,
				org3peer0,
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
			for _, peerProcess := range peerProcesses {
				if peerProcess != nil {
					peerProcess.Signal(syscall.SIGTERM)
					Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
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

		// This test has been extended to also verify private data is pulled if the peer has had its ca cert rolled
		// prior to processing a config update change
		It("verify private data is pulled when joining a new peer in an org that belongs to collection config", func() {
			By("generating new certs for org2peer1")
			org2peer1 := network.Peer("org2", "peer1")
			tempCryptoDir, err := ioutil.TempDir("", "crypto")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempCryptoDir)
			generateNewCertsForPeer(network, tempCryptoDir, org2peer1)

			By("updating the channel config with the new certs")
			updateConfigWithNewCertsForPeer(network, tempCryptoDir, orderer, org2peer1)

			By("starting the peer1.org2 process")
			pr := network.PeerRunner(org2peer1)
			p := ifrit.Invoke(pr)
			peerProcesses[org2peer1.ID()] = p
			Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("joining peer1.org2 to the channel with its Admin2 user")
			tempFile, err := ioutil.TempFile("", "genesis-block")
			Expect(err).NotTo(HaveOccurred())
			tempFile.Close()
			defer os.Remove(tempFile.Name())

			sess, err := network.PeerUserSession(org2peer1, "Admin2", commands.ChannelFetch{
				Block:      "0",
				ChannelID:  "testchannel",
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				OutputFile: tempFile.Name(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			sess, err = network.PeerUserSession(org2peer1, "Admin2", commands.ChannelJoin{
				BlockPath: tempFile.Name(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			org2peer1.Channels = append(org2peer1.Channels, &nwo.PeerChannel{Name: "testchannel", Anchor: false})

			ledgerHeight := getLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel")

			By("fetching latest blocks to peer1.org2")
			// Retry channel fetch until peer1.org2 retrieves latest block
			// Channel Fetch will repeatedly fail until org2peer1 commits the config update adding its new cert
			Eventually(fetchBlocksForPeer(network, org2peer1, "Admin2", testDir), network.EventuallyTimeout).Should(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

			By("installing chaincode on peer1.org2 to be able to query it")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json")}

			sess, err = network.PeerUserSession(org2peer1, "Admin2", commands.ChaincodeInstall{
				Name:        chaincode.Name,
				Version:     chaincode.Version,
				Path:        chaincode.Path,
				Lang:        chaincode.Lang,
				PackageFile: chaincode.PackageFile,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			sess, err = network.PeerUserSession(org2peer1, "Admin2", commands.ChaincodeListInstalled{})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincode.Name, chaincode.Version)))

			expectedPeers = []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org2", "peer1"),
				network.Peer("org3", "peer0"),
			}

			By("making sure all peers have the same ledger height")
			for _, peer := range expectedPeers {
				Eventually(func() int {
					var (
						sess *gexec.Session
						err  error
					)
					if peer.ID() == "org2.peer1" {
						// use Admin2 user for peer1.org2
						sess, err = network.PeerUserSession(peer, "Admin2", commands.ChannelInfo{
							ChannelID: "testchannel",
						})
						Expect(err).NotTo(HaveOccurred())
						Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
						channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
						var channelInfo = common.BlockchainInfo{}
						err = json.Unmarshal([]byte(channelInfoStr), &channelInfo)
						Expect(err).NotTo(HaveOccurred())
						return int(channelInfo.Height)
					}

					// If not org2.peer1, just use regular getLedgerHeight call with User1
					return getLedgerHeight(network, peer, "testchannel")
				}(), network.EventuallyTimeout).Should(Equal(
					getLedgerHeight(network, network.Peer("org1", "peer0"), "testchannel")))
			}

			By("verifying membership")
			expectedDiscoveredPeers := make([]nwo.DiscoveredPeer, 0, len(expectedPeers))
			for _, peer := range expectedPeers {
				expectedDiscoveredPeers = append(expectedDiscoveredPeers, network.DiscoveredPeer(peer, "marblesp"))
			}
			for _, peer := range expectedPeers {
				By(fmt.Sprintf("checking expected peers for peer: %s", peer.ID()))
				if peer.ID() == "org2.peer1" {
					// use Admin2 user for peer1.org2
					Eventually(nwo.DiscoverPeers(network, peer, "Admin2", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
				} else {
					Eventually(nwo.DiscoverPeers(network, peer, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
				}
			}

			By("verifying peer1.org2 got the private data that was created historically")
			sess, err = network.PeerUserSession(org2peer1, "Admin2", commands.ChaincodeQuery{
				ChannelID: "testchannel",
				Name:      "marblesp",
				Ctor:      `{"Args":["readMarble","marble1"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`))

			sess, err = network.PeerUserSession(org2peer1, "Admin2", commands.ChaincodeQuery{
				ChannelID: "testchannel",
				Name:      "marblesp",
				Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(`{"docType":"marblePrivateDetails","name":"marble1","price":99}`))
		})
	})

	// This section verifies that chaincode can return private data hash.
	// Unlike private data that can only be accessed from authorized peers as defined in the collection config,
	// private data hash can be queried on any peer in the channel that has the chaincode instantiated.
	// When calling QueryChaincode with "getMarbleHash", the cc will return the private data hash in collectionMarbles.
	// When calling QueryChaincode with "getMarblePrivateDetailsHash", the cc will return the private data hash in collectionMarblePrivateDetails.
	Describe("private data hash", func() {
		var (
			testDir       string
			network       *nwo.Network
			process       ifrit.Process
			orderer       *nwo.Orderer
			expectedPeers []*nwo.Peer
		)

		BeforeEach(func() {
			testDir, network = initThreeOrgsSetup()
		})

		JustBeforeEach(func() {
			process, orderer, expectedPeers = startNetwork(network)

			By("installing and instantiating chaincode on all peers")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json")}
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
		})

		AfterEach(func() {
			testCleanup(testDir, network, process)
		})

		It("gets private data hash by querying chaincode", func() {
			By("invoking initMarble function of the chaincode")
			invokeChaincode(network, "org2", "peer0", "marblesp", `{"Args":["initMarble","marble3","yellow","53","jerry","33"]}`, "testchannel", orderer)

			By("waiting for block to propagate")
			waitUntilAllPeersSameLedgerHeight(network, expectedPeers, "testchannel", getLedgerHeight(network, network.Peer("org2", "peer0"), "testchannel"))

			By("verifying getMarbleHash is accessible from all peers that has the chaincode instantiated")
			peerList := []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0")}
			expectedBytes := util.ComputeStringHash(`{"docType":"marble","name":"marble3","color":"yellow","size":53,"owner":"jerry"}`)
			verifyPvtdataHash(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["getMarbleHash","marble3"]}`},
				peerList,
				expectedBytes)

			By("verifying getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated")
			expectedBytes = util.ComputeStringHash(`{"docType":"marblePrivateDetails","name":"marble3","price":33}`)
			verifyPvtdataHash(
				network,
				commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "marblesp",
					Ctor:      `{"Args":["getMarblePrivateDetailsHash","marble3"]}`},
				peerList,
				expectedBytes)
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
			testDir, network = initThreeOrgsSetup()
		})

		JustBeforeEach(func() {
			process, orderer, _ = startNetwork(network)
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
			initialLedgerHeight := getLedgerHeight(network, org2peer0, "testchannel")

			By("invoking initMarble function of the chaincode to create private data")
			invokeChaincode(network, "org2", "peer0", "marblesp", `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, "testchannel", orderer)

			By("create a block, for private data existence")
			i := 2
			Eventually(func() int {
				invokeChaincode(network, "org2", "peer0", "marblesp", fmt.Sprintf(`{"Args":["initMarble","marble%d","blue%d","3%d","tom","9%d"]}`, i, i, i, i), "testchannel", orderer)
				i++
				return getLedgerHeight(network, org2peer0, "testchannel")
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
				return getLedgerHeight(network, org2peer0, "testchannel")
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
			ledgerHeight := getLedgerHeight(network, org2peer0, "testchannel")
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
				return getLedgerHeight(network, org2peer1, "testchannel")
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

			network = nwo.New(networkConfig, testDir, client, 35000+1000*GinkgoParallelNode(), components)
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
			verifyMembership(network, expectedPeers, "testchannel")
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
			verifyMembership(network, expectedPeers, "testchannel", "marblesp")

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
			ledgerHeight := getLedgerHeight(network, org1peer0, "testchannel")
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
				return getLedgerHeight(network, org3peer0, "testchannel")
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
			testDir, network = initThreeOrgsSetup()
		})

		JustBeforeEach(func() {
			process, orderer, expectedPeers = startNetwork(network)

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
})

func initThreeOrgsSetup() (string, *nwo.Network) {
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

	n := nwo.New(networkConfig, testDir, client, 35000+1000*GinkgoParallelNode(), components)
	n.GenerateConfigTree()

	return testDir, n
}

func startNetwork(n *nwo.Network) (ifrit.Process, *nwo.Orderer, []*nwo.Peer) {
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
	verifyMembership(n, expectedPeers, "testchannel")

	return process, orderer, expectedPeers
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

func invokeChaincodeExpectErr(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke, expectedErrMsg string) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(expectedErrMsg))
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
		Eventually(func() int {
			return getLedgerHeight(n, peer, channelName)
		}, n.EventuallyTimeout).Should(Equal(height))
	}
}

// this function checks that each peer discovered all other peers in the network
func verifyMembership(n *nwo.Network, expectedPeers []*nwo.Peer, channelName string, chaincodes ...string) {
	expectedDiscoveredPeers := make([]nwo.DiscoveredPeer, 0, len(expectedPeers))
	for _, peer := range expectedPeers {
		expectedDiscoveredPeers = append(expectedDiscoveredPeers, n.DiscoveredPeer(peer, chaincodes...))
	}
	for _, peer := range expectedPeers {
		Eventually(nwo.DiscoverPeers(n, peer, "User1", channelName), n.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
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

// verifyPvtdataHash verifies the private data hash matches the expected bytes.
// Cannot reuse verifyAccess because the hash bytes are not valid utf8 causing gbytes.Say to fail.
func verifyPvtdataHash(n *nwo.Network, chaincodeQueryCmd commands.ChaincodeQuery, peers []*nwo.Peer, expected []byte) {
	for _, peer := range peers {
		sess, err := n.PeerUserSession(peer, "User1", chaincodeQueryCmd)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		actual := sess.Buffer().Contents()
		// verify actual bytes contain expected bytes - cannot use equal because session may contain extra bytes
		Expect(bytes.Contains(actual, expected)).To(Equal(true))
	}
}

// fetchBlocksForPeer attempts to fetch the newest block on the given peer.
// It skips the orderer and returns the session's Err buffer for parsing.
func fetchBlocksForPeer(n *nwo.Network, peer *nwo.Peer, user, testDir string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerUserSession(peer, user, commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  "testchannel",
			OutputFile: filepath.Join(testDir, "newest_block.pb"),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		return sess.Err
	}
}

// updateConfigWithNewCertsForPeer updates the channel config with new certs for the designated peer
func updateConfigWithNewCertsForPeer(network *nwo.Network, tempCryptoDir string, orderer *nwo.Orderer, peer *nwo.Peer) {
	org := network.Organization(peer.Organization)

	By("fetching the channel policy")
	currentConfig := nwo.GetConfig(network, network.Peer("org1", "peer0"), orderer, "testchannel")
	updatedConfig := proto.Clone(currentConfig).(*common.Config)

	By("parsing the old and new MSP configs")
	oldConfig := &mspp.MSPConfig{}
	err := proto.Unmarshal(
		updatedConfig.ChannelGroup.Groups["Application"].Groups[org.Name].Values["MSP"].Value,
		oldConfig)
	Expect(err).NotTo(HaveOccurred())

	tempOrgMSPPath := filepath.Join(tempCryptoDir, "peerOrganizations", org.Domain, "msp")
	newConfig, err := msp.GetVerifyingMspConfig(tempOrgMSPPath, org.MSPID, "bccsp")
	Expect(err).NotTo(HaveOccurred())
	oldMspConfig := &mspp.FabricMSPConfig{}
	newMspConfig := &mspp.FabricMSPConfig{}
	err = proto.Unmarshal(oldConfig.Config, oldMspConfig)
	Expect(err).NotTo(HaveOccurred())
	err = proto.Unmarshal(newConfig.Config, newMspConfig)
	Expect(err).NotTo(HaveOccurred())

	By("merging the two MSP configs")
	updateOldMspConfigWithNewMspConfig(oldMspConfig, newMspConfig)

	By("updating the channel config")
	updatedConfig.ChannelGroup.Groups["Application"].Groups["org2"].Values["MSP"].Value = utils.MarshalOrPanic(
		&mspp.MSPConfig{
			Type:   oldConfig.Type,
			Config: utils.MarshalOrPanic(oldMspConfig),
		})
	nwo.UpdateConfig(network, orderer, "testchannel", currentConfig, updatedConfig, false, network.Peer(org.Name, "peer0"))
}

// updateOldMspConfigWithNewMspConfig updates the oldMspConfig with certs from the newMspConfig
func updateOldMspConfigWithNewMspConfig(oldMspConfig, newMspConfig *mspp.FabricMSPConfig) {
	oldMspConfig.RootCerts = append(oldMspConfig.RootCerts, newMspConfig.RootCerts...)
	oldMspConfig.TlsRootCerts = append(oldMspConfig.TlsRootCerts, newMspConfig.TlsRootCerts...)
	oldMspConfig.FabricNodeOus.PeerOuIdentifier.Certificate = nil
	oldMspConfig.FabricNodeOus.ClientOuIdentifier.Certificate = nil
	oldMspConfig.FabricNodeOus.AdminOuIdentifier.Certificate = nil
}

// generateNewCertsForPeer generates new certs with cryptogen for the designated peer and copies
// the necessary certs to the original crypto dir as well as creating an Admin2 user to use for
// any peer operations involving the peer
func generateNewCertsForPeer(network *nwo.Network, tempCryptoDir string, peer *nwo.Peer) {
	sess, err := network.Cryptogen(commands.Generate{
		Config: network.CryptoConfigPath(),
		Output: tempCryptoDir,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

	By("copying the new msp certs for the peer to the original crypto dir")
	oldPeerMSPPath := network.PeerLocalMSPDir(peer)
	org := network.Organization(peer.Organization)
	tempPeerMSPPath := filepath.Join(
		tempCryptoDir,
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", peer.Name, org.Domain),
		"msp",
	)
	os.RemoveAll(oldPeerMSPPath)
	err = exec.Command("cp", "-r", tempPeerMSPPath, oldPeerMSPPath).Run()
	Expect(err).NotTo(HaveOccurred())

	// This lets us keep the old user certs for the org for any peers still remaining in the org
	// using the old certs
	By("copying the new Admin user cert to the original user certs dir as Admin2")
	oldAdminUserPath := filepath.Join(
		network.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"users",
		fmt.Sprintf("Admin2@%s", org.Domain),
	)
	tempAdminUserPath := filepath.Join(
		tempCryptoDir,
		"peerOrganizations",
		org.Domain,
		"users",
		fmt.Sprintf("Admin@%s", org.Domain),
	)
	os.RemoveAll(oldAdminUserPath)
	err = exec.Command("cp", "-r", tempAdminUserPath, oldAdminUserPath).Run()
	Expect(err).NotTo(HaveOccurred())
	// We need to rename the signcert from Admin to Admin2 as well
	err = os.Rename(
		filepath.Join(oldAdminUserPath, "msp", "signcerts", fmt.Sprintf("Admin@%s-cert.pem", org.Domain)),
		filepath.Join(oldAdminUserPath, "msp", "signcerts", fmt.Sprintf("Admin2@%s-cert.pem", org.Domain)),
	)
	Expect(err).NotTo(HaveOccurred())
}
