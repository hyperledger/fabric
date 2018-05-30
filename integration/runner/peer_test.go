/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Peer", func() {
	var (
		tempDir   string
		cryptoDir string

		orderer        *runner.Orderer
		ordererProcess ifrit.Process
		peerProcess    ifrit.Process
		peer           *runner.Peer
		ordererRunner  *ginkgomon.Runner

		client *docker.Client
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "peer")
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

	It("starts a peer", func() {
		peer.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "peers", "peer0.org1.example.com", "msp")
		peerRunner := peer.NodeStart(0)
		peerProcess = ifrit.Invoke(peerRunner)
		Eventually(peerProcess.Ready()).Should(BeClosed())
		Consistently(peerProcess.Wait()).ShouldNot(Receive())

		By("Listing the installed chaincodes")
		installed := components.Peer()
		installed.ConfigDir = tempDir
		installed.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")

		list := installed.ChaincodeListInstalled()
		err := execute(list)
		Expect(err).NotTo(HaveOccurred())

		By("create channel")
		createChan := components.Peer()
		createChan.ConfigDir = tempDir
		createChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		cRunner := createChan.CreateChannel("mychan", filepath.Join(tempDir, "mychan.tx"), "127.0.0.1:8050")
		err = execute(cRunner)
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
		instantiateCC.InstantiateChaincode("mytest", "1.0", "127.0.0.1:8050", "mychan", `{"Args":["init","a","100","b","200"]}`, "")

		By("list instantiated chaincode")
		listInstan := components.Peer()
		listInstan.ConfigDir = tempDir
		listInstan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		linstRunner := listInstan.ChaincodeListInstantiated("mychan")
		execute(linstRunner)
		Expect(linstRunner).To(gbytes.Say("Name: mytest, Version: 1.0"))

		By("query channel")
		queryChan := components.Peer()
		queryChan.ConfigDir = tempDir
		queryChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		qRunner := queryChan.QueryChaincode("mytest", "mychan", `{"Args":["query","a"]}`)
		execute(qRunner)
		Expect(qRunner).To(gbytes.Say("100"))

		By("invoke channel")
		invokeChan := components.Peer()
		invokeChan.ConfigDir = tempDir
		invokeChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		invkeRunner := invokeChan.InvokeChaincode("mytest", "mychan", `{"Args":["invoke","a","b","10"]}`, "127.0.0.1:8050")
		execute(invkeRunner)
		Expect(invkeRunner.Err()).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

		By("setting the log level for deliver to debug")
		logRun := components.Peer()
		logRun.ConfigDir = tempDir
		logRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		lRunner := logRun.SetLogLevel("common/deliver", "debug")
		execute(lRunner)
		Expect(lRunner.Err()).To(gbytes.Say("Log level set for peer modules matching regular expression 'common/deliver': DEBUG"))

		By("fetching the latest block from the peer")
		fetchRun = components.Peer()
		fetchRun.ConfigDir = tempDir
		fetchRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		fRunner = fetchRun.FetchChannel("mychan", filepath.Join(tempDir, "newest_block.pb"), "newest", "")
		execute(fRunner)
		Expect(peerRunner.Err()).To(gbytes.Say(`\Q[channel: mychan] Done delivering \E`))
		Expect(fRunner.Err()).To(gbytes.Say("Received block: "))

		By("setting the log level for deliver to back to info")
		lRunner = logRun.SetLogLevel("common/deliver", "info")
		execute(lRunner)
		Expect(lRunner.Err()).To(gbytes.Say("Log level set for peer modules matching regular expression 'common/deliver': INFO"))
	})
})
