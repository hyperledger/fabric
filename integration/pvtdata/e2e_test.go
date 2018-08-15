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

	"github.com/hyperledger/fabric/integration/pvtdata/helpers"
	"github.com/hyperledger/fabric/integration/pvtdata/runner"
	"github.com/hyperledger/fabric/integration/pvtdata/world"
	"github.com/hyperledger/fabric/protos/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"

	"github.com/fsouza/go-dockerclient"
)

var _ = Describe("PrivateData-EndToEnd", func() {
	var (
		testDir                 string
		w                       *world.World
		d                       world.Deployment
		expectedDiscoveredPeers []helpers.DiscoveredPeer
	)

	// at the beginning of each test under this block, we have 2 collections defined:
	// 1. collectionMarbles - Org1 and Org2 are have access to this collection
	// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
	// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles[1]
	// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails[2]
	PDescribe("collection config is modified", func() {
		BeforeEach(func() {
			var err error
			testDir, err = ioutil.TempDir("", "e2e-pvtdata")
			Expect(err).NotTo(HaveOccurred())
			w = world.GenerateBasicConfig("solo", 1, 3, testDir, components)

			d = world.Deployment{
				Channel: "testchannel",
				Chaincode: world.Chaincode{
					Name:                  "marblesp",
					Version:               "1.0",
					Path:                  "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					ExecPath:              os.Getenv("PATH"),
					CollectionsConfigPath: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
				},
				InitArgs: `{"Args":["init"]}`,
				Policy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				Orderer:  "127.0.0.1:7050",
			}

			w.SetupWorld(d)

			By("setting up all anchor peers")
			setAnchorsPeerForAllOrgs(1, 3, d, w.Rootpath)

			By("verify membership was built using discovery service")
			expectedDiscoveredPeers = []helpers.DiscoveredPeer{
				{MSPID: "Org1MSP", Endpoint: "0.0.0.0:7051"},
				{MSPID: "Org2MSP", Endpoint: "0.0.0.0:8051"},
				{MSPID: "Org3MSP", Endpoint: "0.0.0.0:9051"},
			}
			verifyMembership(w, d, expectedDiscoveredPeers)

			By("invoking initMarble function of the chaincode")
			adminPeer := getPeer(0, 1, w.Rootpath)
			adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, d.Orderer)
			err = helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))

			By("check that the access of different peers is as defined in collection config")
			peerList := []*runner.Peer{getPeer(0, 1, w.Rootpath), getPeer(0, 2, w.Rootpath)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, peerList, `{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)
			peerList = []*runner.Peer{getPeer(0, 2, w.Rootpath), getPeer(0, 3, w.Rootpath)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, peerList, `{"docType":"marblePrivateDetails","name":"marble1","price":99}`)

			By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
			adminPeer = getPeer(0, 1, w.Rootpath)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, adminPeer, "private data matching public hash version is not available")

			By("querying collectionMarbles by peer0.org3, shouldn't have access")
			adminPeer = getPeer(0, 3, w.Rootpath)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, adminPeer, "Failed to get state for marble1")

			By("installing chaincode version 2.0 on all peers")
			installChaincodeOnAllPeers(1, 3, d.Chaincode.Name, "2.0", "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd", testDir)
		})

		AfterEach(func() {
			if w != nil {
				w.Close(d)
			}

			// Stop the running chaincode containers
			filters := map[string][]string{}
			filters["name"] = []string{
				fmt.Sprintf("%s-1.0", d.Chaincode.Name),
				fmt.Sprintf("%s-2.0", d.Chaincode.Name),
				fmt.Sprintf("%s-3.0", d.Chaincode.Name),
			}
			allContainers, _ := w.DockerClient.ListContainers(docker.ListContainersOptions{
				Filters: filters,
			})
			for _, container := range allContainers {
				w.DockerClient.RemoveContainer(docker.RemoveContainerOptions{
					ID:    container.ID,
					Force: true,
				})
			}

			os.RemoveAll(testDir)
		})

		It("verifies access to private data after an org is added to collection config", func() {
			// after the upgrade the collections will be updated as follows:
			// 1. collectionMarbles - Org1, Org2 and Org3 have access to this collection
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
			By("upgrading chaincode in order to update collections config")
			adminPeer := getPeer(0, 1, testDir)
			adminPeer.UpgradeChaincode(d.Chaincode.Name, "2.0", d.Orderer, d.Channel, `{"Args":["init"]}`, `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`, filepath.Join("testdata", "collection_configs", "collections_config2.json"))

			By("invoking initMarble function of the chaincode")
			adminPeer = getPeer(0, 2, testDir)
			adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble2","yellow","53","jerry","22"]}`, d.Orderer)
			err := helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))

			By("check that the access of different peers is as defined in collection config")
			peerList := []*runner.Peer{getPeer(0, 1, testDir), getPeer(0, 2, testDir), getPeer(0, 3, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble2"]}`, peerList, `{"docType":"marble","name":"marble2","color":"yellow","size":53,"owner":"jerry"}`)
			peerList = []*runner.Peer{getPeer(0, 2, testDir), getPeer(0, 3, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble2"]}`, peerList, `{"docType":"marblePrivateDetails","name":"marble2","price":22}`)

			By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
			adminPeer = getPeer(0, 1, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble2"]}`, adminPeer, "private data matching public hash version is not available")

			By("querying collectionMarbles by peer0.org3, make sure it doesn't have access to marble1 that was created before adding peer0.org3 to the config")
			adminPeer = getPeer(0, 3, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, adminPeer, "Failed to get state for marble1")
		})

		It("verifies access to private data after an org is removed from collection config and then added back", func() {
			// after the upgrade the collections will be updated as follows:
			// 1. collectionMarbles - only Org1 has access to this collection
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
			By("upgrading chaincode in order to update collections config")
			adminPeer := getPeer(0, 1, testDir)
			adminPeer.UpgradeChaincode(d.Chaincode.Name, "2.0", d.Orderer, d.Channel, `{"Args":["init"]}`, `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`, filepath.Join("testdata", "collection_configs", "collections_config3.json"))

			By("invoking initMarble function of the chaincode")
			adminPeer = getPeer(0, 2, testDir)
			adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble2","yellow","53","jerry","22"]}`, d.Orderer)
			err := helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))

			By("check that the access of different peers is as defined in collection config")
			peerList := []*runner.Peer{getPeer(0, 1, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble2"]}`, peerList, `{"docType":"marble","name":"marble2","color":"yellow","size":53,"owner":"jerry"}`)
			peerList = []*runner.Peer{getPeer(0, 2, testDir), getPeer(0, 3, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble2"]}`, peerList, `{"docType":"marblePrivateDetails","name":"marble2","price":22}`)

			By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
			adminPeer = getPeer(0, 1, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble2"]}`, adminPeer, "private data matching public hash version is not available")

			By("querying collectionMarbles by peer0.org2, shouldn't have access")
			adminPeer = getPeer(0, 2, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble2"]}`, adminPeer, "Failed to get state for marble2")

			By("querying collectionMarbles by peer0.org3, shouldn't have access")
			adminPeer = getPeer(0, 3, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble2"]}`, adminPeer, "Failed to get state for marble2")

			By("installing chaincode version 3.0 on all peers")
			installChaincodeOnAllPeers(1, 3, d.Chaincode.Name, "3.0",
				"github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd", testDir)

			// after the upgrade the collections will be updated as follows:
			// 1. collectionMarbles - Org1 and Org2 have access to this collection
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
			adminPeer.UpgradeChaincode(d.Chaincode.Name, "3.0", d.Orderer, d.Channel, `{"Args":["init"]}`, `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`, filepath.Join("testdata", "collection_configs", "collections_config1.json"))

			By("invoking initMarble function of the chaincode")
			adminPeer = getPeer(0, 2, testDir)
			adminRunner = adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble3","green","17","mark","68"]}`, d.Orderer)
			err = helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))

			By("check that the access of different peers is as defined in collection config")
			peerList = []*runner.Peer{getPeer(0, 1, testDir), getPeer(0, 2, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble3"]}`, peerList, `{"docType":"marble","name":"marble3","color":"green","size":17,"owner":"mark"}`)
			peerList = []*runner.Peer{getPeer(0, 2, testDir), getPeer(0, 3, testDir)}
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble3"]}`, peerList, `{"docType":"marblePrivateDetails","name":"marble3","price":68}`)

			By("querying collectionMarblePrivateDetails by peer0.org1, shouldn't have access")
			adminPeer = getPeer(0, 1, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble3"]}`, adminPeer, "private data matching public hash version is not available")

			By("querying collectionMarbles by peer0.org3, shouldn't have access")
			adminPeer = getPeer(0, 3, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble3"]}`, adminPeer, "Failed to get state for marble3")

			By("querying collectionMarbles by peer0.org2, make sure it still has access to marble1 that was created before peer0.org2 was removed from the config")
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, []*runner.Peer{getPeer(0, 2, testDir)}, `{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)

			By("querying collectionMarbles by peer0.org2, make sure it still doesn't have access to marble2 that was created while peer0.org2 wasn't in the config")
			adminPeer = getPeer(0, 2, testDir)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble2"]}`, adminPeer, "Failed to get state for marble2")
		})
	})

	PDescribe("collection config BlockToLive is respected", func() {
		BeforeEach(func() {
			var err error
			testDir, err = ioutil.TempDir("", "e2e-pvtdata")
			Expect(err).NotTo(HaveOccurred())
			w = world.GenerateBasicConfig("solo", 1, 3, testDir, components)

			// instantiate the chaincode with two collections:
			// collectionMarbles - with blockToLive=1000000
			// collectionMarblePrivateDetails - with blockToLive=3
			// will use "readMarble" to read from the first one and "readMarblePrivateDetails" from the second one.
			// need to make sure data is purged only on the second collection
			d = world.Deployment{
				Channel: "testchannel",
				Chaincode: world.Chaincode{
					Name:                  "marblesp",
					Version:               "1.0",
					Path:                  "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					ExecPath:              os.Getenv("PATH"),
					CollectionsConfigPath: filepath.Join("testdata", "collection_configs", "short_btl_config.json"),
				},
				InitArgs: `{"Args":["init"]}`,
				Policy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				Orderer:  "127.0.0.1:7050",
			}

			w.SetupWorld(d)

			By("setting up all anchor peers")
			setAnchorsPeerForAllOrgs(1, 3, d, w.Rootpath)

			By("verify membership was built using discovery service")
			expectedDiscoveredPeers = []helpers.DiscoveredPeer{
				{MSPID: "Org1MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:7051", Identity: "", Chaincodes: []string{}},
				{MSPID: "Org2MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:8051", Identity: "", Chaincodes: []string{}},
				{MSPID: "Org3MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:9051", Identity: "", Chaincodes: []string{}},
			}
			verifyMembership(w, d, expectedDiscoveredPeers)

			By("invoking initMarble function of the chaincode")
			adminPeer := getPeer(0, 1, w.Rootpath)
			adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, d.Orderer)
			err = helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))
		})

		AfterEach(func() {
			if w != nil {
				w.Close(d)
			}
			os.RemoveAll(testDir)
		})

		It("verifies private data is purged after BTL has passed and new peer doesn't pull private data that was purged", func() {
			adminPeer := getPeer(0, 2, testDir)
			By("create 4 blocks to reach BTL threshold and have marble1 private data purged")
			initialLedgerHeight := getLedgerHeight(0, 2, d.Channel, testDir)
			ledgerHeight := initialLedgerHeight
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, []*runner.Peer{adminPeer}, `{"docType":"marblePrivateDetails","name":"marble1","price":99}`)
			for i := 2; ledgerHeight < initialLedgerHeight+4; i++ {
				By(fmt.Sprintf("created %d blocks, still haven't reached BTL, should have access to private data", ledgerHeight-initialLedgerHeight))
				adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, fmt.Sprintf(`{"Args":["initMarble","marble%d","blue%d","3%d","tom","9%d"]}`, i, i, i, i), d.Orderer)
				err := helpers.Execute(adminRunner)
				Expect(err).NotTo(HaveOccurred())
				Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))
				ledgerHeight = getLedgerHeight(0, 2, d.Channel, testDir)
			}
			By("querying collectionMarbles by peer0.org2, marble1 should still be available")
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, []*runner.Peer{adminPeer}, `{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)
			By("querying collectionMarblePrivateDetails by peer0.org2, marble1 should have been purged")
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, adminPeer, "Marble private details does not exist: marble1")

			By("spawn a new peer peer1.org2")
			spawnNewPeer(w, 1, 2)
			adminPeer = getPeer(1, 2, testDir)
			EventuallyWithOffset(1, func() (*gbytes.Buffer, error) {
				adminRunner := adminPeer.FetchChannel(d.Channel, filepath.Join(w.Rootpath, "peer1.org2.example.com", fmt.Sprintf("%s_block.pb", d.Channel)), "0", d.Orderer)
				err := helpers.Execute(adminRunner)
				return adminRunner.Err(), err
			}).Should(gbytes.Say("Received block: 0"))

			By("join peer1.org2 to the channel")
			EventuallyWithOffset(1, func() (*gbytes.Buffer, error) {
				adminRunner := adminPeer.JoinChannel(filepath.Join(w.Rootpath, "peer1.org2.example.com", fmt.Sprintf("%s_block.pb", d.Channel)))
				err := helpers.Execute(adminRunner)
				return adminRunner.Err(), err
			}).Should(gbytes.Say("Successfully submitted proposal to join channel"))

			By("install the chaincode on peer1.org2 in order to query it")
			adminPeer.InstallChaincode("marblesp", "1.0", "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd")

			By("fetch latest blocks to peer1.org2")
			EventuallyWithOffset(1, func() (*gbytes.Buffer, error) {
				adminRunner := adminPeer.FetchChannel(d.Channel, filepath.Join(w.Rootpath, "peer1.org2.example.com", fmt.Sprintf("%s_block.pb", d.Channel)), "newest", d.Orderer)
				err := helpers.Execute(adminRunner)
				return adminRunner.Err(), err
			}).Should(gbytes.Say("Received block: 9"))

			By("wait until ledger is updated with all blocks")
			EventuallyWithOffset(1, func() int {
				return getLedgerHeight(1, 2, d.Channel, testDir)
			}, time.Minute).Should(Equal(10))

			By("query peer1.org2, verify marble1 exist in collectionMarbles and private data doesn't exist")
			verifyAccess(d.Chaincode.Name, d.Channel, `{"Args":["readMarble","marble1"]}`, []*runner.Peer{adminPeer}, `{"docType":"marble","name":"marble1","color":"blue","size":35,"owner":"tom"}`)
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, adminPeer, "Marble private details does not exist: marble1")
		})
	})

	PDescribe("network partition with respect of private data", func() {
		BeforeEach(func() {
			var err error
			testDir, err = ioutil.TempDir("", "e2e-pvtdata")
			Expect(err).NotTo(HaveOccurred())
			w = world.GenerateBasicConfig("solo", 1, 3, testDir, components)

			// instantiate the chaincode with two collections such that only Org1 and Org2 have access to the collections
			d = world.Deployment{
				Channel: "testchannel",
				Chaincode: world.Chaincode{
					Name:                  "marblesp",
					Version:               "1.0",
					Path:                  "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
					ExecPath:              os.Getenv("PATH"),
					CollectionsConfigPath: filepath.Join("testdata", "collection_configs", "collections_config4.json"),
				},
				InitArgs: `{"Args":["init"]}`,
				Policy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
				Orderer:  "127.0.0.1:7050",
			}

			w.SetupWorld(d)

			stopPeer(w, 0, 3)

			By("setting up all anchor peers")
			setAnchorsPeerForAllOrgs(1, 2, d, w.Rootpath)

			By("verify membership was built using discovery service")

			expectedDiscoveredPeers = []helpers.DiscoveredPeer{
				{MSPID: "Org1MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:7051", Identity: "", Chaincodes: []string{}},
				{MSPID: "Org2MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:8051", Identity: "", Chaincodes: []string{}},
			}
			verifyMembership(w, d, expectedDiscoveredPeers)

			By("invoking initMarble function of the chaincode")
			adminPeer := getPeer(0, 1, w.Rootpath)
			adminRunner := adminPeer.InvokeChaincode(d.Chaincode.Name, d.Channel, `{"Args":["initMarble","marble1","blue","35","tom","99"]}`, d.Orderer)
			err = helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Chaincode invoke successful."))

			By("installing chaincode version 2.0 on all p0.org1 and p0.0rg2")
			installChaincodeOnAllPeers(1, 2, d.Chaincode.Name, "2.0", "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd", testDir)
		})

		AfterEach(func() {
			if w != nil {
				w.Close(d)
			}

			// Stop the running chaincode containers
			filters := map[string][]string{}
			filters["name"] = []string{
				fmt.Sprintf("%s-1.0", d.Chaincode.Name),
				fmt.Sprintf("%s-2.0", d.Chaincode.Name),
			}
			allContainers, _ := w.DockerClient.ListContainers(docker.ListContainersOptions{
				Filters: filters,
			})
			for _, container := range allContainers {
				w.DockerClient.RemoveContainer(docker.RemoveContainerOptions{
					ID:    container.ID,
					Force: true,
				})
			}

			os.RemoveAll(testDir)
		})

		It("verifies private data not distributed when there is network partition", func() {
			// after the upgrade:
			// 1. collectionMarbles - Org1 and Org2 are have access to this collection, using readMarble to read from it.
			// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection - using readMarblePrivateDetails to read from it.
			By("upgrading chaincode in order to update collections config, now org2 and org3 have access to private collection")
			adminPeer := getPeer(0, 2, testDir)
			adminPeer.UpgradeChaincode(d.Chaincode.Name, "2.0", d.Orderer, d.Channel, `{"Args":["init"]}`, `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`, filepath.Join("testdata", "collection_configs", "collections_config1.json"))

			By("stop p0.org2 process")
			stopPeer(w, 0, 2)

			By("verify membership using discovery service")
			expectedDiscoveredPeers = []helpers.DiscoveredPeer{
				{MSPID: "Org1MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:7051", Identity: "", Chaincodes: []string{}},
			}
			verifyMembership(w, d, expectedDiscoveredPeers)

			By("start p0.org3 process")
			spawnNewPeer(w, 0, 3)

			By("set p0.org3 as anchor peer of org3")
			adminPeer = getPeer(0, 3, testDir)
			adminRunner := adminPeer.UpdateChannel(filepath.Join(testDir, "Org3_anchors_update_tx.pb"), d.Channel, d.Orderer)
			err := helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Successfully submitted channel update"))

			By("verify membership was built using discovery service")
			expectedDiscoveredPeers = []helpers.DiscoveredPeer{
				{MSPID: "Org1MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:7051", Identity: "", Chaincodes: []string{}},
				{MSPID: "Org3MSP", LedgerHeight: 0, Endpoint: "0.0.0.0:9051", Identity: "", Chaincodes: []string{}},
			}
			verifyMembership(w, d, expectedDiscoveredPeers)

			By("install the chaincode on p0.org3 in order to query it")
			adminPeer.InstallChaincode("marblesp", "2.0", "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd")

			By("fetch latest blocks to peer0.org3 to make sure it reached the same height as the other alive peer")
			EventuallyWithOffset(1, func() (*gbytes.Buffer, error) {
				adminRunner := adminPeer.FetchChannel(d.Channel, filepath.Join(w.Rootpath, "peer0.org3.example.com", fmt.Sprintf("%s_block.pb", d.Channel)), "newest", d.Orderer)
				err := helpers.Execute(adminRunner)
				return adminRunner.Err(), err
			}).Should(gbytes.Say("Received block: 6"))

			By("wait until ledger is updated with all blocks")
			EventuallyWithOffset(1, func() int {
				return getLedgerHeight(0, 3, d.Channel, testDir)
			}, time.Minute).Should(Equal(7))

			By("verify p0.org3 didn't get private data")
			verifyAccessFailed(d.Chaincode.Name, d.Channel, `{"Args":["readMarblePrivateDetails","marble1"]}`, adminPeer, "Failed to get private details for marble1")
		})
	})
})

func getPeer(peer int, org int, testDir string) *runner.Peer {
	adminPeer := components.Peer()
	adminPeer.LogLevel = "debug"
	adminPeer.ConfigDir = filepath.Join(testDir, fmt.Sprintf("peer%d.org%d.example.com", peer, org))
	adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users", fmt.Sprintf("Admin@org%d.example.com", org), "msp")
	return adminPeer
}

func getLedgerHeight(peer int, org int, channel string, testDir string) int {
	adminPeer := getPeer(peer, org, testDir)
	adminRunner := adminPeer.GetChannelInfo(channel)
	err := helpers.Execute(adminRunner)
	Expect(err).NotTo(HaveOccurred())

	channelInfoStr := strings.TrimPrefix(string(adminRunner.Buffer().Contents()[:]), "Blockchain info:")
	var channelInfo = common.BlockchainInfo{}
	json.Unmarshal([]byte(channelInfoStr), &channelInfo)
	return int(channelInfo.Height)
}

func setAnchorsPeerForAllOrgs(numPeers int, numOrgs int, d world.Deployment, testDir string) {
	for orgIndex := 1; orgIndex <= numOrgs; orgIndex++ {
		for peerIndex := 0; peerIndex < numPeers; peerIndex++ {
			adminPeer := getPeer(peerIndex, orgIndex, testDir)
			adminRunner := adminPeer.UpdateChannel(filepath.Join(testDir, fmt.Sprintf("Org%d_anchors_update_tx.pb", orgIndex)), d.Channel, d.Orderer)
			err := helpers.Execute(adminRunner)
			Expect(err).NotTo(HaveOccurred())
			Expect(adminRunner.Err()).To(gbytes.Say("Successfully submitted channel update"))
		}
	}
}

func installChaincodeOnAllPeers(numPeers int, numOrgs int, ccname string, ccversion string, ccpath string, testDir string) {
	for orgIndex := 1; orgIndex <= numOrgs; orgIndex++ {
		for peerIndex := 0; peerIndex < numPeers; peerIndex++ {
			adminPeer := getPeer(peerIndex, orgIndex, testDir)
			adminPeer.InstallChaincode(ccname, ccversion, ccpath)
		}
	}
}

func verifyAccess(ccname string, channel string, args string, peers []*runner.Peer, expected string) {
	for _, peer := range peers {
		EventuallyWithOffset(1, func() (*gbytes.Buffer, error) {
			adminRunner := peer.QueryChaincode(ccname, channel, args)
			err := helpers.Execute(adminRunner)
			return adminRunner.Buffer(), err
		}, time.Minute).Should(gbytes.Say(expected)) // this calls will also verify error didn't occur
	}
}

func verifyAccessFailed(ccname string, channel string, args string, peer *runner.Peer, expectedFailureMessage string) {
	EventuallyWithOffset(1, func() *gbytes.Buffer {
		adminRunner := peer.QueryChaincode(ccname, channel, args)
		err := helpers.Execute(adminRunner)
		Expect(err).To(HaveOccurred())
		return adminRunner.Err()
	}, time.Minute).Should(gbytes.Say(expectedFailureMessage))
}

func spawnNewPeer(w *world.World, peerIndex int, orgIndex int) {
	peerName := fmt.Sprintf("peer%d.org%d.example.com", peerIndex, orgIndex)
	if _, err := os.Stat(filepath.Join(w.Rootpath, peerName)); os.IsNotExist(err) {
		err := os.Mkdir(filepath.Join(w.Rootpath, peerName), 0755)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	helpers.CopyFile(
		filepath.Join("testdata", fmt.Sprintf("%s-core.yaml", peerName)),
		filepath.Join(w.Rootpath, peerName, "core.yaml"),
	)

	peer := w.Components.Peer()
	peer.ConfigDir = filepath.Join(w.Rootpath, fmt.Sprintf("peer%d.org%d.example.com", peerIndex, orgIndex))
	peer.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", orgIndex),
		"users", fmt.Sprintf("Admin@org%d.example.com", orgIndex), "msp")
	peerProcess := ifrit.Invoke(peer.NodeStart(peerIndex))
	EventuallyWithOffset(2, peerProcess.Ready()).Should(BeClosed())
	ConsistentlyWithOffset(2, peerProcess.Wait()).ShouldNot(Receive())
	w.LocalProcess = append(w.LocalProcess, peerProcess)
	w.NameToProcessMapping[peerName] = peerProcess
}

func stopPeer(w *world.World, peer int, org int) {
	peerName := fmt.Sprintf("peer%d.org%d.example.com", peer, org)
	w.NameToProcessMapping[peerName].Signal(syscall.SIGTERM)
	w.NameToProcessMapping[peerName].Signal(syscall.SIGKILL)
	delete(w.NameToProcessMapping, peerName)
}

func verifyMembership(w *world.World, d world.Deployment, expectedDiscoveredPeers []helpers.DiscoveredPeer) {
	sd := getDiscoveryService(1, filepath.Join(w.Rootpath, "config_org1.yaml"), w.Rootpath)
	EventuallyWithOffset(1, func() bool {
		_, result := runner.VerifyAllPeersDiscovered(sd, expectedDiscoveredPeers, d.Channel, "127.0.0.1:7051")
		return result
	}, time.Minute).Should(BeTrue())
}

func getDiscoveryService(org int, configFilePath string, testDir string) *runner.DiscoveryService {
	userCert := filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users",
		fmt.Sprintf("User1@org%d.example.com", org), "msp", "signcerts", fmt.Sprintf("User1@org%d.example.com-cert.pem", org))

	userKeyDir := filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users",
		fmt.Sprintf("User1@org%d.example.com", org), "msp", "keystore")

	return runner.SetupDiscoveryService(components.DiscoveryService(), org, configFilePath, userCert, userKeyDir)
}
