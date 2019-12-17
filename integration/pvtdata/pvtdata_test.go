/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdata

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	docker "github.com/fsouza/go-dockerclient"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

// The chaincode used in these tests has two collections defined:
// collectionMarbles and collectionMarblePrivateDetails
// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles
// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails

const channelID = "testchannel"

var _ bool = Describe("PrivateData", func() {
	var (
		network *nwo.Network
		process ifrit.Process

		orderer *nwo.Orderer
	)

	AfterEach(func() {
		testCleanup(network, process)
	})

	Describe("Dissemination when pulling is disabled", func() {
		It("disseminates private data per collections_config1", func() {
			By("setting up the network")
			network = initThreeOrgsSetup()

			By("setting the pull retry threshold to 0 on all peers")
			// set pull retry threshold to 0
			for _, p := range network.Peers {
				core := network.ReadPeerConfig(p)
				core.Peer.Gossip.PvtData.PullRetryThreshold = 0
				network.WritePeerConfig(p, core)
			}

			By("starting the network")
			process, orderer = startNetwork(network)

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
					CollectionsConfig: collectionConfig("collections_config1.json"),
				},
				isLegacy: true,
			}
			deployChaincode(network, orderer, testChaincode)
			addMarble(network, orderer, testChaincode.Name,
				`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`,
				network.Peer("Org1", "peer0"),
			)

			assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble1")
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
			network = initThreeOrgsSetup()
			legacyChaincode = nwo.Chaincode{
				Name:    "marblesp",
				Version: "1.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
				Ctor:    `{"Args":["init"]}`,
				Policy:  `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				// collections_config1.json defines the access as follows:
				// 1. collectionMarbles - Org1, Org2 have access to this collection
				// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
				CollectionsConfig: collectionConfig("collections_config1.json"),
			}

			newLifecycleChaincode = nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
				Lang:              "binary",
				PackageFile:       filepath.Join(network.RootDir, "marbles-pvtdata.tar.gz"),
				Label:             "marbles-private-20",
				SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				CollectionsConfig: collectionConfig("collections_config1.json"),
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

			process, orderer = startNetwork(network)
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
				addMarble(network, orderer, testChaincode.Name,
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
						testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
						if !testChaincode.isLegacy {
							testChaincode.Sequence = "2"
						}
						upgradeChaincode(network, orderer, testChaincode)
						addMarble(network, orderer, testChaincode.Name,
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

			When("chaincode in legacy lifecycle", func() {
				assertReconcileBehavior()
			})

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

			assertBlockToLiveBehavior := func() {
				eligiblePeer := network.Peer("Org2", "peer0")
				ccName := testChaincode.Name
				By("adding three blocks")
				for i := 0; i < 3; i++ {
					addMarble(network, orderer, ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
				}

				By("verifying that marble1 still not purged in collection MarblesPD")
				assertPresentInCollectionMPD(network, ccName, "marble1", eligiblePeer)

				By("adding one more block")
				addMarble(network, orderer, ccName, `{"name":"fun-marble-3", "color":"blue", "size":35, "owner":"tom", "price":99}`, eligiblePeer)

				By("verifying that marble1 purged in collection MarblesPD")
				assertDoesNotExistInCollectionMPD(network, ccName, "marble1", eligiblePeer)

				By("verifying that marble1 still not purged in collection Marbles")
				assertPresentInCollectionM(network, ccName, "marble1", eligiblePeer)

				By("adding new peer that is eligible to recieve data")
				newPeerProcess = addPeer(network, orderer, org2Peer1)
				installChaincode(network, testChaincode, org2Peer1)
				network.VerifyMembership(network.Peers, channelID, ccName)
				assertPresentInCollectionM(network, ccName, "marble1", org2Peer1)
				assertDoesNotExistInCollectionMPD(network, ccName, "marble1", org2Peer1)
			}

			When("chaincode in legacy lifecycle", func() {
				It("purges private data after BTL and causes new peer not to pull the purged private data", func() {
					By("deploying legacy chaincode and adding marble1")
					testChaincode = chaincode{
						Chaincode: legacyChaincode,
						isLegacy:  true,
					}

					testChaincode.CollectionsConfig = collectionConfig("short_btl_config.json")
					deployChaincode(network, orderer, testChaincode)
					addMarble(network, orderer, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))

					assertBlockToLiveBehavior()
				})
			})

			When("chaincode in new lifecycle", func() {
				It("purges private data after BTL and causes new peer not to pull the purged private data", func() {
					By("deploying new lifecycle chaincode and adding marble1")
					testChaincode = chaincode{
						Chaincode: newLifecycleChaincode,
						isLegacy:  false,
					}
					nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)

					testChaincode.CollectionsConfig = collectionConfig("short_btl_config.json")
					deployChaincode(network, orderer, testChaincode)
					addMarble(network, orderer, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))

					assertBlockToLiveBehavior()
				})
			})
		})

		Describe("Org removal from collection", func() {
			assertOrgRemovalBehavior := func() {
				By("upgrading chaincode to remove org3 from collectionMarbles")
				testChaincode.CollectionsConfig = collectionConfig("collections_config1.json")
				testChaincode.Version = "1.1"
				if !testChaincode.isLegacy {
					testChaincode.Sequence = "2"
				}
				upgradeChaincode(network, orderer, testChaincode)
				addMarble(network, orderer, testChaincode.Name, `{"name":"marble2", "color":"yellow", "size":53, "owner":"jerry", "price":22}`, network.Peer("Org2", "peer0"))
				assertPvtdataPresencePerCollectionConfig1(network, testChaincode.Name, "marble2")
			}

			When("chaincode in legacy lifecycle", func() {
				It("causes removed org not to get new data", func() {
					By("deploying legacy chaincode and adding marble1")
					testChaincode = chaincode{
						Chaincode: legacyChaincode,
						isLegacy:  true,
					}
					testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
					deployChaincode(network, orderer, testChaincode)
					addMarble(network, orderer, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))
					assertPvtdataPresencePerCollectionConfig2(network, testChaincode.Name, "marble1")

					assertOrgRemovalBehavior()
				})
			})

			When("chaincode in new lifecycle", func() {
				It("causes removed org not to get new data", func() {
					By("deploying new lifecycle chaincode and adding marble1")
					testChaincode = chaincode{
						Chaincode: newLifecycleChaincode,
						isLegacy:  false,
					}
					nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
					testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
					deployChaincode(network, orderer, testChaincode)
					addMarble(network, orderer, testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("Org2", "peer0"))
					assertPvtdataPresencePerCollectionConfig2(network, testChaincode.Name, "marble1")

					assertOrgRemovalBehavior()
				})
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

				newLifecycleChaincode.CollectionsConfig = collectionConfig("short_btl_config.json")
				newLifecycleChaincode.PackageID = "test-package-id"

				approveChaincodeForMyOrgExpectErr(
					network,
					orderer,
					newLifecycleChaincode,
					`the BlockToLive in an existing collection \[collectionMarblePrivateDetails\] modified. Existing value \[1000000\]`,
					network.Peer("Org2", "peer0"))
			})
		})

		Describe("marble APIs invocation and private data delivery", func() {
			// call marble APIs: getMarblesByRange, transferMarble, delete, getMarbleHash, getMarblePrivateDetailsHash and verify ACL Behavior
			assertMarbleAPIs := func() {
				eligiblePeer := network.Peer("Org2", "peer0")
				ccName := testChaincode.Name

				// Verifies marble private chaincode APIs: getMarblesByRange, transferMarble, delete

				By("adding five marbles")
				for i := 0; i < 5; i++ {
					addMarble(network, orderer, ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
				}

				By("getting marbles by range")
				assertGetMarblesByRange(network, ccName, `"test-marble-0", "test-marble-2"`, eligiblePeer)

				By("transferring test-marble-0 to jerry")
				transferMarble(network, orderer, ccName, `{"name":"test-marble-0", "owner":"jerry"}`, eligiblePeer)

				By("verifying the new ownership of test-marble-0")
				assertOwnershipInCollectionM(network, ccName, `test-marble-0`, eligiblePeer)

				By("deleting test-marble-0")
				deleteMarble(network, orderer, ccName, `{"name":"test-marble-0"}`, eligiblePeer)

				By("verifying the deletion of test-marble-0")
				assertDoesNotExistInCollectionM(network, ccName, `test-marble-0`, eligiblePeer)

				// This section verifies that chaincode can return private data hash.
				// Unlike private data that can only be accessed from authorized peers as defined in the collection config,
				// private data hash can be queried on any peer in the channel that has the chaincode instantiated.
				// When calling QueryChaincode with "getMarbleHash", the cc will return the private data hash in collectionMarbles.
				// When calling QueryChaincode with "getMarblePrivateDetailsHash", the cc will return the private data hash in collectionMarblePrivateDetails.

				peerList := []*nwo.Peer{
					network.Peer("Org1", "peer0"),
					network.Peer("Org2", "peer0"),
					network.Peer("Org3", "peer0")}

				By("verifying getMarbleHash is accessible from all peers that has the chaincode instantiated")
				expectedBytes := util.ComputeStringHash(`{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}`)
				assertMarblesPrivateHashM(network, ccName, "test-marble-1", expectedBytes, peerList)

				By("verifying getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated")
				expectedBytes = util.ComputeStringHash(`{"docType":"marblePrivateDetails","name":"test-marble-1","price":99}`)
				assertMarblesPrivateDetailsHashMPD(network, ccName, "test-marble-1", expectedBytes, peerList)

				// collection ACL while reading private data: not allowed to non-members
				// collections_config3: collectionMarblePrivateDetails - member_only_read is set to true

				By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, shouldn't have read access")
				assertNoReadAccessToCollectionMPD(network, testChaincode.Name, "test-marble-1", network.Peer("Org1", "peer0"))
			}

			// verify DeliverWithPrivateData sends private data based on the ACL in collection config
			// before and after upgrade.
			assertDeliverWithPrivateDataACLBehavior := func() {
				By("getting signing identity for a user in org1")
				signingIdentity := getSigningIdentity(network, "Org1", "User1", "Org1MSP", "bccsp")

				By("adding a marble")
				peer := network.Peer("Org2", "peer0")
				addMarble(network, orderer, testChaincode.Name, `{"name":"marble11", "color":"blue", "size":35, "owner":"tom", "price":99}`, peer)

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
				testChaincode.CollectionsConfig = collectionConfig("collections_config1.json")
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
				addMarble(network, orderer, testChaincode.Name,
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

			When("chaincode in legacy lifecycle", func() {
				It("calls marbles APIs and delivers private data", func() {
					By("deploying legacy chaincode")
					testChaincode = chaincode{
						Chaincode: legacyChaincode,
						isLegacy:  true,
					}
					testChaincode.CollectionsConfig = collectionConfig("collections_config3.json")
					deployChaincode(network, orderer, testChaincode)

					assertMarbleAPIs()
					assertDeliverWithPrivateDataACLBehavior()
				})
			})

			When("chaincode in new lifecycle", func() {
				It("calls marbles APIs and delivers private data", func() {
					By("deploying new lifecycle chaincode")
					testChaincode = chaincode{
						Chaincode: newLifecycleChaincode,
						isLegacy:  false,
					}
					nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
					testChaincode.CollectionsConfig = collectionConfig("collections_config3.json")
					deployChaincode(network, orderer, testChaincode)

					assertMarbleAPIs()
					assertDeliverWithPrivateDataACLBehavior()
				})
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
					testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")

					By("deploying legacy chaincode")
					deployChaincode(network, orderer, testChaincode)

					By("adding marble1 with an org 1 peer as endorser")
					peer := network.Peer("Org1", "peer0")
					marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
					addMarble(network, orderer, testChaincode.Name, marbleDetails, peer)
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
						testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")

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
							Ctor:      fmt.Sprintf(`{"Args":["initMarble"]}`),
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
							testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")

							By("setting the chaincode endorsement policy to org1 or org2 peers")
							testChaincode.SignaturePolicy = `OR ('Org1MSP.member','Org2MSP.member')`

							By("deploying new lifecycle chaincode")
							// set collection endorsement policy to org2 or org3
							deployChaincode(network, orderer, testChaincode)

							By("adding marble1 with an org3 peer as endorser")
							peer := network.Peer("Org3", "peer0")
							marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
							addMarble(network, orderer, testChaincode.Name, marbleDetails, peer)
						})
					})

					When("the collection endorsement policy is a channel config policy reference", func() {
						It("successfully invokes the chaincode", func() {
							// collection config endorsement policy specifies channel config policy reference /Channel/Application/Readers
							By("setting the collection config endorsement policy to use a channel config policy reference")
							testChaincode.CollectionsConfig = collectionConfig("collections_config5.json")

							By("setting the channel endorsement policy to org1 or org2 peers")
							testChaincode.SignaturePolicy = `OR ('Org1MSP.member','Org2MSP.member')`

							By("deploying new lifecycle chaincode")
							deployChaincode(network, orderer, testChaincode)

							By("adding marble1 with an org3 peer as endorser")
							peer := network.Peer("Org3", "peer0")
							marbleDetails := `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`
							addMarble(network, orderer, testChaincode.Name, marbleDetails, peer)
						})
					})
				})

				When("the collection config endorsement policy specifies a semantically wrong, but well formed signature policy", func() {
					It("fails to invoke the chaincode with an endorsement policy failure", func() {
						By("setting the collection config endorsement policy to non existent org4 peers")
						testChaincode.CollectionsConfig = collectionConfig("collections_config6.json")

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
							Ctor:      fmt.Sprintf(`{"Args":["initMarble"]}`),
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
})

func initThreeOrgsSetup() *nwo.Network {
	var err error
	testDir, err := ioutil.TempDir("", "e2e-pvtdata")
	Expect(err).NotTo(HaveOccurred())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	config := nwo.BasicSolo()

	// add org3 with one peer
	config.Organizations = append(config.Organizations, &nwo.Organization{
		Name:          "Org3",
		MSPID:         "Org3MSP",
		Domain:        "org3.example.com",
		EnableNodeOUs: true,
		Users:         2,
		CA:            &nwo.CA{Hostname: "ca"},
	})
	config.Consortiums[0].Organizations = append(config.Consortiums[0].Organizations, "Org3")
	config.Profiles[1].Organizations = append(config.Profiles[1].Organizations, "Org3")
	config.Peers = append(config.Peers, &nwo.Peer{
		Name:         "peer0",
		Organization: "Org3",
		Channels: []*nwo.PeerChannel{
			{Name: channelID, Anchor: true},
		},
	})

	n := nwo.New(config, testDir, client, StartPort(), components)
	n.GenerateConfigTree()

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

func startNetwork(n *nwo.Network) (ifrit.Process, *nwo.Orderer) {
	n.Bootstrap()
	networkRunner := n.NetworkGroupRunner()
	process := ifrit.Invoke(networkRunner)
	Eventually(process.Ready(), n.EventuallyTimeout).Should(BeClosed())

	orderer := n.Orderer("orderer")
	n.CreateAndJoinChannel(orderer, channelID)
	n.UpdateChannelAnchors(orderer, channelID)

	By("verifying membership")
	n.VerifyMembership(n.Peers, channelID)

	return process, orderer
}

func testCleanup(network *nwo.Network, process ifrit.Process) {
	if process != nil {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
	}
	if network != nil {
		network.Cleanup()
	}
	os.RemoveAll(network.RootDir)
}

func collectionConfig(collConfigFile string) string {
	return filepath.Join("testdata", "collection_configs", collConfigFile)
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

func queryChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeQuery, expectedMessage string, expectSuccess bool) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMessage))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedMessage))
	}
}

func invokeChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
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

func addMarble(n *nwo.Network, orderer *nwo.Orderer, chaincodeName, marbleDetails string, peer *nwo.Peer) {
	marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDetails))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["initMarble"]}`),
		Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

func deleteMarble(n *nwo.Network, orderer *nwo.Orderer, chaincodeName, marbleDelete string, peer *nwo.Peer) {
	marbleDeleteBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDelete))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["delete"]}`),
		Transient: fmt.Sprintf(`{"marble_delete":"%s"}`, marbleDeleteBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

func transferMarble(n *nwo.Network, orderer *nwo.Orderer, chaincodeName, marbleOwner string, peer *nwo.Peer) {
	marbleOwnerBase64 := base64.StdEncoding.EncodeToString([]byte(marbleOwner))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["transferMarble"]}`),
		Transient: fmt.Sprintf(`{"marble_owner":"%s"}`, marbleOwnerBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

func assertPvtdataPresencePerCollectionConfig1(n *nwo.Network, chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.Peers
	}
	for _, peer := range peers {
		switch peer.Organization {

		case "Org1":
			assertPresentInCollectionM(n, chaincodeName, marbleName, peer)
			assertNotPresentInCollectionMPD(n, chaincodeName, marbleName, peer)

		case "Org2":
			assertPresentInCollectionM(n, chaincodeName, marbleName, peer)
			assertPresentInCollectionMPD(n, chaincodeName, marbleName, peer)

		case "Org3":
			assertNotPresentInCollectionM(n, chaincodeName, marbleName, peer)
			assertPresentInCollectionMPD(n, chaincodeName, marbleName, peer)
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
			assertPresentInCollectionM(n, chaincodeName, marbleName, peer)
			assertNotPresentInCollectionMPD(n, chaincodeName, marbleName, peer)

		case "Org2", "Org3":
			assertPresentInCollectionM(n, chaincodeName, marbleName, peer)
			assertPresentInCollectionMPD(n, chaincodeName, marbleName, peer)
		}
	}
}

// assertGetMarblesByRange asserts that
func assertGetMarblesByRange(n *nwo.Network, chaincodeName, marbleRange string, peer *nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarblesByRange", %s]}`, marbleRange)
	expectedMsg := `\Q[{"Key":"test-marble-0", "Record":{"docType":"marble","name":"test-marble-0","color":"blue","size":35,"owner":"tom"}},{"Key":"test-marble-1", "Record":{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}}]\E`
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, true, peer)
}

// assertPresentInCollectionM asserts that the private data for given marble is present in collection
// 'readMarble' at the given peers
func assertPresentInCollectionM(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`{"docType":"marble","name":"%s"`, marbleName)
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, true, peerList...)
}

// assertPresentInCollectionMPD asserts that the private data for given marble is present
// in collection 'readMarblePrivateDetails' at the given peers
func assertPresentInCollectionMPD(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`{"docType":"marblePrivateDetails","name":"%s"`, marbleName)
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, true, peerList...)
}

// assertNotPresentInCollectionM asserts that the private data for given marble is NOT present
// in collection 'readMarble' at the given peers
func assertNotPresentInCollectionM(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := "private data matching public hash version is not available"
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, false, peerList...)
}

// assertNotPresentInCollectionMPD asserts that the private data for given marble is NOT present
// in collection 'readMarblePrivateDetails' at the given peers
func assertNotPresentInCollectionMPD(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "private data matching public hash version is not available"
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, false, peerList...)
}

// assertDoesNotExistInCollectionM asserts that the private data for given marble
// does not exist in collection 'readMarble' (i.e., is never created/has been deleted/has been purged)
func assertDoesNotExistInCollectionM(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := "Marble does not exist"
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, false, peerList...)
}

// assertDoesNotExistInCollectionMPD asserts that the private data for given marble
// does not exist in collection 'readMarblePrivateDetails' (i.e., is never created/has been deleted/has been purged)
func assertDoesNotExistInCollectionMPD(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "Marble private details does not exist"
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, false, peerList...)
}

// assertOwnershipInCollectionM asserts that the private data for given marble is present
// in collection 'readMarble' at the given peers
func assertOwnershipInCollectionM(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`{"docType":"marble","name":"test-marble-0","color":"blue","size":35,"owner":"jerry"}`)
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, true, peerList...)
}

// assertNoReadAccessToCollectionMPD asserts that the orgs of the given peers do not have
// read access to private data for the collection readMarblePrivateDetails
func assertNoReadAccessToCollectionMPD(n *nwo.Network, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "tx creator does not have read access permission"
	queryChaincodePerPeer(n, query, chaincodeName, expectedMsg, false, peerList...)
}

func queryChaincodePerPeer(n *nwo.Network, query, chaincodeName, expectedMsg string, expectSuccess bool, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}
	for _, peer := range peerList {
		queryChaincode(n, peer, command, expectedMsg, expectSuccess)
	}
}

// assertMarblesPrivateHashM asserts that getMarbleHash is accessible from all peers that has the chaincode instantiated
func assertMarblesPrivateHashM(n *nwo.Network, chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarbleHash","%s"]}`, marbleName)
	verifyPvtdataHash(n, query, chaincodeName, peerList, expectedBytes)
}

// assertMarblesPrivateDetailsHashMPD asserts that getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated
func assertMarblesPrivateDetailsHashMPD(n *nwo.Network, chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarblePrivateDetailsHash","%s"]}`, marbleName)
	verifyPvtdataHash(n, query, chaincodeName, peerList, expectedBytes)
}

// verifyPvtdataHash verifies the private data hash matches the expected bytes.
// Cannot reuse verifyAccess because the hash bytes are not valid utf8 causing gbytes.Say to fail.
func verifyPvtdataHash(n *nwo.Network, query, chaincodeName string, peers []*nwo.Peer, expected []byte) {
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}

	for _, peer := range peers {
		sess, err := n.PeerUserSession(peer, "User1", command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		actual := sess.Buffer().Contents()
		// verify actual bytes contain expected bytes - cannot use equal because session may contain extra bytes
		Expect(bytes.Contains(actual, expected)).To(Equal(true))
	}
}

// deliverEvent contains the response and related info from a DeliverWithPrivateData call
type deliverEvent struct {
	BlockAndPvtData *pb.BlockAndPrivateData
	BlockNum        uint64
	Err             error
}

// getEventFromDeliverService send a request to DeliverWithPrivateData grpc service
// and receive the response
func getEventFromDeliverService(network *nwo.Network, peer *nwo.Peer, channelID string, signingIdentity msp.SigningIdentity, blockNum uint64) *deliverEvent {
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
	signingIdentity msp.SigningIdentity,
	blockNum uint64,
) (<-chan deliverEvent, *grpc.ClientConn) {
	// create a comm.GRPCClient
	tlsRootCertFile := filepath.Join(network.PeerLocalTLSDir(peer), "ca.crt")
	caPEM, err := ioutil.ReadFile(tlsRootCertFile)
	Expect(err).NotTo(HaveOccurred())
	clientConfig := comm.ClientConfig{Timeout: 10 * time.Second}
	clientConfig.SecOpts = comm.SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{caPEM},
		RequireClientCert: false,
	}
	grpcClient, err := comm.NewGRPCClient(clientConfig)
	Expect(err).NotTo(HaveOccurred())
	// create a client for DeliverWithPrivateData
	address := network.PeerAddress(peer, nwo.ListenPort)
	conn, err := grpcClient.NewConnection(address)
	Expect(err).NotTo(HaveOccurred())
	dp, err := pb.NewDeliverClient(conn).DeliverWithPrivateData(ctx)
	Expect(err).NotTo(HaveOccurred())
	// send a deliver request
	envelope, err := createDeliverEnvelope(channelID, signingIdentity, blockNum)
	Expect(err).NotTo(HaveOccurred())
	err = dp.Send(envelope)
	dp.CloseSend()
	Expect(err).NotTo(HaveOccurred())
	// create a goroutine to receive the response in a separate thread
	eventCh := make(chan deliverEvent, 1)
	go receiveDeliverResponse(dp, address, eventCh)

	return eventCh, conn
}

func getSigningIdentity(network *nwo.Network, org, user, mspID, mspType string) msp.SigningIdentity {
	peerForOrg := network.Peer(org, "peer0")
	mspConfigPath := network.PeerUserMSPDir(peerForOrg, user)
	mspInstance, err := loadLocalMSPAt(mspConfigPath, mspID, mspType)
	Expect(err).NotTo(HaveOccurred())

	signingIdentity, err := mspInstance.GetDefaultSigningIdentity()
	Expect(err).NotTo(HaveOccurred())
	return signingIdentity
}

// loadLocalMSPAt loads an MSP whose configuration is stored at 'dir', and whose
// id and type are the passed as arguments.
func loadLocalMSPAt(dir, id, mspType string) (msp.MSP, error) {
	if mspType != "bccsp" {
		return nil, errors.Errorf("invalid msp type, expected 'bccsp', got %s", mspType)
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, id)
	if err != nil {
		return nil, err
	}
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	if err != nil {
		return nil, err
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}
	thisMSP, err := msp.NewBccspMspWithKeyStore(msp.MSPv1_0, ks, cryptoProvider)
	if err != nil {
		return nil, err
	}
	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, err
	}
	return thisMSP, nil
}

// receiveDeliverResponse expectes to receive the BlockAndPrivateData response for the requested block.
func receiveDeliverResponse(dp pb.Deliver_DeliverWithPrivateDataClient, address string, eventCh chan<- deliverEvent) error {
	event := deliverEvent{}

	resp, err := dp.Recv()
	if err != nil {
		event.Err = errors.WithMessagef(err, "error receiving deliver response from peer %s\n", address)
	}
	switch r := resp.Type.(type) {
	case *pb.DeliverResponse_BlockAndPrivateData:
		event.BlockAndPvtData = r.BlockAndPrivateData
		event.BlockNum = r.BlockAndPrivateData.Block.Header.Number
	case *pb.DeliverResponse_Status:
		event.Err = errors.Errorf("deliver completed with status (%s) before DeliverResponse_BlockAndPrivateData received from peer %s", r.Status, address)
	default:
		event.Err = errors.Errorf("received unexpected response type (%T) from peer %s", r, address)
	}

	select {
	case eventCh <- event:
	default:
	}
	return nil
}

// createDeliverEnvelope creates a deliver request based on the block number.
// blockNum=0 means newest block
func createDeliverEnvelope(channelID string, signingIdentity msp.SigningIdentity, blockNum uint64) (*cb.Envelope, error) {
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
