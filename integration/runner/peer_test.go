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
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
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

		copyFile(filepath.Join("testdata", "cryptogen-config.yaml"), filepath.Join(tempDir, "cryptogen-config.yaml"))
		cryptogen := components.Cryptogen()
		cryptogen.Config = filepath.Join(tempDir, "cryptogen-config.yaml")
		cryptogen.Output = cryptoDir

		crypto := cryptogen.Generate()
		Expect(execute(crypto)).To(Succeed())

		// Generate orderer config block
		copyFile(filepath.Join("testdata", "configtx.yaml"), filepath.Join(tempDir, "configtx.yaml"))
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
		copyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(tempDir, "orderer.yaml"))
		orderer = components.Orderer()
		orderer.ConfigDir = tempDir
		orderer.LedgerLocation = tempDir
		orderer.LogLevel = "DEBUG"

		ordererRunner = orderer.New()
		ordererProcess = ifrit.Invoke(ordererRunner)
		Eventually(ordererProcess.Ready()).Should(BeClosed())
		Consistently(ordererProcess.Wait()).ShouldNot(Receive())

		copyFile(filepath.Join("testdata", "core.yaml"), filepath.Join(tempDir, "core.yaml"))
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
		Eventually(ordererRunner.Err(), 5*time.Second).Should(gbytes.Say("Created and starting new chain mychan"))

		By("fetch channel block file")
		fetchRun := components.Peer()
		fetchRun.ConfigDir = tempDir
		fetchRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		fRunner := fetchRun.FetchChannel("mychan", filepath.Join(tempDir, "mychan.block"), "0", "127.0.0.1:8050")
		execute(fRunner)
		time.Sleep(5 * time.Second)
		Expect(string(ordererRunner.Err().Contents())).To(ContainSubstring(`[channel: mychan] Done delivering `))
		Expect(string(fRunner.Err().Contents())).To(ContainSubstring("Received block: 0"))

		By("join channel")
		joinRun := components.Peer()
		joinRun.ConfigDir = tempDir
		joinRun.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		jRunner := joinRun.JoinChannel(filepath.Join(tempDir, "mychan.block"))
		err = execute(jRunner)
		Expect(err).NotTo(HaveOccurred())
		Expect(jRunner.Err().Contents()).To(ContainSubstring("Successfully submitted proposal to join channel"))

		By("installs chaincode")
		copyDir(filepath.Join("testdata", "chaincode"), filepath.Join(tempDir, "chaincode"))
		installCC := components.Peer()
		installCC.ConfigDir = tempDir
		installCC.LogLevel = "debug"
		installCC.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		installCC.ExecPath = os.Getenv("PATH")
		installCC.GoPath = filepath.Join(tempDir, "chaincode")
		iRunner := installCC.InstallChaincode("mytest", "1.0", filepath.Join("simple", "cmd"))
		execute(iRunner)
		Expect(string(iRunner.Err().Contents())).To(ContainSubstring(`Installed remotely response:<status:200 payload:"OK" >`))
		Expect(string(peerRunner.Err().Contents())).To(ContainSubstring(`Installed Chaincode [mytest] Version [1.0] to peer`))

		By("list installed chaincode")
		listInstalled := components.Peer()
		listInstalled.ConfigDir = tempDir
		listInstalled.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		listInstalled.LogLevel = "debug"
		liRunner := listInstalled.ChaincodeListInstalled()
		liProcess := ifrit.Invoke(liRunner)
		Eventually(liProcess.Ready(), 2*time.Second).Should(BeClosed())
		Eventually(liRunner.Buffer()).Should(gbytes.Say("Path: simple/cmd"))
		Eventually(liProcess.Wait(), 5*time.Second).ShouldNot(Receive(BeNil()))
		Expect(liRunner.Buffer().Contents()).To(ContainSubstring("Name: mytest"))
		Expect(liRunner.Buffer().Contents()).To(ContainSubstring("Version: 1.0"))

		By("instantiate channel")
		instantiateCC := components.Peer()
		instantiateCC.ConfigDir = tempDir
		instantiateCC.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		instRunner := instantiateCC.InstantiateChaincode("mytest", "1.0", "127.0.0.1:8050", "mychan", `{"Args":["init","a","100","b","200"]}`, "")
		instProcess := ifrit.Invoke(instRunner)
		Eventually(instProcess.Ready(), 2*time.Second).Should(BeClosed())
		Eventually(instProcess.Wait(), 10*time.Second).ShouldNot(Receive(BeNil()))

		By("list instantiated chaincode")
		listInstantiated := func() bool {
			listInstan := components.Peer()
			listInstan.ConfigDir = tempDir
			listInstan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
			linstRunner := listInstan.ChaincodeListInstantiated("mychan")
			err := execute(linstRunner)
			if err != nil {
				return false
			}
			return strings.Contains(string(linstRunner.Buffer().Contents()), fmt.Sprintf("Path: %s", "simple/cmd"))
		}
		Eventually(listInstantiated, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

		listInstan := components.Peer()
		listInstan.ConfigDir = tempDir
		listInstan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		linstRunner := listInstan.ChaincodeListInstantiated("mychan")
		execute(linstRunner)
		Expect(linstRunner.Buffer().Contents()).To(ContainSubstring("Name: mytest"))
		Expect(linstRunner.Buffer().Contents()).To(ContainSubstring("Version: 1.0"))

		By("query channel")
		queryChan := components.Peer()
		queryChan.ConfigDir = tempDir
		queryChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		qRunner := queryChan.QueryChaincode("mytest", "mychan", `{"Args":["query","a"]}`)
		execute(qRunner)
		Expect(qRunner.Buffer().Contents()).To(ContainSubstring("100"))

		By("invoke channel")
		invokeChan := components.Peer()
		invokeChan.ConfigDir = tempDir
		invokeChan.MSPConfigPath = filepath.Join(cryptoDir, "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		invkeRunner := invokeChan.InvokeChaincode("mytest", "mychan", `{"Args":["invoke","a","b","10"]}`, "127.0.0.1:8050")
		execute(invkeRunner)
		Expect(invkeRunner.Err().Contents()).To(ContainSubstring("Chaincode invoke successful. result: status:200"))
	})

})
