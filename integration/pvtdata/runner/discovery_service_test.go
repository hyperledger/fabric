/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/pvtdata/helpers"
	"github.com/hyperledger/fabric/integration/pvtdata/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	. "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/msp"
)

var _ bool = Describe("DiscoveryService", func() {
	var (
		tempDir   string
		cryptoDir string

		orderer        *runner.Orderer
		ordererProcess ifrit.Process
		peer           *runner.Peer
		peerProcess    ifrit.Process
		ordererRunner  *ginkgomon.Runner

		client *docker.Client

		expectedDiscoverdPeers         []helpers.DiscoveredPeer
		expectedConfig                 ConfigResult
		expectedEndrorsementDescriptor helpers.EndorsementDescriptor
	)

	BeforeEach(func() {
		expectedDiscoverdPeers = []helpers.DiscoveredPeer{
			{MSPID: "Org1MSP", Endpoint: "127.0.0.1:12051"},
		}
		ordererEndpoint := Endpoint{Host: "127.0.0.1", Port: 8050}
		expectedConfig = ConfigResult{
			Msps: map[string]*msp.FabricMSPConfig{
				"OrdererMSP": {Name: "OrdererMSP"},
				"Org1MSP":    {Name: "Org1MSP"},
				"Org2MSP":    {Name: "Org2MSP"},
			},
			Orderers: map[string]*Endpoints{
				"OrdererMSP": {Endpoint: []*Endpoint{&ordererEndpoint}},
			},
		}
		expectedEndrorsementDescriptor = helpers.EndorsementDescriptor{
			Chaincode: "mytest",
			EndorsersByGroups: map[string][]helpers.DiscoveredPeer{
				"A": {{MSPID: "Org1MSP", Endpoint: "127.0.0.1:12051"}},
			},
			Layouts: []*Layout{
				{QuantitiesByGroup: map[string]uint32{
					"A": 1,
				}},
			},
		}

		var err error
		tempDir, err = ioutil.TempDir("", "ServiceDiscovery")
		Expect(err).NotTo(HaveOccurred())

		// Generate crypto info
		cryptoDir = filepath.Join(tempDir, "crypto-config")
		peer = components.Peer()

		helpers.CopyFile(filepath.Join("testdata", "cryptogen-config.yaml"), filepath.Join(tempDir, "cryptogen-config.yaml"))
		cryptogen := components.Cryptogen()
		cryptogen.Config = filepath.Join(tempDir, "cryptogen-config.yaml")
		cryptogen.Output = cryptoDir

		crypto := cryptogen.Generate()
		Expect(execute(crypto)).To(Succeed())

		// Generate orderer config block
		helpers.CopyFile(filepath.Join("testdata", "configtx.yaml"), filepath.Join(tempDir, "configtx.yaml"))
		configtxgen := components.Configtxgen()
		configtxgen.ChannelID = "mychannel"
		configtxgen.Profile = "TwoOrgsOrdererGenesis"
		configtxgen.ConfigDir = tempDir
		configtxgen.Output = filepath.Join(tempDir, "mychannel.block")
		r := configtxgen.OutputBlock()
		err = execute(r)
		Expect(err).NotTo(HaveOccurred())

		// Generate channel transaction file
		configtxgen = components.Configtxgen()
		configtxgen.ChannelID = "mychan"
		configtxgen.Profile = "TwoOrgsChannel"
		configtxgen.ConfigDir = tempDir
		configtxgen.Output = filepath.Join(tempDir, "mychan.tx")
		r = configtxgen.OutputCreateChannelTx()
		err = execute(r)
		Expect(err).NotTo(HaveOccurred())

		// Start the orderer
		helpers.CopyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(tempDir, "orderer.yaml"))
		orderer = components.Orderer()
		orderer.ConfigDir = tempDir
		orderer.LedgerLocation = tempDir
		orderer.LogLevel = "DEBUG"

		ordererRunner = orderer.New()
		ordererProcess = ifrit.Invoke(ordererRunner)
		Eventually(ordererProcess.Ready()).Should(BeClosed())
		Consistently(ordererProcess.Wait()).ShouldNot(Receive())

		helpers.CopyFile(filepath.Join("testdata", "core.yaml"), filepath.Join(tempDir, "core.yaml"))
		peer.ConfigDir = tempDir

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
		}
		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
		}

		// Stop the running chaincode containers
		filters := map[string][]string{}
		filters["name"] = []string{"mytest-1.0"}
		allContainers, _ := client.ListContainers(docker.ListContainersOptions{
			Filters: filters,
		})

		if len(allContainers) > 0 {
			for _, container := range allContainers {
				client.RemoveContainer(docker.RemoveContainerOptions{
					ID:    container.ID,
					Force: true,
				})
			}
		}

		// Remove chaincode image
		filters = map[string][]string{}
		filters["label"] = []string{"org.hyperledger.fabric.chaincode.id.name=mytest"}
		images, _ := client.ListImages(docker.ListImagesOptions{
			Filters: filters,
		})
		if len(images) > 0 {
			for _, image := range images {
				client.RemoveImage(image.ID)
			}
		}

		os.RemoveAll(tempDir)
	})

	It("test discovery service capabilities", func() {
		By("start a peer")
		peer.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "peers", "peer0.org1.example.com", "msp")
		peerRunner := peer.NodeStart(0)
		peerProcess = ifrit.Invoke(peerRunner)
		Eventually(peerProcess.Ready()).Should(BeClosed())
		Consistently(peerProcess.Wait()).ShouldNot(Receive())

		By("create channel")
		createChan := components.Peer()
		createChan.ConfigDir = tempDir
		createChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		cRunner := createChan.CreateChannel("mychan", filepath.Join(tempDir, "mychan.tx"), "127.0.0.1:8050")
		err := execute(cRunner)
		Expect(err).NotTo(HaveOccurred())
		Eventually(ordererRunner.Err(), 30*time.Second).Should(gbytes.Say("Created and starting new chain mychan"))

		By("fetch channel block file")
		fetchRun := components.Peer()
		fetchRun.ConfigDir = tempDir
		fetchRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		fRunner := fetchRun.FetchChannel("mychan", filepath.Join(tempDir, "mychan.block"), "0", "127.0.0.1:8050")
		execute(fRunner)
		Expect(ordererRunner.Err()).To(gbytes.Say(`\Q[channel: mychan] Done delivering \E`))
		Expect(fRunner.Err()).To(gbytes.Say("Received block: 0"))

		By("join channel")
		joinRun := components.Peer()
		joinRun.ConfigDir = tempDir
		joinRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		jRunner := joinRun.JoinChannel(filepath.Join(tempDir, "mychan.block"))
		err = execute(jRunner)
		Expect(err).NotTo(HaveOccurred())
		Expect(jRunner.Err()).To(gbytes.Say("Successfully submitted proposal to join channel"))

		By("installs chaincode")
		installCC := components.Peer()
		installCC.ConfigDir = tempDir
		installCC.LogLevel = "debug"
		installCC.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		installCC.InstallChaincode("mytest", "1.0", "github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		Expect(peerRunner.Err()).To(gbytes.Say(`\QInstalled Chaincode [mytest] Version [1.0] to peer\E`))

		By("list installed chaincode")
		listInstalled := components.Peer()
		listInstalled.ConfigDir = tempDir
		listInstalled.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		listInstalled.LogLevel = "debug"
		liRunner := listInstalled.ChaincodeListInstalled()
		liProcess := ifrit.Invoke(liRunner)
		Eventually(liProcess.Ready(), 2*time.Second).Should(BeClosed())
		Eventually(liProcess.Wait(), 10*time.Second).Should(Receive(BeNil()))
		Expect(liRunner).To(gbytes.Say("Name: mytest, Version: 1.0,"))

		By("instantiate chaincode")
		instantiateCC := components.Peer()
		instantiateCC.ConfigDir = tempDir
		instantiateCC.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		instantiateCC.InstantiateChaincode("mytest", "1.0", "127.0.0.1:8050", "mychan", `{"Args":["init","a","100","b","200"]}`, "", "")

		By("list instantiated chaincode")
		listInstan := components.Peer()
		listInstan.ConfigDir = tempDir
		listInstan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		linstRunner := listInstan.ChaincodeListInstantiated("mychan")
		execute(linstRunner)
		Expect(linstRunner).To(gbytes.Say("Name: mytest, Version: 1.0"))

		By("discover peers")
		sd := getDiscoveryService(1, filepath.Join(tempDir, "config_org1.yaml"), tempDir)
		EventuallyWithOffset(1, func() bool {
			_, result := runner.VerifyAllPeersDiscovered(sd, expectedDiscoverdPeers, "mychan", "127.0.0.1:12051")
			return result
		}, time.Minute).Should(BeTrue())
		By("discover config")
		EventuallyWithOffset(1, runner.VerifyConfigDiscovered(sd, expectedConfig, "mychan", "127.0.0.1:12051"), time.Minute).Should(BeTrue())
		By("discover endorsers")
		EventuallyWithOffset(1, runner.VerifyEndorsersDiscovered(sd, expectedEndrorsementDescriptor, "mychan", "127.0.0.1:12051", "mytest", ""), time.Minute).Should(BeTrue())
	})

})

func getDiscoveryService(org int, configFilePath string, testDir string) *runner.DiscoveryService {
	userCert := filepath.Join(testDir, "crypto-config", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users",
		fmt.Sprintf("User1@org%d.example.com", org), "msp", "signcerts", fmt.Sprintf("User1@org%d.example.com-cert.pem", org))

	userKeyDir := filepath.Join(testDir, "crypto-config", "peerOrganizations", fmt.Sprintf("org%d.example.com", org), "users",
		fmt.Sprintf("User1@org%d.example.com", org), "msp", "keystore")

	return runner.SetupDiscoveryService(components.DiscoveryService(), org, configFilePath, userCert, userKeyDir)
}
