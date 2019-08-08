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
	yaml "gopkg.in/yaml.v2"
)

// The chaincode used in these tests has two collections defined:
// collectionMarbles and collectionMarblePrivateDetails
// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles
// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails

var _ bool = Describe("PrivateData", func() {
	var (
		testDir  string
		network  *nwo.Network
		process  ifrit.Process
		orderer  *nwo.Orderer
		allPeers []*nwo.Peer

		legacyChaincode       nwo.Chaincode
		newLifecycleChaincode nwo.Chaincode
		testChaincode         chaincode
		helper                *testHelper
	)

	BeforeEach(func() {
		testDir, network, process, orderer, allPeers = initThreeOrgsSetup()
		helper = &testHelper{
			networkHelper: &networkHelper{
				Network:   network,
				orderer:   orderer,
				peers:     allPeers,
				testDir:   testDir,
				channelID: "testchannel",
			},
		}

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
			PackageFile:       filepath.Join(testDir, "marbles-pvtdata.tar.gz"),
			Label:             "marbles-private-20",
			SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			CollectionsConfig: collectionConfig("collections_config1.json"),
			Sequence:          "1",
		}
	})

	AfterEach(func() {
		testCleanup(testDir, network, process)
	})

	Describe("Reconciliation", func() {
		BeforeEach(func() {
			By("deploying legacy chaincode and adding marble1")
			testChaincode = chaincode{
				Chaincode: legacyChaincode,
				isLegacy:  true,
			}
			helper.deployChaincode(testChaincode)
			helper.addMarble(testChaincode.Name,
				`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`,
				network.Peer("org1", "peer0"),
			)
		})

		assertReconcileBehavior := func() {
			It("disseminates private data per collections_config1", func() {
				helper.assertPvtdataPresencePerCollectionConfig1(testChaincode.Name, "marble1")
			})

			When("org3 is added to collectionMarbles via chaincode upgrade with collections_config2", func() {
				BeforeEach(func() {
					// collections_config2.json defines the access as follows:
					// 1. collectionMarbles - Org1, Org2 and Org3 have access to this collection
					// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
					// the change from collections_config1 - org3 was added to collectionMarbles
					testChaincode.Version = "1.1"
					testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
					if !testChaincode.isLegacy {
						testChaincode.Sequence = "2"
					}
					helper.upgradeChaincode(testChaincode)
				})

				It("distributes and allows access to newly added private data per collections_config2", func() {
					helper.addMarble(testChaincode.Name,
						`{"name":"marble2", "color":"yellow", "size":53, "owner":"jerry", "price":22}`,
						network.Peer("org2", "peer0"),
					)
					helper.assertPvtdataPresencePerCollectionConfig2(testChaincode.Name, "marble2")
				})
			})

			When("a new peer in org1 joins the channel", func() {
				var (
					newPeer *nwo.Peer
				)
				BeforeEach(func() {
					newPeer = network.Peer("org1", "peer1")
					helper.addPeer(newPeer)
					allPeers = append(allPeers, newPeer)
					helper.installChaincode(testChaincode, newPeer)
					network.VerifyMembership(allPeers, "testchannel", "marblesp")
				})

				It("causes the new peer to receive the existing private data only for collectionMarbles", func() {
					helper.assertPvtdataPresencePerCollectionConfig1(testChaincode.Name, "marble1", newPeer)
				})
			})
		}

		Context("chaincode in legacy lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
			})
			assertReconcileBehavior()
		})

		Context("chaincode is migrated from legacy to new lifecycle with same collection config", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
				helper.upgradeChaincode(testChaincode)
			})
			assertReconcileBehavior()
		})
	})

	Describe("BlockToLive", func() {
		assertBlockToLiveBehavior := func() {
			It("purges private data after BTL and causes new peer not to pull the purged private data", func() {
				testChaincode.CollectionsConfig = collectionConfig("short_btl_config.json")
				eligiblePeer := network.Peer("org2", "peer0")
				ccName := testChaincode.Name

				By("deploying chaincode and adding marble1")
				helper.deployChaincode(testChaincode)
				helper.addMarble(ccName, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, eligiblePeer)

				By("adding three blocks")
				for i := 0; i < 3; i++ {
					helper.addMarble(ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
				}

				By("verifying that marble1 still not purged in collection MarblesPD")
				helper.assertPresentInCollectionMPD(ccName, "marble1", eligiblePeer)

				By("adding one more block")
				helper.addMarble(ccName, `{"name":"fun-marble-3", "color":"blue", "size":35, "owner":"tom", "price":99}`, eligiblePeer)

				By("verifying that marble1 purged in collection MarblesPD")
				helper.assertDoesNotExistInCollectionMPD(ccName, "marble1", eligiblePeer)

				By("verifying that marble1 still not purged in collection Marbles")
				helper.assertPresentInCollectionM(ccName, "marble1", eligiblePeer)

				By("adding new peer that is eligible to recieve data")
				newEligiblePeer := network.Peer("org2", "peer1")
				helper.addPeer(newEligiblePeer)
				allPeers = append(allPeers, newEligiblePeer)
				helper.installChaincode(testChaincode, newEligiblePeer)
				helper.VerifyMembership(allPeers, "testchannel", ccName)
				helper.assertPresentInCollectionM(ccName, "marble1", newEligiblePeer)
				helper.assertDoesNotExistInCollectionMPD(ccName, "marble1", newEligiblePeer)
			})
		}

		Context("chaincode in legacy lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
			})
			assertBlockToLiveBehavior()
		})

		Context("chaincode in new lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
			})
			assertBlockToLiveBehavior()
		})
	})

	Describe("Org removal from collection", func() {
		assertOrgRemovalBehavior := func() {
			It("causes removed org not to get new data", func() {
				testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
				helper.deployChaincode(testChaincode)
				helper.addMarble(testChaincode.Name, `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, network.Peer("org2", "peer0"))
				helper.assertPvtdataPresencePerCollectionConfig2(testChaincode.Name, "marble1")

				By("upgrading chaincode to remove org3 from collectionMarbles")
				testChaincode.CollectionsConfig = collectionConfig("collections_config1.json")
				testChaincode.Version = "1.1"
				if !testChaincode.isLegacy {
					testChaincode.Sequence = "2"
				}
				helper.upgradeChaincode(testChaincode)
				helper.addMarble(testChaincode.Name, `{"name":"marble2", "color":"yellow", "size":53, "owner":"jerry", "price":22}`, network.Peer("org2", "peer0"))
				helper.assertPvtdataPresencePerCollectionConfig1(testChaincode.Name, "marble2")
			})
		}

		Context("chaincode in legacy lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
			})
			assertOrgRemovalBehavior()
		})

		Context("chaincode in new lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
			})
			assertOrgRemovalBehavior()
		})
	})

	Describe("Collection Config Updates", func() {
		BeforeEach(func() {
			By("deploying legacy chaincode")
			testChaincode = chaincode{
				Chaincode: legacyChaincode,
				isLegacy:  true,
			}
			helper.deployChaincode(testChaincode)
		})

		When("migrating a chaincode from legacy lifecycle to new lifecycle", func() {
			BeforeEach(func() {
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
				newLifecycleChaincode.CollectionsConfig = collectionConfig("short_btl_config.json")
				newLifecycleChaincode.PackageID = "test-package-id"
			})

			It("performs check against collection config from legacy lifecycle", func() {
				helper.approveChaincodeForMyOrgExpectErr(
					newLifecycleChaincode,
					`the BlockToLive in an existing collection \[collectionMarblePrivateDetails\] modified. Existing value \[1000000\]`,
					network.Peer("org2", "peer0"))
			})
		})
	})

	Describe("marble APIs invocation and private data delivery", func() {
		// call marble APIs: getMarblesByRange, transferMarble, delete, getMarbleHash, getMarblePrivateDetailsHash and verify ACL Behavior
		assertMarbleAPIs := func() {
			eligiblePeer := network.Peer("org2", "peer0")
			ccName := testChaincode.Name

			// Verifies marble private chaincode APIs: getMarblesByRange, transferMarble, delete

			By("adding five marbles")
			for i := 0; i < 5; i++ {
				helper.addMarble(ccName, fmt.Sprintf(`{"name":"test-marble-%d", "color":"blue", "size":35, "owner":"tom", "price":99}`, i), eligiblePeer)
			}

			By("getting marbles by range")
			expectedMsg := `\Q[{"Key":"test-marble-0", "Record":{"docType":"marble","name":"test-marble-0","color":"blue","size":35,"owner":"tom"}},{"Key":"test-marble-1", "Record":{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}}]\E`
			helper.assertGetMarblesByRange(ccName, `"test-marble-0", "test-marble-2"`, expectedMsg, eligiblePeer)

			By("transferring test-marble-0 to jerry")
			helper.transferMarble(ccName, `{"name":"test-marble-0", "owner":"jerry"}`, eligiblePeer)

			By("verifying the new ownership of test-marble-0")
			expectedMsg = fmt.Sprintf(`{"docType":"marble","name":"test-marble-0","color":"blue","size":35,"owner":"jerry"}`)
			helper.assertOwnershipInCollectionM(ccName, `test-marble-0`, expectedMsg, eligiblePeer)

			By("deleting test-marble-0")
			helper.deleteMarble(ccName, `{"name":"test-marble-0"}`, eligiblePeer)

			By("verifying the deletion of test-marble-0")
			helper.assertDoesNotExistInCollectionM(ccName, `test-marble-0`, eligiblePeer)

			// This section verifies that chaincode can return private data hash.
			// Unlike private data that can only be accessed from authorized peers as defined in the collection config,
			// private data hash can be queried on any peer in the channel that has the chaincode instantiated.
			// When calling QueryChaincode with "getMarbleHash", the cc will return the private data hash in collectionMarbles.
			// When calling QueryChaincode with "getMarblePrivateDetailsHash", the cc will return the private data hash in collectionMarblePrivateDetails.

			peerList := []*nwo.Peer{
				network.Peer("org1", "peer0"),
				network.Peer("org2", "peer0"),
				network.Peer("org3", "peer0")}

			By("verifying getMarbleHash is accessible from all peers that has the chaincode instantiated")
			expectedBytes := util.ComputeStringHash(`{"docType":"marble","name":"test-marble-1","color":"blue","size":35,"owner":"tom"}`)
			helper.assertMarblesPrivateHashM(ccName, "test-marble-1", expectedBytes, peerList)

			By("verifying getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated")
			expectedBytes = util.ComputeStringHash(`{"docType":"marblePrivateDetails","name":"test-marble-1","price":99}`)
			helper.assertMarblesPrivateDetailsHashMPD(ccName, "test-marble-1", expectedBytes, peerList)

			// collection ACL while reading private data: not allowed to non-members
			// collections_config4: collectionMarblePrivateDetails - member_only_read is set to true

			By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, shouldn't have read access")
			helper.assertNoReadAccessToCollectionMPD(testChaincode.Name, "test-marble-1", network.Peer("org1", "peer0"))
		}

		// verify DeliverWithPrivateData sends private data based on the ACL in collection config
		// before and after upgrade.
		assertDeliverWithPrivateDataACLBehavior := func() {
			By("getting signing identity for a user in org1")
			signingIdentity := getSigningIdentity(network, "org1", "User1", "Org1MSP", "bccsp")

			By("adding a marble")
			peer := network.Peer("org2", "peer0")
			helper.addMarble(testChaincode.Name, `{"name":"marble11", "color":"blue", "size":35, "owner":"tom", "price":99}`, peer)

			By("getting the deliver event for newest block")
			event := getEventFromDeliverService(network, peer, "testchannel", signingIdentity, 0)

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
			helper.upgradeChaincode(testChaincode)

			By("getting the deliver event for an old block committed before upgrade")
			event = getEventFromDeliverService(network, peer, "testchannel", signingIdentity, event.BlockNum)

			By("verifying the deliver event for the old block uses old config")
			assertPrivateDataAsExpected(event.BlockAndPvtData.PrivateDataMap, expectedKVWritesMap)

			By("adding a new marble after upgrade")
			helper.addMarble(testChaincode.Name,
				`{"name":"marble12", "color":"blue", "size":35, "owner":"tom", "price":99}`,
				network.Peer("org1", "peer0"),
			)
			By("getting the deliver event for a new block committed after upgrade")
			event = getEventFromDeliverService(network, peer, "testchannel", signingIdentity, 0)

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

		Context("chaincode in legacy lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
				testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")
				helper.deployChaincode(testChaincode)
			})

			It("calls marbles APIs and delivers private data", func() {
				assertMarbleAPIs()
				assertDeliverWithPrivateDataACLBehavior()
			})
		})

		Context("chaincode in new lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
				testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")
				helper.deployChaincode(testChaincode)
			})

			It("calls marbles APIs and delivers private data", func() {
				assertMarbleAPIs()
				assertDeliverWithPrivateDataACLBehavior()
			})
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

func collectionConfig(collConfigFile string) string {
	return filepath.Join("testdata", "collection_configs", collConfigFile)
}

type chaincode struct {
	nwo.Chaincode
	isLegacy bool
}

type networkHelper struct {
	*nwo.Network
	orderer   *nwo.Orderer
	peers     []*nwo.Peer
	channelID string
	testDir   string
}

func (nh *networkHelper) addPeer(peer *nwo.Peer) {
	nh.JoinChannel(nh.channelID, nh.orderer, peer)
	peer.Channels = append(peer.Channels, &nwo.PeerChannel{Name: nh.channelID, Anchor: false})
	ledgerHeight := nwo.GetLedgerHeight(nh.Network, nh.peers[0], nh.channelID)
	sess, err := nh.PeerAdminSession(
		peer,
		commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  nh.channelID,
			Orderer:    nh.OrdererAddress(nh.orderer, nwo.ListenPort),
			OutputFile: filepath.Join(nh.testDir, "newest_block.pb"),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

	nh.peers = append(nh.peers, peer)
	nwo.WaitUntilEqualLedgerHeight(nh.Network, nh.channelID, nwo.GetLedgerHeight(nh.Network, nh.peers[0], nh.channelID), nh.peers...)
}

func (nh *networkHelper) deployChaincode(chaincode chaincode) {
	if chaincode.isLegacy {
		nwo.DeployChaincodeLegacy(nh.Network, nh.channelID, nh.orderer, chaincode.Chaincode)
	} else {
		nwo.DeployChaincode(nh.Network, nh.channelID, nh.orderer, chaincode.Chaincode)
	}
}

func (nh *networkHelper) upgradeChaincode(chaincode chaincode) {
	if chaincode.isLegacy {
		nwo.UpgradeChaincodeLegacy(nh.Network, nh.channelID, nh.orderer, chaincode.Chaincode)
	} else {
		nwo.DeployChaincode(nh.Network, nh.channelID, nh.orderer, chaincode.Chaincode)
	}
}

func (nh *networkHelper) installChaincode(chaincode chaincode, peer *nwo.Peer) {
	if chaincode.isLegacy {
		nwo.InstallChaincodeLegacy(nh.Network, chaincode.Chaincode, peer)
	} else {
		nwo.PackageAndInstallChaincode(nh.Network, chaincode.Chaincode, peer)
	}
}

func (nh *networkHelper) queryChaincode(peer *nwo.Peer, command commands.ChaincodeQuery, expectedMessage string, expectSuccess bool) {
	sess, err := nh.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMessage))
	} else {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedMessage))
	}
}

func (nh *networkHelper) invokeChaincode(peer *nwo.Peer, command commands.ChaincodeInvoke) {
	sess, err := nh.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func (nh *networkHelper) approveChaincodeForMyOrgExpectErr(chaincode nwo.Chaincode, expectedErrMsg string, peers ...*nwo.Peer) {
	// used to ensure we only approve once per org
	approvedOrgs := map[string]bool{}
	for _, p := range peers {
		if _, ok := approvedOrgs[p.Organization]; !ok {
			sess, err := nh.PeerAdminSession(p, commands.ChaincodeApproveForMyOrg{
				ChannelID:           nh.channelID,
				Orderer:             nh.OrdererAddress(nh.orderer, nwo.ListenPort),
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
			Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
			approvedOrgs[p.Organization] = true
			Eventually(sess.Err, nh.EventuallyTimeout).Should(gbytes.Say(expectedErrMsg))
		}
	}
}

type testHelper struct {
	*networkHelper
}

func (th *testHelper) addMarble(chaincodeName, marbleDetails string, peer *nwo.Peer) {
	marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDetails))

	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["initMarble"]}`),
		Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
		PeerAddresses: []string{
			th.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	th.invokeChaincode(peer, command)
	nwo.WaitUntilEqualLedgerHeight(th.Network, th.channelID, nwo.GetLedgerHeight(th.Network, peer, th.channelID), th.peers...)
}

func (th *testHelper) deleteMarble(chaincodeName, marbleDelete string, peer *nwo.Peer) {
	marbleDeleteBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDelete))

	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["delete"]}`),
		Transient: fmt.Sprintf(`{"marble_delete":"%s"}`, marbleDeleteBase64),
		PeerAddresses: []string{
			th.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	th.invokeChaincode(peer, command)
	nwo.WaitUntilEqualLedgerHeight(th.Network, th.channelID, nwo.GetLedgerHeight(th.Network, peer, th.channelID), th.peers...)
}

func (th *testHelper) assertGetMarblesByRange(chaincodeName, marbleRange string, expectedMsg string, peer *nwo.Peer) {

	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["getMarblesByRange", %s]}`, marbleRange),
	}
	th.queryChaincode(peer, command, expectedMsg, true)
}

func (th *testHelper) transferMarble(chaincodeName, marbleOwner string, peer *nwo.Peer) {
	marbleOwnerBase64 := base64.StdEncoding.EncodeToString([]byte(marbleOwner))

	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["transferMarble"]}`),
		Transient: fmt.Sprintf(`{"marble_owner":"%s"}`, marbleOwnerBase64),
		PeerAddresses: []string{
			th.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	th.invokeChaincode(peer, command)
	nwo.WaitUntilEqualLedgerHeight(th.Network, th.channelID, nwo.GetLedgerHeight(th.Network, peer, th.channelID), th.peers...)
}

func (th *testHelper) assertPvtdataPresencePerCollectionConfig1(chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = th.peers
	}
	for _, peer := range peers {
		switch peer.Organization {

		case "org1":
			th.assertPresentInCollectionM(chaincodeName, marbleName, peer)
			th.assertNotPresentInCollectionMPD(chaincodeName, marbleName, peer)

		case "org2":
			th.assertPresentInCollectionM(chaincodeName, marbleName, peer)
			th.assertPresentInCollectionMPD(chaincodeName, marbleName, peer)

		case "org3":
			th.assertNotPresentInCollectionM(chaincodeName, marbleName, peer)
			th.assertPresentInCollectionMPD(chaincodeName, marbleName, peer)
		}
	}
}

func (th *testHelper) assertPvtdataPresencePerCollectionConfig2(chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = th.peers
	}
	for _, peer := range peers {
		switch peer.Organization {

		case "org1":
			th.assertPresentInCollectionM(chaincodeName, marbleName, peer)
			th.assertNotPresentInCollectionMPD(chaincodeName, marbleName, peer)

		case "org2", "org3":
			th.assertPresentInCollectionM(chaincodeName, marbleName, peer)
			th.assertPresentInCollectionMPD(chaincodeName, marbleName, peer)
		}
	}
}

// assertPresentInCollectionM asserts that the private data for given marble is present in collection
// 'readMarble' at the given peers
func (th *testHelper) assertPresentInCollectionM(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	expectedMsg := fmt.Sprintf(`{"docType":"marble","name":"%s"`, marbleName)
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, true)
	}
}

// assertPresentInCollectionMPD asserts that the private data for given marble is present
// in collection 'readMarblePrivateDetails' at the given peers
func (th *testHelper) assertPresentInCollectionMPD(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName),
	}
	expectedMsg := fmt.Sprintf(`{"docType":"marblePrivateDetails","name":"%s"`, marbleName)
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, true)
	}
}

// assertNotPresentInCollectionM asserts that the private data for given marble is NOT present
// in collection 'readMarble' at the given peers
func (th *testHelper) assertNotPresentInCollectionM(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	expectedMsg := "private data matching public hash version is not available"
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, false)
	}
}

// assertNotPresentInCollectionMPD asserts that the private data for given marble is NOT present
// in collection 'readMarblePrivateDetails' at the given peers
func (th *testHelper) assertNotPresentInCollectionMPD(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName),
	}
	expectedMsg := "private data matching public hash version is not available"
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, false)
	}
}

// assertDoesNotExistInCollectionM asserts that the private data for given marble
// does not exist in collection 'readMarble' (i.e., is never created/has been deleted/has been purged)
func (th *testHelper) assertDoesNotExistInCollectionM(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	expectedMsg := "Marble does not exist"
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, false)
	}
}

// assertDoesNotExistInCollectionMPD asserts that the private data for given marble
// does not exist in collection 'readMarblePrivateDetails' (i.e., is never created/has been deleted/has been purged)
func (th *testHelper) assertDoesNotExistInCollectionMPD(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName),
	}
	expectedMsg := "Marble private details does not exist"
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, false)
	}
}

// assertOwnershipInCollectionM asserts that the private data for given marble is present
// in collection 'readMarble' at the given peers
func (th *testHelper) assertOwnershipInCollectionM(chaincodeName, marbleName string, expectedMsg string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}

	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, true)
	}
}

// assertMarblesPrivateHashM asserts that getMarbleHash is accessible from all peers that has the chaincode instantiated
func (th *testHelper) assertMarblesPrivateHashM(chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["getMarbleHash","%s"]}`, marbleName),
	}

	verifyPvtdataHash(th.Network, command, peerList, expectedBytes)
}

// assertMarblesPrivateDetailsHashMPD asserts that getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated
func (th *testHelper) assertMarblesPrivateDetailsHashMPD(chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["getMarblePrivateDetailsHash","%s"]}`, marbleName),
	}

	verifyPvtdataHash(th.Network, command, peerList, expectedBytes)
}

// assertNoReadAccessToCollectionMPD asserts that the orgs of the given peers do not have
// read access to private data for the collection readMarblePrivateDetails
func (th *testHelper) assertNoReadAccessToCollectionMPD(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName),
	}
	expectedMsg := "tx creator does not have read access permission"
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, false)
	}
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

// deliverEvent contains the response and related info from a DeliverWithPrivateData call
type deliverEvent struct {
	BlockAndPvtData *pb.BlockAndPrivateData
	BlockNum        uint64
	Err             error
}

// getEventFromDeliverService send a request to DeliverWithPrivateData grpc service
// and receive the response
func getEventFromDeliverService(network *nwo.Network, peer *nwo.Peer, channelID string, signingIdentity msp.SigningIdentity, blockNum uint64) *deliverEvent {
	ctx, cancelFunc1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc1()
	eventCh, conn := registerForDeliverEvent(ctx, network, peer, channelID, signingIdentity, blockNum)
	defer conn.Close()
	event := &deliverEvent{}
	Eventually(eventCh, 30*time.Second).Should(Receive(event))
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
