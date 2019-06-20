/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdata

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
			Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
			Lang:              "golang",
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
				`"marble1", "blue", "35", "tom", "99"`,
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
						`"marble2","yellow","53","jerry","22"`,
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
				helper.addMarble(ccName, `"marble1", "blue", "35", "tom", "99"`, eligiblePeer)

				By("adding three blocks")
				for i := 0; i < 3; i++ {
					helper.addMarble(ccName, fmt.Sprintf(`"test-marble-%d", "blue", "35", "tom", "99"`, i), eligiblePeer)
				}

				By("verifying that marble1 still not purged in collection MarblesPD")
				helper.assertPresentInCollectionMPD(ccName, "marble1", eligiblePeer)

				By("adding one more block")
				helper.addMarble(ccName, `"fun-marble-3", "blue", "35", "tom", "99"`, eligiblePeer)

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

	Describe("collection ACL while reading private data", func() {
		assertCollectionACLBehavior := func() {
			It("does not allow private data reads to non-members", func() {
				// collections_config4: collectionMarblePrivateDetails - member_only_read is set to true
				testChaincode.CollectionsConfig = collectionConfig("collections_config4.json")
				helper.deployChaincode(testChaincode)
				helper.addMarble(
					testChaincode.Name,
					`"marble1", "blue", "35", "tom", "99"`,
					network.Peer("org2", "peer0"),
				)

				By("querying collectionMarblePrivateDetails on org1-peer0 by org1-user1, shouldn't have read access")
				helper.assertNoReadAccessToCollectionMPD(testChaincode.Name, "marble1", network.Peer("org1", "peer0"))
			})
		}

		Context("chaincode in legacy lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: legacyChaincode,
					isLegacy:  true,
				}
			})
			assertCollectionACLBehavior()
		})

		Context("chaincode in new lifecycle", func() {
			BeforeEach(func() {
				testChaincode = chaincode{
					Chaincode: newLifecycleChaincode,
					isLegacy:  false,
				}
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, allPeers...)
			})
			assertCollectionACLBehavior()
		})
	})

	Describe("Org removal from collection", func() {
		assertOrgRemovalBehavior := func() {
			It("causes removed org not to get new data", func() {
				testChaincode.CollectionsConfig = collectionConfig("collections_config2.json")
				helper.deployChaincode(testChaincode)
				helper.addMarble(testChaincode.Name, `"marble1", "blue", "35", "tom", "99"`, network.Peer("org2", "peer0"))
				helper.assertPvtdataPresencePerCollectionConfig2(testChaincode.Name, "marble1")

				By("upgrading chaincode to remove org3 from collectionMarbles")
				testChaincode.CollectionsConfig = collectionConfig("collections_config1.json")
				testChaincode.Version = "1.1"
				if !testChaincode.isLegacy {
					testChaincode.Sequence = "2"
				}
				helper.upgradeChaincode(testChaincode)
				helper.addMarble(testChaincode.Name, `"marble2", "yellow", "53", "jerry", "22"`, network.Peer("org2", "peer0"))
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
	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["initMarble",%s]}`, marbleDetails),
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
