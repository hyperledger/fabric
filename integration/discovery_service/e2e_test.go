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

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/protos/common"
	. "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	configBlockFile        = "config_block.pb"
	updatedConfigBlockFile = "updated_config.pb"
)

var _ = Describe("DiscoveryService EndToEnd", func() {

	var (
		testDir                      string
		w                            *world.World
		d                            world.Deployment
		expectedDiscoverdPeers       []helpers.DiscoveredPeer
		expectedConfig               ConfigResult
		expectedChaincodeEndrorsers  helpers.EndorsementDescriptor
		expectedChaincodeEndrorsers2 helpers.EndorsementDescriptor
		expectedChaincodeEndrorsers3 helpers.EndorsementDescriptor
	)

	BeforeEach(func() {
		expectedDiscoverdPeers = []helpers.DiscoveredPeer{
			{"Org1MSP", 0, "0.0.0.0:7051", "", []string{}},
			{"Org1MSP", 0, "0.0.0.0:14051", "", []string{}},
			{"Org2MSP", 0, "0.0.0.0:8051", "", []string{}},
			{"Org2MSP", 0, "0.0.0.0:15051", "", []string{}},
			{"Org3MSP", 0, "0.0.0.0:9051", "", []string{}},
			{"Org3MSP", 0, "0.0.0.0:16051", "", []string{}},
		}
		ordererEndpoint := Endpoint{"0.0.0.0", 7050}
		expectedConfig = ConfigResult{
			Msps: map[string]*msp.FabricMSPConfig{
				"OrdererMSP": {"OrdererMSP", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
				"Org1MSP":    {"Org1MSP", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
				"Org2MSP":    {"Org2MSP", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
				"Org3MSP":    {"Org3MSP", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
			},
			Orderers: map[string]*Endpoints{
				"OrdererMSP": {[]*Endpoint{&ordererEndpoint}},
			},
		}
		expectedChaincodeEndrorsers = helpers.EndorsementDescriptor{
			Chaincode: "mycc",
			EndorsersByGroups: map[string][]helpers.DiscoveredPeer{
				"A": {{"Org1MSP", 0, "0.0.0.0:7051", "", []string{}}},
				"B": {{"Org2MSP", 0, "0.0.0.0:8051", "", []string{}}},
				"C": {{"Org3MSP", 0, "0.0.0.0:9051", "", []string{}}},
			},
			Layouts: []*Layout{
				{QuantitiesByGroup: map[string]uint32{
					"A": 1,
					"B": 1,
				}},
				{QuantitiesByGroup: map[string]uint32{
					"A": 1,
					"C": 1,
				}},
				{QuantitiesByGroup: map[string]uint32{
					"B": 1,
					"C": 1,
				}},
			},
		}

		expectedChaincodeEndrorsers2 = helpers.EndorsementDescriptor{
			Chaincode: "mycc2",
			EndorsersByGroups: map[string][]helpers.DiscoveredPeer{
				"A": {
					{"Org1MSP", 0, "0.0.0.0:7051", "", []string{}},
					{"Org1MSP", 0, "0.0.0.0:14051", "", []string{}},
				},
				"B": {
					{"Org2MSP", 0, "0.0.0.0:8051", "", []string{}},
					{"Org2MSP", 0, "0.0.0.0:15051", "", []string{}},
				},
				"C": {
					{"Org3MSP", 0, "0.0.0.0:9051", "", []string{}},
					{"Org3MSP", 0, "0.0.0.0:16051", "", []string{}},
				},
			},
			Layouts: []*Layout{
				{QuantitiesByGroup: map[string]uint32{
					"A": 1,
					"B": 1,
					"C": 1,
				}},
			},
		}

		expectedChaincodeEndrorsers3 = helpers.EndorsementDescriptor{
			Chaincode: "mycc",
			EndorsersByGroups: map[string][]helpers.DiscoveredPeer{
				"A": {{"Org1MSP", 0, "0.0.0.0:7051", "", []string{}}},
				"B": {{"Org2MSP", 0, "0.0.0.0:8051", "", []string{}}},
			},
			Layouts: []*Layout{
				{QuantitiesByGroup: map[string]uint32{
					"A": 1,
					"B": 1,
				}},
			},
		}

		var err error
		testDir, err = ioutil.TempDir("", "e2e-ServiceDiscovery")
		Expect(err).NotTo(HaveOccurred())
		w = world.GenerateBasicConfig("solo", 2, 3, testDir, components)
		d = world.Deployment{
			Channel: "testchannel",
			Chaincode: world.Chaincode{
				Name:     "simple",
				Version:  "1.0",
				Path:     "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Policy:   `OR (AND ('Org1MSP.member','Org2MSP.member'), AND ('Org1MSP.member','Org3MSP.member'), AND ('Org2MSP.member','Org3MSP.member'))`,
			Orderer:  "127.0.0.1:7050",
		}
		w.SetupWorld(d)

		By("setting up all anchor peers")
		setAnchorsPeerForAllOrgs(1, 3, d, w.Rootpath)
	})

	AfterEach(func() {
		if w != nil {
			w.Close(d)
		}

		// Stop the running chaincode containers
		filters := map[string][]string{}
		filters["name"] = []string{
			"mycc-1.0",
			"mycc-2.0",
			"mycc2-1.0",
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

	It("test discovery service capabilities", func() {
		By("discover peers")
		sd := getDiscoveryService(3, filepath.Join(testDir, "config_org3.yaml"), testDir)
		EventuallyWithOffset(1, func() bool {
			_, result := runner.VerifyAllPeersDiscovered(sd, expectedDiscoverdPeers, d.Channel, "127.0.0.1:9051")
			return result
		}, time.Minute).Should(BeTrue())
		By("discover config")
		EventuallyWithOffset(1, runner.VerifyConfigDiscovered(sd, expectedConfig, d.Channel, "127.0.0.1:9051"), time.Minute).Should(BeTrue())
		By("discover endorsers, shouldn't get a valid result since cc was not installed yet")
		sdRunner := sd.DiscoverEndorsers(d.Channel, "127.0.0.1:7051", "mycc", "")
		err := helpers.Execute(sdRunner)
		Expect(err).To(HaveOccurred())
		Expect(sdRunner.Err()).Should(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("install and instantiate chaincode on p0.org1")
		adminPeer := getPeer(0, 1, testDir)
		adminPeer.InstallChaincode("mycc", "1.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		adminPeer.InstantiateChaincode("mycc", "1.0", d.Orderer, d.Channel, `{"Args":["init","a","100","b","200"]}`, `OR (AND ('Org1MSP.member','Org2MSP.member'), AND ('Org1MSP.member','Org3MSP.member'), AND ('Org2MSP.member','Org3MSP.member'))`, "")

		By("discover peers")
		EventuallyWithOffset(1, func() bool {
			discoveredPeers, result := runner.VerifyAllPeersDiscovered(sd, expectedDiscoverdPeers, d.Channel, "127.0.0.1:9051")
			discoveredPeer := getDiscoveredPeer(discoveredPeers, "0.0.0.0:7051", "Org1MSP")
			return result && helpers.Contains(discoveredPeer.Chaincodes, "mycc")
		}, time.Minute).Should(BeTrue())
		By("discover config")
		EventuallyWithOffset(1, runner.VerifyConfigDiscovered(sd, expectedConfig, d.Channel, "127.0.0.1:9051"), time.Minute).Should(BeTrue())
		By("discover endorsers, shouldn't get a valid result since cc was not installed on sufficient number of orgs yet")
		sdRunner = sd.DiscoverEndorsers(d.Channel, "127.0.0.1:7051", "mycc", "")
		err = helpers.Execute(sdRunner)
		Expect(err).To(HaveOccurred())
		Expect(sdRunner.Err()).Should(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("install chaincode on p0.org2")
		adminPeer = getPeer(0, 2, testDir)
		adminPeer.InstallChaincode("mycc", "1.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")

		By("install chaincode on p0.org3")
		adminPeer = getPeer(0, 3, testDir)
		adminPeer.InstallChaincode("mycc", "1.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")

		By("discover peers")
		EventuallyWithOffset(1, func() bool {
			discoveredPeers, result := runner.VerifyAllPeersDiscovered(sd, expectedDiscoverdPeers, d.Channel, "127.0.0.1:9051")
			discoveredPeerOrg1 := getDiscoveredPeer(discoveredPeers, "0.0.0.0:7051", "Org1MSP")
			discoveredPeerOrg2 := getDiscoveredPeer(discoveredPeers, "0.0.0.0:8051", "Org2MSP")
			discoveredPeerOrg3 := getDiscoveredPeer(discoveredPeers, "0.0.0.0:9051", "Org3MSP")
			return result && helpers.Contains(discoveredPeerOrg1.Chaincodes, "mycc") && helpers.Contains(discoveredPeerOrg2.Chaincodes, "mycc") && helpers.Contains(discoveredPeerOrg3.Chaincodes, "mycc")
		}, time.Minute).Should(BeTrue())
		By("discover config")
		EventuallyWithOffset(1, runner.VerifyConfigDiscovered(sd, expectedConfig, d.Channel, "127.0.0.1:9051"), time.Minute).Should(BeTrue())
		By("discover endorsers")
		EventuallyWithOffset(1, runner.VerifyEndorsersDiscovered(sd, expectedChaincodeEndrorsers, d.Channel, "127.0.0.1:9051", "mycc", ""), time.Minute).Should(BeTrue())

		By("install a new chaincode on all peers")
		installChaincodeOnAllPeers(2, 3, "mycc2", "1.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd", testDir)
		By("instantiate new chaincode")
		adminPeer.InstantiateChaincode("mycc2", "1.0", d.Orderer, d.Channel, `{"Args":["init","a","100","b","200"]}`, `AND ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`, "")
		By("discover endorsers")
		EventuallyWithOffset(1, func() bool {
			return runner.VerifyEndorsersDiscovered(sd, expectedChaincodeEndrorsers2, d.Channel, "127.0.0.1:9051", "mycc2", "")
		}, time.Minute).Should(BeTrue())

		By("upgrading mycc chaincode and add collections config")
		adminPeer = getPeer(0, 2, testDir)
		adminPeer.InstallChaincode("mycc", "2.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		adminPeer = getPeer(0, 3, testDir)
		adminPeer.InstallChaincode("mycc", "2.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		adminPeer = getPeer(0, 1, testDir)
		adminPeer.InstallChaincode("mycc", "2.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		adminPeer.UpgradeChaincode("mycc", "2.0", d.Orderer, d.Channel, `{"Args":["init","a","100","b","200"]}`, `OR (AND ('Org1MSP.member','Org2MSP.member'), AND ('Org1MSP.member','Org3MSP.member'), AND ('Org2MSP.member','Org3MSP.member'))`, filepath.Join("testdata", "collections_config1.json"))
		By("discover endorsers")
		EventuallyWithOffset(1, func() bool {
			return runner.VerifyEndorsersDiscovered(sd, expectedChaincodeEndrorsers3, d.Channel, "127.0.0.1:7051", "mycc", "collectionMarbles")
		}, time.Minute).Should(BeTrue())
		By("remove org3 members (except admin) from channel writers")
		sendChannelConfigUpdate(0, 3, d.Channel, d.Orderer, testDir)
		By("try to discover peers using org3, should get access denied")
		EventuallyWithOffset(1, func() *gbytes.Buffer {
			sdRunner = sd.DiscoverPeers(d.Channel, "127.0.0.1:9051")
			helpers.Execute(sdRunner)
			return sdRunner.Err()
		}, time.Minute).Should(gbytes.Say("access denied"))
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
			Expect(adminRunner.Err()).Should(gbytes.Say("Successfully submitted channel update"))
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

func getDiscoveredPeer(discoveredPeers []helpers.DiscoveredPeer, endpoint string, mspid string) helpers.DiscoveredPeer {
	for _, discoveredPeer := range discoveredPeers {
		if discoveredPeer.Endpoint == endpoint && discoveredPeer.MSPID == mspid {
			return discoveredPeer
		}
	}
	return helpers.DiscoveredPeer{}
}

func getDiscoveryService(org int, configFilePath string, testDir string) *runner.DiscoveryService {
	userCert := filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users", fmt.Sprintf("User1@org%d.example.com", org), "msp", "signcerts", fmt.Sprintf("User1@org%d.example.com-cert.pem", org))
	userKeyDir := filepath.Join(testDir, "crypto", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users", fmt.Sprintf("User1@org%d.example.com", org), "msp", "keystore")
	return runner.SetupDiscoveryService(components.DiscoveryService(), org, configFilePath, userCert, userKeyDir)
}

func sendChannelConfigUpdate(peer int, org int, channel string, orderer string, testDir string) {
	generateConfigUpdateBlock(peer, org, channel, orderer, testDir)

	By("Signing update")
	adminPeer := getPeer(peer, org, testDir)
	adminRunner := adminPeer.SignConfigTx(filepath.Join(testDir, updatedConfigBlockFile))
	err := helpers.Execute(adminRunner)
	Expect(err).NotTo(HaveOccurred())

	By("Sending update")
	adminRunner = adminPeer.UpdateChannel(filepath.Join(testDir, updatedConfigBlockFile), channel, orderer)
	err = helpers.Execute(adminRunner)
	Expect(err).NotTo(HaveOccurred())
	Expect(adminRunner.Err()).To(gbytes.Say("Successfully submitted channel update"))
}

func generateConfigUpdateBlock(peer int, org int, channel string, orderer string, testDir string) {
	// fetch the config block
	By("Fetching channel config block")
	adminRunner := getPeer(peer, org, testDir)
	configBlockFilePath := filepath.Join(testDir, configBlockFile)
	fRunner := adminRunner.FetchChannel(channel, configBlockFilePath, "config", orderer)
	err := helpers.Execute(fRunner)
	Expect(err).NotTo(HaveOccurred())
	Expect(fRunner.Err()).To(gbytes.Say("Received block: "))

	By("Parsing config block")
	// read the config block file
	fileBytes, err := ioutil.ReadFile(configBlockFilePath)
	Expect(err).NotTo(HaveOccurred())
	Expect(fileBytes).NotTo(BeNil())

	// unmarshal the config block bytes
	configBlock := &common.Block{}
	err = proto.Unmarshal(fileBytes, configBlock)
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the envelope bytes
	env := &common.Envelope{}
	err = proto.Unmarshal(configBlock.Data.Data[0], env)
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the payload bytes
	payload := &common.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	Expect(err).NotTo(HaveOccurred())

	// unmarshal the config envelope bytes
	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	Expect(err).NotTo(HaveOccurred())

	By("Cloning config block")
	// clone the config
	config := configEnv.Config
	updatedConfig := proto.Clone(config).(*common.Config)

	updatedConfig.ChannelGroup.Groups["Application"].Groups["Org3"].Policies["Writers"].Policy.Value = utils.MarshalOrPanic(cauthdsl.SignedByMspAdmin("Org3MSP"))

	// compute the config update and package it into an envelope
	configUpdate, err := update.Compute(config, updatedConfig)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel
	configUpdateEnvelope := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configUpdate),
	}

	signedEnv, err := utils.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil,
		configUpdateEnvelope,
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnv).NotTo(BeNil())

	// write the config update envelope to the file system
	signedEnvelopeBytes := utils.MarshalOrPanic(signedEnv)
	err = ioutil.WriteFile(filepath.Join(testDir, updatedConfigBlockFile), signedEnvelopeBytes, 0660)
	Expect(err).NotTo(HaveOccurred())
}
