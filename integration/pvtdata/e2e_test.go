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
	"time"

	"github.com/hyperledger/fabric/integration/pvtdata/helpers"
	"github.com/hyperledger/fabric/integration/pvtdata/runner"
	"github.com/hyperledger/fabric/integration/pvtdata/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

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
	Describe("collection config is modified", func() {
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
})

func getPeer(peer int, org int, testDir string) *runner.Peer {
	adminPeer := components.Peer()
	adminPeer.LogLevel = "debug"
	adminPeer.ConfigDir = filepath.Join(testDir, fmt.Sprintf("peer%d.org%d.example.com", peer, org))
	adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users", fmt.Sprintf("Admin@org%d.example.com", org), "msp")
	return adminPeer
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
