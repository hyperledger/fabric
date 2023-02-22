/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	mspp "github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/pvtdata/marblechaincodeutil"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
	"google.golang.org/grpc"
)

// The chaincode used in these tests has two collections defined:
// collectionMarbles and collectionMarblePrivateDetails
// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles
// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails

const channelID = "testchannel"

var _ bool = Describe("PrivateData", func() {
	var (
		network                     *nwo.Network
		ordererProcess, peerProcess ifrit.Process
		orderer                     *nwo.Orderer
	)

	AfterEach(func() {
		testCleanup(network, ordererProcess, peerProcess)
	})

	Describe("Dissemination when pulling and reconciliation are disabled", func() {
		BeforeEach(func() {
			By("setting up the network")
			network = initThreeOrgsSetup(true)

			By("setting the pull retry threshold to 0 and disabling reconciliation on all peers")
			for _, p := range network.Peers {
				core := network.ReadPeerConfig(p)
				core.Peer.Gossip.PvtData.PullRetryThreshold = 0
				core.Peer.Gossip.PvtData.ReconciliationEnabled = false
				network.WritePeerConfig(p, core)
			}

			By("starting the network")
			ordererProcess, peerProcess, orderer = startNetwork(network)
		})

		It("disseminates private data per collections_config1 (positive test) and collections_config8 (negative test)", func() {
			By("deploying legacy chaincode and adding marble1")
			testChaincode := chaincode{
				Chaincode: nwo.Chaincode{
					Name:    "marblesp",
					Version: "1.0",
					Path:    "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					Ctor:    `{"Args":["init"]}`,
					Policy:  `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
					// collections_config1.json defines the access as follows:
					// 1. collectionMarbles - Org1, Org2 have access to this collection
					// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
					CollectionsConfig: CollectionConfig("collections_config1.json"),
				},
				isLegacy: true,
			}
			deployChaincode(network, orderer, testChaincode)
			marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name,
				`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`,
				network.Peer("Org1", "peer0"),
			)

			assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble1")

			By("deploying chaincode with RequiredPeerCount greater than number of peers, endorsement will fail")
			testChaincodeHighRequiredPeerCount := chaincode{
				Chaincode: nwo.Chaincode{
					Name:              "marblespHighRequiredPeerCount",
					Version:           "1.0",
					Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					Ctor:              `{"Args":["init"]}`,
					Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
					CollectionsConfig: CollectionConfig("collections_config8_high_requiredPeerCount.json"),
				},
				isLegacy: true,
			}
			deployChaincode(network, orderer, testChaincodeHighRequiredPeerCount)

			// attempt to add a marble with insufficient dissemination to meet RequiredPeerCount
			marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`))

			command := commands.ChaincodeInvoke{
				ChannelID: channelID,
				Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
				Name:      testChaincodeHighRequiredPeerCount.Name,
				Ctor:      `{"Args":["initMarble"]}`,
				Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
				PeerAddresses: []string{
					network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
				},
				WaitForEvent: true,
			}
			expectedErrMsg := `Error: endorsement failure during invoke. response: status:500 message:"error in simulation: failed to distribute private collection`
			invokeChaincodeExpectErr(network, network.Peer("Org1", "peer0"), command, expectedErrMsg)
		})

		When("collection config does not have maxPeerCount or requiredPeerCount", func() {
			It("disseminates private data per collections_config7 with default maxPeerCount and requiredPeerCount", func() {
				By("deploying legacy chaincode and adding marble1")
				testChaincode := chaincode{
					Chaincode: nwo.Chaincode{
						Name:    "marblesp",
						Version: "1.0",
						Path:    "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
						Ctor:    `{"Args":["init"]}`,
						Policy:  `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
						// collections_config1.json defines the access as follows:
						// 1. collectionMarbles - Org1, Org2 have access to this collection
						// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
						CollectionsConfig: CollectionConfig("collections_config7.json"),
					},
					isLegacy: true,
				}
				deployChaincode(network, orderer, testChaincode)
				peer := network.Peer("Org1", "peer0")
				By("adding marble1")
				marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name,
					`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`,
					peer,
				)

				By("asserting pvtdata in each collection for config7")
				assertPvtdataPresencePerCollectionConfig7(network, testChaincode.Name, "marble1", peer)
			})
		})
	})

	Describe("Pvtdata behavior when a peer with new certs joins the network", func() {
		var peerProcesses map[string]ifrit.Process

		BeforeEach(func() {
			By("setting up the network")
			network = initThreeOrgsSetup(true)

			By("starting the network")
			peerProcesses = make(map[string]ifrit.Process)
			network.Bootstrap()

			orderer = network.Orderer("orderer")
			ordererRunner := network.OrdererRunner(orderer)
			ordererProcess = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready()).Should(BeClosed())

			org1peer0 := network.Peer("Org1", "peer0")
			org2peer0 := network.Peer("Org2", "peer0")
			org3peer0 := network.Peer("Org3", "peer0")

			testPeers := []*nwo.Peer{org1peer0, org2peer0, org3peer0}
			for _, peer := range testPeers {
				pr := network.PeerRunner(peer)
				p := ifrit.Invoke(pr)
				peerProcesses[peer.ID()] = p
				Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			channelparticipation.JoinOrdererJoinPeersAppChannel(network, channelID, orderer, ordererRunner)

			By("verifying membership")
			network.VerifyMembership(network.Peers, channelID)

			By("installing and instantiating chaincode on all peers")
			testChaincode := chaincode{
				Chaincode: nwo.Chaincode{
					Name:              "marblesp",
					Version:           "1.0",
					Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					Ctor:              `{"Args":["init"]}`,
					Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
					CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
				},
				isLegacy: true,
			}
			deployChaincode(network, orderer, testChaincode)

			By("adding marble1 with an org 1 peer as endorser")
			peer := network.Peer("Org1", "peer0")
			marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
			marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, marbleDetails, peer)

			By("waiting for block to propagate")
			nwo.WaitUntilEqualLedgerHeight(network, channelID, nwo.GetLedgerHeight(network, network.Peers[0], channelID), network.Peers...)

			org2Peer1 := &nwo.Peer{
				Name:         "peer1",
				Organization: "Org2",
				Channels:     []*nwo.PeerChannel{}, // Don't set channels here so the UpdateConfig call doesn't try to fetch blocks for org2Peer1 with the default Admin user
			}
			network.Peers = append(network.Peers, org2Peer1)
		})

		AfterEach(func() {
			for _, peerProcess := range peerProcesses {
				if peerProcess != nil {
					peerProcess.Signal(syscall.SIGTERM)
					Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
		})

		It("verifies private data is pulled when joining a new peer with new certs", func() {
			By("generating new certs for org2Peer1")
			org2Peer1 := network.Peer("Org2", "peer1")
			tempCryptoDir, err := ioutil.TempDir("", "crypto")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempCryptoDir)
			generateNewCertsForPeer(network, tempCryptoDir, org2Peer1)

			By("updating the channel config with the new certs")
			updateConfigWithNewCertsForPeer(network, tempCryptoDir, orderer, org2Peer1)

			By("starting the peer1.org2 process")
			pr := network.PeerRunner(org2Peer1)
			p := ifrit.Invoke(pr)
			peerProcesses[org2Peer1.ID()] = p
			Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("joining peer1.org2 to the channel with its Admin2 user")
			tempFile, err := ioutil.TempFile("", "genesis-block")
			Expect(err).NotTo(HaveOccurred())
			tempFile.Close()
			defer os.Remove(tempFile.Name())

			sess, err := network.PeerUserSession(org2Peer1, "Admin2", commands.ChannelFetch{
				Block:      "0",
				ChannelID:  channelID,
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				OutputFile: tempFile.Name(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			sess, err = network.PeerUserSession(org2Peer1, "Admin2", commands.ChannelJoin{
				BlockPath: tempFile.Name(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			org2Peer1.Channels = append(org2Peer1.Channels, &nwo.PeerChannel{Name: channelID, Anchor: false})

			ledgerHeight := nwo.GetLedgerHeight(network, network.Peers[0], channelID)

			By("fetching latest blocks to peer1.org2")
			// Retry channel fetch until peer1.org2 retrieves latest block
			// Channel Fetch will repeatedly fail until org2Peer1 commits the config update adding its new cert
			Eventually(fetchBlocksForPeer(network, org2Peer1, "Admin2"), network.EventuallyTimeout).Should(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

			By("installing chaincode on peer1.org2 to be able to query it")
			chaincode := nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:              `{"Args":["init"]}`,
				Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
			}

			sess, err = network.PeerUserSession(org2Peer1, "Admin2", commands.ChaincodeInstallLegacy{
				Name:        chaincode.Name,
				Version:     chaincode.Version,
				Path:        chaincode.Path,
				Lang:        chaincode.Lang,
				PackageFile: chaincode.PackageFile,
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			sess, err = network.PeerUserSession(org2Peer1, "Admin2", commands.ChaincodeListInstalledLegacy{
				ClientAuth: network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincode.Name, chaincode.Version)))

			expectedPeers := []*nwo.Peer{
				network.Peer("Org1", "peer0"),
				network.Peer("Org2", "peer0"),
				network.Peer("Org2", "peer1"),
				network.Peer("Org3", "peer0"),
			}

			By("making sure all peers have the same ledger height")
			for _, peer := range expectedPeers {
				Eventually(func() int {
					var (
						sess *gexec.Session
						err  error
					)
					if peer.ID() == "Org2.peer1" {
						// use Admin2 user for peer1.org2
						sess, err = network.PeerUserSession(peer, "Admin2", commands.ChannelInfo{
							ChannelID: channelID,
						})
						Expect(err).NotTo(HaveOccurred())
						Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
						channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
						channelInfo := cb.BlockchainInfo{}
						err = json.Unmarshal([]byte(channelInfoStr), &channelInfo)
						Expect(err).NotTo(HaveOccurred())
						return int(channelInfo.Height)
					}

					// If not Org2.peer1, just use regular getLedgerHeight call with User1
					return nwo.GetLedgerHeight(network, peer, channelID)
				}(), network.EventuallyTimeout).Should(Equal(ledgerHeight))
			}

			By("verifying membership")
			expectedDiscoveredPeers := make([]nwo.DiscoveredPeer, 0, len(expectedPeers))
			for _, peer := range expectedPeers {
				expectedDiscoveredPeers = append(expectedDiscoveredPeers, network.DiscoveredPeer(peer, "_lifecycle", "marblesp"))
			}
			for _, peer := range expectedPeers {
				By(fmt.Sprintf("checking expected peers for peer: %s", peer.ID()))
				if peer.ID() == "Org2.peer1" {
					// use Admin2 user for peer1.org2
					Eventually(nwo.DiscoverPeers(network, peer, "Admin2", channelID), network.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
				} else {
					Eventually(nwo.DiscoverPeers(network, peer, "User1", channelID), network.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
				}
			}

			By("verifying peer1.org2 got the private data that was created historically")
			sess, err = network.PeerUserSession(org2Peer1, "Admin2", commands.ChaincodeQuery{
				ChannelID: channelID,
				Name:      "marblesp",
				Ctor:      `{"Args":["readMarble","marble1"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(`{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`))

			sess, err = network.PeerUserSession(org2Peer1, "Admin2", commands.ChaincodeQuery{
				ChannelID: channelID,
				Name:      "marblesp",
				Ctor:      `{"Args":["readMarblePrivateDetails","marble1"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say(`{"docType":"marblePrivateDetails","name":"marble1","price":99}`))
		})
	})

	Describe("Pvtdata behavior with untouched peer configs", func() {
		var (
			legacyChaincode       nwo.Chaincode
			newLifecycleChaincode nwo.Chaincode
			testChaincode         chaincode

			org1Peer1, org2Peer1 *nwo.Peer
		)

		BeforeEach(func() {
			By("setting up the network")
			network = initThreeOrgsSetup(true)
			legacyChaincode = nwo.Chaincode{
				Name:    "marblesp",
				Version: "1.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:    `{"Args":["init"]}`,
				Policy:  `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				// collections_config1.json defines the access as follows:
				// 1. collectionMarbles - Org1, Org2 have access to this collection
				// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
				CollectionsConfig: CollectionConfig("collections_config1.json"),
			}

			newLifecycleChaincode = nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
				Lang:              "binary",
				PackageFile:       filepath.Join(network.RootDir, "marbles-pvtdata.tar.gz"),
				Label:             "marbles-private-20",
				SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: CollectionConfig("collections_config1.json"),
				Sequence:          "1",
			}
			org1Peer1 = &nwo.Peer{
				Name:         "peer1",
				Organization: "Org1",
				Channels: []*nwo.PeerChannel{
					{Name: channelID},
				},
			}
			org2Peer1 = &nwo.Peer{
				Name:         "peer1",
				Organization: "Org2",
				Channels: []*nwo.PeerChannel{
					{Name: channelID},
				},
			}

			ordererProcess, peerProcess, orderer = startNetwork(network)
		})

		Describe("Reconciliation and pulling", func() {
			var newPeerProcess ifrit.Process

			BeforeEach(func() {
				By("deploying legacy chaincode and adding marble1")
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
				deployChaincode(network, orderer, testChaincode)
				marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name,
					`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`,
					network.Peer("Org1", "peer0"),
				)
			})

			AfterEach(func() {
				if newPeerProcess != nil {
					newPeerProcess.Signal(syscall.SIGTERM)
					Eventually(newPeerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			})

			assertReconcileBehavior := func() {
				It("disseminates private data per collections_config1", func() {
					assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble1")
				})

				When("org3 is added to collectionMarbles via chaincode upgrade with collections_config2", func() {
					It("distributes and allows access to newly added private data per collections_config2", func() {
						By("upgrading the chaincode and adding marble2")
						// collections_config2.json defines the access as follows:
						// 1. collectionMarbles - Org1, Org2 and Org3 have access to this collection
						// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
						// the change from collections_config1 - org3 was added to collectionMarbles
						testChaincode.Version = "1.1"
						testChaincode.CollectionsConfig = CollectionConfig("collections_config2.json")
						if !testChaincode.isLegacy {
							testChaincode.Sequence = "2"
						}
						upgradeChaincode(network, orderer, testChaincode)
						marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name,
							`{"name":"marble2", "color":"yellow", "size":53, "owner":"jerry", "price":22}`,
							network.Peer("Org2", "peer0"),
						)

						assertPvtdataPresencePerCollectionConfig2(network, testChaincode.Name, "marble2")
					})
				})

				When("a new peer in org1 joins the channel", func() {
					It("causes the new peer to pull the existing private data only for collectionMarbles", func() {
						By("adding a new peer")
						newPeerProcess = addPeer(network, orderer, org1Peer1)
						installChaincode(network, testChaincode, org1Peer1)
						network.VerifyMembership(network.Peers, channelID, "marblesp")

						assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble1", org1Peer1)
					})
				})
			}

			When("chaincode is migrated from legacy to new lifecycle with same collection config", func() {
				BeforeEach(func() {
					testChaincode = chaincode{
						Chaincode: newLifecycleChaincode,
						isLegacy:  false,
					}
					nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
					upgradeChaincode(network, orderer, testChaincode)
				})
				assertReconcileBehavior()
			})
		})

		Describe("BlockToLive", func() {
			var newPeerProcess ifrit.Process

			AfterEach(func() {
				if newPeerProcess != nil {
					newPeerProcess.Signal(syscall.SIGTERM)
					Eventually(newPeerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			})

			It("purges private data after BTL and causes new peer not to pull the purged private data", func() {
				By("deploying new lifecycle chaincode and adding marble1")
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)

				testChaincode.CollectionsConfig = CollectionConfig("short_btl_config.json")
				deployChaincode(network, orderer, testChaincode)
				marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))

				eligiblePeer := network.Peer("Org2", "peer0")
				ccName := testChaincode.Name
				By("adding three blocks")
				for i := 0; i < 3; i++ {
					marblechaincodeutil.AddMarble(network, orderer, channelID, ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
				}

				By("verifying that marble1 still not purged in collection MarblesPD")
				marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, ccName, "marble1", eligiblePeer)

				By("adding one more block")
				marblechaincodeutil.AddMarble(network, orderer, channelID, ccName, `{"name":"fun-marble-3", "color":"blue", "size":35, "owner":"tom", "price":99}`, eligiblePeer)

				By("verifying that marble1 purged in collection MarblesPD")
				marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, ccName, "marble1", eligiblePeer)

				By("verifying that marble1 still not purged in collection Marbles")
				marblechaincodeutil.AssertPresentInCollectionM(network, channelID, ccName, "marble1", eligiblePeer)

				By("adding new peer that is eligible to receive data")
				newPeerProcess = addPeer(network, orderer, org2Peer1)
				installChaincode(network, testChaincode, org2Peer1)
				network.VerifyMembership(network.Peers, channelID, ccName)
				marblechaincodeutil.AssertPresentInCollectionM(network, channelID, ccName, "marble1", org2Peer1)
				marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, ccName, "marble1", org2Peer1)
			})
		})

		Describe("Org removal from collection", func() {
			assertOrgRemovalBehavior := func() {
				By("upgrading chaincode to remove org3 from collectionMarbles")
				testChaincode.CollectionsConfig = CollectionConfig("collections_config1.json")
				testChaincode.Version = "1.1"
				if !testChaincode.isLegacy {
					testChaincode.Sequence = "2"
				}
				upgradeChaincode(network, orderer, testChaincode)
				marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, `{"name":"marble2", "color":"yellow", "size":53, "owner":"jerry", "price":22}`, network.Peer("Org2", "peer0"))
				assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble2")
			}

			It("causes removed org not to get new data", func() {
				By("deploying new lifecycle chaincode and adding marble1")
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
				testChaincode.CollectionsConfig = CollectionConfig("collections_config2.json")
				deployChaincode(network, orderer, testChaincode)
				marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))
				assertPvtdataPresencePerCollectionConfig2(network, testChaincode.Name, "marble1")

				assertOrgRemovalBehavior()
			})
		})

		When("migrating a chaincode from legacy lifecycle to new lifecycle", func() {
			It("performs check against collection config from legacy lifecycle", func() {
				By("deploying legacy chaincode")
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
				deployChaincode(network, orderer, testChaincode)
				nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)

				newLifecycleChaincode.CollectionsConfig = CollectionConfig("short_btl_config.json")
				newLifecycleChaincode.PackageID = "test-package-id"

				approveChaincodeForMyOrgExpectErr(
					network,
					orderer,
					newLifecycleChaincode,
					`the BlockToLive in an existing collection \[collectionMarblePrivateDetails\] modified. Existing value \[1000000\]`,
					network.Peer("Org2", "peer0"))
			})
		})

		Describe("Collection Config Endorsement Policy", func() {
			When("using legacy lifecycle chaincode", func() {
				It("ignores the collection config endorsement policy and successfully invokes the chaincode", func() {
					testChaincode = chaincode{
						Chaincode: legacyChaincode,
						isLegacy:  true,
					}
					By("setting the collection config endorsement policy to org2 or org3 peers")
					testChaincode.CollectionsConfig = CollectionConfig("collections_config4.json")

					By("deploying legacy chaincode")
					deployChaincode(network, orderer, testChaincode)

					By("adding marble1 with an org 1 peer as endorser")
					peer := network.Peer("Org1", "peer0")
					marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
					marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, marbleDetails, peer)
				})
			})

			When("using new lifecycle chaincode", func() {
				BeforeEach(func() {
					testChaincode = chaincode{
						Chaincode: newLifecycleChaincode,
						isLegacy:  false,
					}
					nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peers...)
				})

				When("a peer specified in the chaincode endorsement policy but not in the collection config endorsement policy is used to invoke the chaincode", func() {
					It("fails validation", func() {
						By("setting the collection config endorsement policy to org2 or org3 peers")
						testChaincode.CollectionsConfig = CollectionConfig("collections_config4.json")

						By("deploying new lifecycle chaincode")
						// set collection endorsement policy to org2 or org3
						deployChaincode(network, orderer, testChaincode)

						By("adding marble1 with an org1 peer as endorser")
						peer := network.Peer("Org1", "peer0")
						marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
						marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDetails))

						command := commands.ChaincodeInvoke{
							ChannelID: channelID,
							Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
							Name:      testChaincode.Name,
							Ctor:      `{"Args":["initMarble"]}`,
							Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
							PeerAddresses: []string{
								network.PeerAddress(peer, nwo.ListenPort),
							},
							WaitForEvent: true,
						}

						sess, err := network.PeerUserSession(peer, "User1", command)
						Expect(err).NotTo(HaveOccurred())
						Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
						Expect(sess.Err).To(gbytes.Say("ENDORSEMENT_POLICY_FAILURE"))
					})
				})

				When("a peer specified in the collection endorsement policy but not in the chaincode endorsement policy is used to invoke the chaincode", func() {
					When("the collection endorsement policy is a signature policy", func() {
						It("successfully invokes the chaincode", func() {
							// collection config endorsement policy specifies org2 or org3 peers for endorsement
							By("setting the collection config endorsement policy to use a signature policy")
							testChaincode.CollectionsConfig = CollectionConfig("collections_config4.json")

							By("setting the chaincode endorsement policy to org1 or org2 peers")
							testChaincode.SignaturePolicy = `OR ('Org1MSP.member','Org2MSP.member')`

							By("deploying new lifecycle chaincode")
							// set collection endorsement policy to org2 or org3
							deployChaincode(network, orderer, testChaincode)

							By("adding marble1 with an org3 peer as endorser")
							peer := network.Peer("Org3", "peer0")
							marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
							marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, marbleDetails, peer)
						})
					})

					When("the collection endorsement policy is a channel config policy reference", func() {
						It("successfully invokes the chaincode", func() {
							// collection config endorsement policy specifies channel config policy reference /Channel/Application/Readers
							By("setting the collection config endorsement policy to use a channel config policy reference")
							testChaincode.CollectionsConfig = CollectionConfig("collections_config5.json")

							By("setting the channel endorsement policy to org1 or org2 peers")
							testChaincode.SignaturePolicy = `OR ('Org1MSP.member','Org2MSP.member')`

							By("deploying new lifecycle chaincode")
							deployChaincode(network, orderer, testChaincode)

							By("adding marble1 with an org3 peer as endorser")
							peer := network.Peer("Org3", "peer0")
							marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
							marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, marbleDetails, peer)
						})
					})
				})

				When("the collection config endorsement policy specifies a semantically wrong, but well formed signature policy", func() {
					It("fails to invoke the chaincode with an endorsement policy failure", func() {
						By("setting the collection config endorsement policy to non existent org4 peers")
						testChaincode.CollectionsConfig = CollectionConfig("collections_config6.json")

						By("deploying new lifecycle chaincode")
						deployChaincode(network, orderer, testChaincode)

						By("adding marble1 with an org1 peer as endorser")
						peer := network.Peer("Org1", "peer0")
						marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
						marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDetails))

						command := commands.ChaincodeInvoke{
							ChannelID: channelID,
							Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
							Name:      testChaincode.Name,
							Ctor:      `{"Args":["initMarble"]}`,
							Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
							PeerAddresses: []string{
								network.PeerAddress(peer, nwo.ListenPort),
							},
							WaitForEvent: true,
						}

						sess, err := network.PeerUserSession(peer, "User1", command)
						Expect(err).NotTo(HaveOccurred())
						Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
						Expect(sess.Err).To(gbytes.Say("ENDORSEMENT_POLICY_FAILURE"))
					})
				})
			})
		})
	})

	Describe("marble APIs invocation and private data delivery", func() {
		var (
			newLifecycleChaincode nwo.Chaincode
			testChaincode         chaincode
		)

		BeforeEach(func() {
			By("setting up the network")
			network = initThreeOrgsSetup(true)

			newLifecycleChaincode = nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
				Lang:              "binary",
				PackageFile:       filepath.Join(network.RootDir, "marbles-pvtdata.tar.gz"),
				Label:             "marbles-private-20",
				SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: CollectionConfig("collections_config1.json"),
				Sequence:          "1",
			}

			// In assertDeliverWithPrivateDataACLBehavior, there is a single goroutine that communicates to DeliverService.
			// Set DeliverService concurrency limit to 1 in order to verify that the grpc server in a peer works correctly
			// when reaching (but not exceeding) the concurrency limit.
			By("setting deliverservice concurrency limit to 1")
			for _, p := range network.Peers {
				core := network.ReadPeerConfig(p)
				core.Peer.Limits.Concurrency.DeliverService = 1
				network.WritePeerConfig(p, core)
			}
			ordererProcess, peerProcess, orderer = startNetwork(network)
		})

		// call marble APIs: getMarblesByRange, transferMarble, delete, getMarbleHash, getMarblePrivateDetailsHash and verify ACL Behavior
		assertMarbleAPIs := func() {
			eligiblePeer := network.Peer("Org2", "peer0")
			ccName := testChaincode.Name

			// Verifies marble private chaincode APIs: getMarblesByRange, transferMarble, delete

			By("adding five marbles")
			for i := 0; i < 5; i++ {
				marblechaincodeutil.AddMarble(network, orderer, channelID, ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
			}

			By("getting marbles by range")
			expectedMsg := `\Q[{"Key":"test-marble-0", "Record":{"docType":"marble","name":"test-marble-0","color":"blue","size":35,"owner":"tom"}},{"Key":"test-marble-1", "Record":{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}}]\E`
			marblechaincodeutil.AssertGetMarblesByRange(network, channelID, ccName, `"test-marble-0", "test-marble-2"`, expectedMsg, eligiblePeer)

			By("transferring test-marble-0 to jerry")
			marblechaincodeutil.TransferMarble(network, orderer, channelID, ccName, `{"name":"test-marble-0", "owner":"jerry"}`, eligiblePeer)

			By("verifying the new ownership of test-marble-0")
			marblechaincodeutil.AssertOwnershipInCollectionM(network, channelID, ccName, `test-marble-0`, "jerry", eligiblePeer)

			By("deleting test-marble-0")
			marblechaincodeutil.DeleteMarble(network, orderer, channelID, ccName, `{"name":"test-marble-0"}`, eligiblePeer)

			By("verifying the deletion of test-marble-0")
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, ccName, `test-marble-0`, eligiblePeer)

			// This section verifies that chaincode can return private data hash.
			// Unlike private data that can only be accessed from authorized peers as defined in the collection config,
			// private data hash can be queried on any peer in the channel that has the chaincode instantiated.
			// When calling QueryChaincode with "getMarbleHash", the cc will return the private data hash in collectionMarbles.
			// When calling QueryChaincode with "getMarblePrivateDetailsHash", the cc will return the private data hash in collectionMarblePrivateDetails.

			peerList := []*nwo.Peer{
				network.Peer("Org1", "peer0"),
				network.Peer("Org2", "peer0"),
				network.Peer("Org3", "peer0"),
			}

			By("verifying getMarbleHash is accessible from all peers that has the chaincode instantiated")
			expectedBytes := util.ComputeStringHash(`{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}`)
			marblechaincodeutil.AssertMarblesPrivateHashM(network, channelID, ccName, "test-marble-1", expectedBytes, peerList)

			By("verifying getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated")
			expectedBytes = util.ComputeStringHash(`{"docType":"marblePrivateDetails","name":"test-marble-1","price":99}`)
			marblechaincodeutil.AssertMarblesPrivateDetailsHashMPD(network, channelID, ccName, "test-marble-1", expectedBytes, peerList)

			// collection ACL while reading private data: not allowed to non-members
			// collections_config3: collectionMarblePrivateDetails - member_only_read is set to true

			By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, shouldn't have read access")
			marblechaincodeutil.AssertNoReadAccessToCollectionMPD(network, channelID, testChaincode.Name, "test-marble-1", network.Peer("Org1", "peer0"))
		}

		// verify DeliverWithPrivateData sends private data based on the ACL in collection config
		// before and after upgrade.
		assertDeliverWithPrivateDataACLBehavior := func() {
			By("getting signing identity for a user in org1")
			signingIdentity := network.PeerUserSigner(network.Peer("Org1", "peer0"), "User1")

			By("adding a marble")
			peer := network.Peer("Org2", "peer0")
			marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name, `{"name":"marble11", "color":"blue", "size":35, "owner":"tom", "price":99}`, peer)

			By("getting the deliver event for newest block")
			event := getEventFromDeliverService(network, peer, channelID, signingIdentity, 0)

			By("verifying private data in deliver event contains 'collectionMarbles' only")
			// it should receive pvtdata for 'collectionMarbles' only because memberOnlyRead is true
			expectedKVWritesMap := map[string]map[string][]byte{
				"collectionMarbles": {
					"\000color~name\000blue\000marble11\000": []byte("\000"),
					"marble11":                               getValueForCollectionMarbles("marble11", "blue", "tom", 35),
				},
			}
			assertPrivateDataAsExpected(event.BlockAndPvtData.PrivateDataMap, expectedKVWritesMap)

			By("upgrading chaincode with collections_config1.json where isMemberOnlyRead is false")
			testChaincode.CollectionsConfig = CollectionConfig("collections_config1.json")
			testChaincode.Version = "1.1"
			if !testChaincode.isLegacy {
				testChaincode.Sequence = "2"
			}
			upgradeChaincode(network, orderer, testChaincode)

			By("getting the deliver event for an old block committed before upgrade")
			event = getEventFromDeliverService(network, peer, channelID, signingIdentity, event.BlockNum)

			By("verifying the deliver event for the old block uses old config")
			assertPrivateDataAsExpected(event.BlockAndPvtData.PrivateDataMap, expectedKVWritesMap)

			By("adding a new marble after upgrade")
			marblechaincodeutil.AddMarble(network, orderer, channelID, testChaincode.Name,
				`{"name":"marble12", "color":"blue", "size":35, "owner":"tom", "price":99}`,
				network.Peer("Org1", "peer0"),
			)
			By("getting the deliver event for a new block committed after upgrade")
			event = getEventFromDeliverService(network, peer, channelID, signingIdentity, 0)

			// it should receive pvtdata for both collections because memberOnlyRead is false
			By("verifying the deliver event for the new block uses new config")
			expectedKVWritesMap = map[string]map[string][]byte{
				"collectionMarbles": {
					"\000color~name\000blue\000marble12\000": []byte("\000"),
					"marble12":                               getValueForCollectionMarbles("marble12", "blue", "tom", 35),
				},
				"collectionMarblePrivateDetails": {
					"marble12": getValueForCollectionMarblePrivateDetails("marble12", 99),
				},
			}
			assertPrivateDataAsExpected(event.BlockAndPvtData.PrivateDataMap, expectedKVWritesMap)
		}

		It("calls marbles APIs and delivers private data", func() {
			By("deploying new lifecycle chaincode")
			testChaincode = chaincode{
				Chaincode: newLifecycleChaincode,
				isLegacy:  false,
			}
			nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
			testChaincode.CollectionsConfig = CollectionConfig("collections_config3.json")
			deployChaincode(network, orderer, testChaincode)

			By("attempting to invoke chaincode from a user (org1) not in any collection member orgs (org2 and org3)")
			peer2 := network.Peer("Org2", "peer0")
			marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(`{"name":"memberonly-marble", "color":"blue", "size":35, "owner":"tom", "price":99}`))
			command := commands.ChaincodeInvoke{
				ChannelID:     channelID,
				Orderer:       network.OrdererAddress(orderer, nwo.ListenPort),
				Name:          "marblesp",
				Ctor:          `{"Args":["initMarble"]}`,
				Transient:     fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
				PeerAddresses: []string{network.PeerAddress(peer2, nwo.ListenPort)},
				WaitForEvent:  true,
			}
			peer1 := network.Peer("Org1", "peer0")
			expectedErrMsg := "tx creator does not have write access permission"
			invokeChaincodeExpectErr(network, peer1, command, expectedErrMsg)

			assertMarbleAPIs()
			assertDeliverWithPrivateDataACLBehavior()
		})
	})
})

func initThreeOrgsSetup(removePeer1 bool) *nwo.Network {
	var err error
	testDir, err := ioutil.TempDir("", "e2e-pvtdata")
	Expect(err).NotTo(HaveOccurred())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	config := nwo.FullEtcdRaftNoSysChan()

	// add org3 with one peer
	config.Organizations = append(config.Organizations, &nwo.Organization{
		Name:          "Org3",
		MSPID:         "Org3MSP",
		Domain:        "org3.example.com",
		EnableNodeOUs: true,
		Users:         2,
		CA:            &nwo.CA{Hostname: "ca"},
	})
	config.Profiles[0].Organizations = append(config.Profiles[0].Organizations, "Org3")
	config.Peers = append(config.Peers, &nwo.Peer{
		Name:         "peer0",
		Organization: "Org3",
		Channels: []*nwo.PeerChannel{
			{Name: channelID, Anchor: true},
		},
	})

	n := nwo.New(config, testDir, client, StartPort(), components)
	n.GenerateConfigTree()

	if !removePeer1 {
		Expect(n.Peers).To(HaveLen(5))
		return n
	}

	// remove peer1 from org1 and org2 so we can add it back later, we generate the config tree above
	// with the two peers so the config files exist later when adding the peer back
	peers := []*nwo.Peer{}
	for _, p := range n.Peers {
		if p.Name != "peer1" {
			peers = append(peers, p)
		}
	}
	n.Peers = peers
	Expect(n.Peers).To(HaveLen(3))

	return n
}

func startNetwork(n *nwo.Network) (ifrit.Process, ifrit.Process, *nwo.Orderer) {
	n.Bootstrap()

	// Start all the fabric processes
	ordererRunner, ordererProcess, peerProcess := n.StartSingleOrdererNetwork("orderer")

	orderer := n.Orderer("orderer")
	channelparticipation.JoinOrdererJoinPeersAppChannel(n, channelID, orderer, ordererRunner)

	By("verifying membership")
	n.VerifyMembership(n.Peers, channelID)

	return ordererProcess, peerProcess, orderer
}

func testCleanup(network *nwo.Network, ordererProcess, peerProcess ifrit.Process) {
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
	os.RemoveAll(network.RootDir)
}

type chaincode struct {
	nwo.Chaincode
	isLegacy bool
}

func addPeer(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) ifrit.Process {
	process := ifrit.Invoke(n.PeerRunner(peer))
	Eventually(process.Ready(), n.EventuallyTimeout).Should(BeClosed())

	n.JoinChannel(channelID, orderer, peer)
	ledgerHeight := nwo.GetLedgerHeight(n, n.Peers[0], channelID)
	sess, err := n.PeerAdminSession(
		peer,
		commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  channelID,
			Orderer:    n.OrdererAddress(orderer, nwo.ListenPort),
			OutputFile: filepath.Join(n.RootDir, "newest_block.pb"),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

	n.Peers = append(n.Peers, peer)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, n.Peers[0], channelID), n.Peers...)

	return process
}

func deployChaincode(n *nwo.Network, orderer *nwo.Orderer, chaincode chaincode) {
	if chaincode.isLegacy {
		nwo.DeployChaincodeLegacy(n, channelID, orderer, chaincode.Chaincode)
	} else {
		nwo.DeployChaincode(n, channelID, orderer, chaincode.Chaincode)
	}
}

func upgradeChaincode(n *nwo.Network, orderer *nwo.Orderer, chaincode chaincode) {
	if chaincode.isLegacy {
		nwo.UpgradeChaincodeLegacy(n, channelID, orderer, chaincode.Chaincode)
	} else {
		nwo.DeployChaincode(n, channelID, orderer, chaincode.Chaincode)
	}
}

func installChaincode(n *nwo.Network, chaincode chaincode, peer *nwo.Peer) {
	if chaincode.isLegacy {
		nwo.InstallChaincodeLegacy(n, chaincode.Chaincode, peer)
	} else {
		nwo.PackageAndInstallChaincode(n, chaincode.Chaincode, peer)
	}
}

func invokeChaincodeExpectErr(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke, expectedErrMsg string) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(expectedErrMsg))
}

func approveChaincodeForMyOrgExpectErr(n *nwo.Network, orderer *nwo.Orderer, chaincode nwo.Chaincode, expectedErrMsg string, peers ...*nwo.Peer) {
	// used to ensure we only approve once per org
	approvedOrgs := map[string]bool{}
	for _, p := range peers {
		if _, ok := approvedOrgs[p.Organization]; !ok {
			sess, err := n.PeerAdminSession(p, commands.ChaincodeApproveForMyOrg{
				ChannelID:           channelID,
				Orderer:             n.OrdererAddress(orderer, nwo.ListenPort),
				Name:                chaincode.Name,
				Version:             chaincode.Version,
				PackageID:           chaincode.PackageID,
				Sequence:            chaincode.Sequence,
				EndorsementPlugin:   chaincode.EndorsementPlugin,
				ValidationPlugin:    chaincode.ValidationPlugin,
				SignaturePolicy:     chaincode.SignaturePolicy,
				ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
				InitRequired:        chaincode.InitRequired,
				CollectionsConfig:   chaincode.CollectionsConfig,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
			approvedOrgs[p.Organization] = true
			Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(expectedErrMsg))
		}
	}
}

func assertPvtdataPresencePerCollectionConfig1(n *nwo.Network, chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.Peers
	}
	for _, peer := range peers {
		switch peer.Organization {

		case "Org1":
			marblechaincodeutil.AssertPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
			marblechaincodeutil.AssertNotPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)

		case "Org2":
			marblechaincodeutil.AssertPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
			marblechaincodeutil.AssertPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)

		case "Org3":
			marblechaincodeutil.AssertNotPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
			marblechaincodeutil.AssertPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)
		}
	}
}

func assertPvtdataPresencePerCollectionConfig2(n *nwo.Network, chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.Peers
	}
	for _, peer := range peers {
		switch peer.Organization {

		case "Org1":
			marblechaincodeutil.AssertPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
			marblechaincodeutil.AssertNotPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)

		case "Org2", "Org3":
			marblechaincodeutil.AssertPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
			marblechaincodeutil.AssertPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)
		}
	}
}

func assertPvtdataPresencePerCollectionConfig7(n *nwo.Network, chaincodeName, marbleName string, excludedPeer *nwo.Peer, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.Peers
	}
	collectionMPresence := 0
	collectionMPDPresence := 0
	for _, peer := range peers {
		// exclude the peer that invoked originally and count number of peers disseminated to
		if peer != excludedPeer {
			switch peer.Organization {

			case "Org1":
				collectionMPresence += marblechaincodeutil.CheckPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
				marblechaincodeutil.AssertNotPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)

			case "Org2":
				collectionMPresence += marblechaincodeutil.CheckPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
				collectionMPDPresence += marblechaincodeutil.CheckPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)
			case "Org3":
				marblechaincodeutil.AssertNotPresentInCollectionM(n, channelID, chaincodeName, marbleName, peer)
				collectionMPDPresence += marblechaincodeutil.CheckPresentInCollectionMPD(n, channelID, chaincodeName, marbleName, peer)
			}
		}
	}
	Expect(collectionMPresence).To(Equal(1))
	Expect(collectionMPDPresence).To(Equal(1))
}

// deliverEvent contains the response and related info from a DeliverWithPrivateData call
type deliverEvent struct {
	BlockAndPvtData *pb.BlockAndPrivateData
	BlockNum        uint64
	Err             error
}

// getEventFromDeliverService send a request to DeliverWithPrivateData grpc service
// and receive the response
func getEventFromDeliverService(network *nwo.Network, peer *nwo.Peer, channelID string, signingIdentity *nwo.SigningIdentity, blockNum uint64) *deliverEvent {
	ctx, cancelFunc1 := context.WithTimeout(context.Background(), network.EventuallyTimeout)
	defer cancelFunc1()
	eventCh, conn := registerForDeliverEvent(ctx, network, peer, channelID, signingIdentity, blockNum)
	defer conn.Close()
	event := &deliverEvent{}
	Eventually(eventCh, network.EventuallyTimeout).Should(Receive(event))
	Expect(event.Err).NotTo(HaveOccurred())
	return event
}

func registerForDeliverEvent(
	ctx context.Context,
	network *nwo.Network,
	peer *nwo.Peer,
	channelID string,
	signingIdentity *nwo.SigningIdentity,
	blockNum uint64,
) (<-chan deliverEvent, *grpc.ClientConn) {
	// create a grpc.ClientConn
	conn := network.PeerClientConn(peer)

	dp, err := pb.NewDeliverClient(conn).DeliverWithPrivateData(ctx)
	Expect(err).NotTo(HaveOccurred())

	// send a deliver request
	envelope, err := createDeliverEnvelope(channelID, signingIdentity, blockNum)
	Expect(err).NotTo(HaveOccurred())
	err = dp.Send(envelope)
	Expect(err).NotTo(HaveOccurred())
	err = dp.CloseSend()
	Expect(err).NotTo(HaveOccurred())

	// create a goroutine to receive the response in a separate thread
	eventCh := make(chan deliverEvent, 1)
	go receiveDeliverResponse(dp, peer, eventCh)

	return eventCh, conn
}

// receiveDeliverResponse expects to receive the BlockAndPrivateData response for the requested block.
func receiveDeliverResponse(dp pb.Deliver_DeliverWithPrivateDataClient, peer *nwo.Peer, eventCh chan<- deliverEvent) {
	event := deliverEvent{}

	resp, err := dp.Recv()
	if err != nil {
		event.Err = errors.WithMessagef(err, "error receiving deliver response from peer %s", peer.ID())
	}
	switch r := resp.Type.(type) {
	case *pb.DeliverResponse_BlockAndPrivateData:
		event.BlockAndPvtData = r.BlockAndPrivateData
		event.BlockNum = r.BlockAndPrivateData.Block.Header.Number
	case *pb.DeliverResponse_Status:
		event.Err = errors.Errorf("deliver completed with status (%s) before DeliverResponse_BlockAndPrivateData received from peer %s", r.Status, peer.ID())
	default:
		event.Err = errors.Errorf("received unexpected response type (%T) from peer %s", r, peer.ID())
	}

	select {
	case eventCh <- event:
	default:
	}
}

// createDeliverEnvelope creates a deliver request based on the block number.
// blockNum=0 means newest block
func createDeliverEnvelope(channelID string, signingIdentity *nwo.SigningIdentity, blockNum uint64) (*cb.Envelope, error) {
	creator, err := signingIdentity.Serialize()
	if err != nil {
		return nil, err
	}
	header, err := createHeader(cb.HeaderType_DELIVER_SEEK_INFO, channelID, creator)
	if err != nil {
		return nil, err
	}

	// if blockNum is not greater than 0, seek the newest block
	var seekInfo *ab.SeekInfo
	if blockNum > 0 {
		seekInfo = &ab.SeekInfo{
			Start: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{Number: blockNum},
				},
			},
			Stop: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{Number: blockNum},
				},
			},
		}
	} else {
		seekInfo = &ab.SeekInfo{
			Start: &ab.SeekPosition{
				Type: &ab.SeekPosition_Newest{
					Newest: &ab.SeekNewest{},
				},
			},
			Stop: &ab.SeekPosition{
				Type: &ab.SeekPosition_Newest{
					Newest: &ab.SeekNewest{},
				},
			},
		}
	}

	// create the envelope
	raw := protoutil.MarshalOrPanic(seekInfo)
	payload := &cb.Payload{
		Header: header,
		Data:   raw,
	}
	payloadBytes := protoutil.MarshalOrPanic(payload)
	signature, err := signingIdentity.Sign(payloadBytes)
	if err != nil {
		return nil, err
	}
	return &cb.Envelope{
		Payload:   payloadBytes,
		Signature: signature,
	}, nil
}

func createHeader(txType cb.HeaderType, channelID string, creator []byte) (*cb.Header, error) {
	ts, err := ptypes.TimestampProto(time.Now())
	if err != nil {
		return nil, err
	}
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, err
	}
	chdr := &cb.ChannelHeader{
		Type:      int32(txType),
		ChannelId: channelID,
		TxId:      protoutil.ComputeTxID(nonce, creator),
		Epoch:     0,
		Timestamp: ts,
	}
	chdrBytes := protoutil.MarshalOrPanic(chdr)

	shdr := &cb.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}
	shdrBytes := protoutil.MarshalOrPanic(shdr)
	header := &cb.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	}
	return header, nil
}

// verify collection names and pvtdataMap match expectedKVWritesMap
func assertPrivateDataAsExpected(pvtdataMap map[uint64]*rwset.TxPvtReadWriteSet, expectedKVWritesMap map[string]map[string][]byte) {
	// In the test, each block has only 1 tx, so txSeqInBlock is 0
	txPvtRwset := pvtdataMap[uint64(0)]
	Expect(txPvtRwset.NsPvtRwset).To(HaveLen(1))
	Expect(txPvtRwset.NsPvtRwset[0].Namespace).To(Equal("marblesp"))
	Expect(txPvtRwset.NsPvtRwset[0].CollectionPvtRwset).To(HaveLen(len(expectedKVWritesMap)))

	// verify the collections returned in private data have expected collection names and kvRwset.Writes
	for _, col := range txPvtRwset.NsPvtRwset[0].CollectionPvtRwset {
		Expect(expectedKVWritesMap).To(HaveKey(col.CollectionName))
		expectedKvWrites := expectedKVWritesMap[col.CollectionName]
		kvRwset := kvrwset.KVRWSet{}
		err := proto.Unmarshal(col.GetRwset(), &kvRwset)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvRwset.Writes).To(HaveLen(len(expectedKvWrites)))
		for _, kvWrite := range kvRwset.Writes {
			Expect(expectedKvWrites).To(HaveKey(kvWrite.Key))
			Expect(kvWrite.Value).To(Equal(expectedKvWrites[kvWrite.Key]))
		}
	}
}

func getValueForCollectionMarbles(marbleName, color, owner string, size int) []byte {
	marbleJSONasString := `{"docType":"marble","name":"` + marbleName + `","color":"` + color + `","size":` + strconv.Itoa(size) + `,"owner":"` + owner + `"}`
	return []byte(marbleJSONasString)
}

func getValueForCollectionMarblePrivateDetails(marbleName string, price int) []byte {
	marbleJSONasString := `{"docType":"marblePrivateDetails","name":"` + marbleName + `","price":` + strconv.Itoa(price) + `}`
	return []byte(marbleJSONasString)
}

// fetchBlocksForPeer attempts to fetch the newest block on the given peer.
// It skips the orderer and returns the session's Err buffer for parsing.
func fetchBlocksForPeer(n *nwo.Network, peer *nwo.Peer, user string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerUserSession(peer, user, commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  channelID,
			OutputFile: filepath.Join(n.RootDir, "newest_block.pb"),
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
	currentConfig := nwo.GetConfig(network, network.Peers[0], orderer, channelID)
	updatedConfig := proto.Clone(currentConfig).(*cb.Config)

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
	updatedConfig.ChannelGroup.Groups["Application"].Groups[org.Name].Values["MSP"].Value = protoutil.MarshalOrPanic(
		&mspp.MSPConfig{
			Type:   oldConfig.Type,
			Config: protoutil.MarshalOrPanic(oldMspConfig),
		})
	nwo.UpdateConfig(network, orderer, channelID, currentConfig, updatedConfig, false, network.Peer(org.Name, "peer0"))
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
