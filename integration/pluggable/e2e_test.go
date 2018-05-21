/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("EndToEnd", func() {
	var (
		testdataDir string
		client      *docker.Client
		network     *docker.Network
		w           world.World
	)

	BeforeEach(func() {
		var err error

		testdataDir, err = filepath.Abs("testdata")
		Expect(err).NotTo(HaveOccurred())
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		pOrg := []*localconfig.Organization{{
			Name:   "Org1",
			ID:     "Org1MSP",
			MSPDir: "crypto/peerOrganizations/org1.example.com/msp",
			AnchorPeers: []*localconfig.AnchorPeer{{
				Host: "0.0.0.0",
				Port: 7051,
			}},
		}, {
			Name:   "Org2",
			ID:     "Org2MSP",
			MSPDir: "crypto/peerOrganizations/org2.example.com/msp",
			AnchorPeers: []*localconfig.AnchorPeer{{
				Host: "0.0.0.0",
				Port: 8051,
			}},
		}}

		ordererOrgs := world.OrdererConfig{
			OrganizationName: "OrdererOrg",
			Domain:           "example.com",
			OrdererNames:     []string{"orderer"},
			BrokerCount:      0,
		}

		peerOrgs := []world.PeerOrgConfig{{
			OrganizationName: pOrg[0].Name,
			Domain:           "org1.example.com",
			EnableNodeOUs:    false,
			UserCount:        1,
			PeerCount:        1,
		}, {
			OrganizationName: pOrg[1].Name,
			Domain:           "org2.example.com",
			EnableNodeOUs:    false,
			UserCount:        1,
			PeerCount:        1,
		}}

		oOrg := []*localconfig.Organization{{
			Name:   ordererOrgs.OrganizationName,
			ID:     "OrdererMSP",
			MSPDir: filepath.Join("crypto", "ordererOrganizations", "example.com", "orderers", "orderer.example.com", "msp"),
		}}

		deployment := world.Deployment{
			SystemChannel: "systestchannel",
			Channel:       "testchannel",
			Chaincode: world.Chaincode{
				Name:     "mycc",
				Version:  "1.0",
				Path:     filepath.Join("simple", "cmd"),
				GoPath:   filepath.Join(testDir, "chaincode"),
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Peers:    []string{"peer0.org1.example.com", "peer0.org2.example.com"},
			Policy:   `OR ('Org1MSP.member','Org2MSP.member')`,
			Orderer:  "127.0.0.1:7050",
		}

		peerProfile := localconfig.Profile{
			Consortium: "SampleConsortium",
			Application: &localconfig.Application{
				Organizations: pOrg,
				Capabilities: map[string]bool{
					"V1_2": true,
				},
			},
			Capabilities: map[string]bool{
				"V1_1": true,
			},
		}

		orderer := &localconfig.Orderer{
			BatchTimeout: 1 * time.Second,
			BatchSize: localconfig.BatchSize{
				MaxMessageCount:   1,
				AbsoluteMaxBytes:  (uint32)(98 * 1024 * 1024),
				PreferredMaxBytes: (uint32)(512 * 1024),
			},
			Kafka: localconfig.Kafka{
				Brokers: []string{
					"127.0.0.1:9092",
					"127.0.0.1:8092",
					"127.0.0.1:7092",
					"127.0.0.1:6092",
				},
			},
			Organizations: oOrg,
			OrdererType:   "solo",
			Addresses:     []string{"0.0.0.0:7050"},
			Capabilities:  map[string]bool{"V1_1": true},
		}

		ordererProfile := localconfig.Profile{
			Application: &localconfig.Application{
				Organizations: oOrg,
				Capabilities:  map[string]bool{"V1_2": true}},
			Orderer: orderer,
			Consortiums: map[string]*localconfig.Consortium{
				"SampleConsortium": &localconfig.Consortium{
					Organizations: append(oOrg, pOrg...),
				},
			},
			Capabilities: map[string]bool{"V1_1": true},
		}

		profiles := map[string]localconfig.Profile{
			"TwoOrgsOrdererGenesis": ordererProfile,
			"TwoOrgsChannel":        peerProfile,
		}

		crypto := runner.Cryptogen{
			Config: filepath.Join(testDir, "crypto.yaml"),
			Output: filepath.Join(testDir, "crypto"),
		}

		w = world.World{
			Rootpath:           testDir,
			Components:         components,
			Cryptogen:          crypto,
			Deployment:         deployment,
			OrdererOrgs:        []world.OrdererConfig{ordererOrgs},
			PeerOrgs:           peerOrgs,
			OrdererProfileName: "TwoOrgsOrdererGenesis",
			ChannelProfileName: "TwoOrgsChannel",
			Profiles:           profiles,
		}
	})

	AfterEach(func() {
		// Stop the docker constainers for zookeeper and kafka
		for _, cont := range w.LocalStoppers {
			cont.Stop()
		}

		// Stop the running chaincode containers
		filters := map[string][]string{}
		filters["name"] = []string{fmt.Sprintf("%s-%s", w.Deployment.Chaincode.Name, w.Deployment.Chaincode.Version)}
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
		filters["label"] = []string{fmt.Sprintf("org.hyperledger.fabric.chaincode.id.name=%s", w.Deployment.Chaincode.Name)}
		images, _ := client.ListImages(docker.ListImagesOptions{
			Filters: filters,
		})
		if len(images) > 0 {
			for _, image := range images {
				client.RemoveImage(image.ID)
			}
		}

		// Stop the orderers and peers
		for _, localProc := range w.LocalProcess {
			localProc.Signal(syscall.SIGTERM)
		}

		// Remove any started networks
		if network != nil {
			client.RemoveNetwork(network.Name)
		}
	})

	It("executes a basic solo network with 2 orgs", func() {
		By("generating files to bootstrap the network")
		w.BootstrapNetwork()
		Expect(filepath.Join(testDir, "configtx.yaml")).To(BeARegularFile())
		Expect(filepath.Join(testDir, "crypto.yaml")).To(BeARegularFile())
		Expect(filepath.Join(testDir, "crypto", "peerOrganizations")).To(BeADirectory())
		Expect(filepath.Join(testDir, "crypto", "ordererOrganizations")).To(BeADirectory())
		Expect(filepath.Join(testDir, "systestchannel_block.pb")).To(BeARegularFile())
		Expect(filepath.Join(testDir, "testchannel_tx.pb")).To(BeARegularFile())
		Expect(filepath.Join(testDir, "Org1_anchors_update_tx.pb")).To(BeARegularFile())
		Expect(filepath.Join(testDir, "Org2_anchors_update_tx.pb")).To(BeARegularFile())

		By("setting up directories for the network")
		copyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(testDir, "orderer.yaml"))
		copyPeerConfigs(w.PeerOrgs, w.Rootpath)

		By("building the network")
		w.BuildNetwork()

		By("setting up the channel")
		copyDir(filepath.Join("testdata", "chaincode"), filepath.Join(testDir, "chaincode"))
		err := w.SetupChannel()
		Expect(err).NotTo(HaveOccurred())

		// count peers
		peerCount := 0
		for _, peerOrg := range w.PeerOrgs {
			peerCount += peerOrg.PeerCount
		}
		// Make sure plugins activated
		activations := CountEndorsementPluginActivations()
		Expect(activations).To(Equal(peerCount))
		activations = CountValidationPluginActivations()
		Expect(activations).To(Equal(peerCount))

		By("verifying the chaincode is installed")
		adminPeer := components.Peer()
		adminPeer.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner := adminPeer.ChaincodeListInstalled()
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("Path: simple/cmd"))

		By("waiting for the chaincode to complete instantiation")
		listInstantiated := func() bool {
			p := components.Peer()
			p.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
			p.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
			adminRunner := p.ChaincodeListInstantiated(w.Deployment.Channel)
			err := execute(adminRunner)
			if err != nil {
				return false
			}
			return strings.Contains(string(adminRunner.Buffer().Contents()), "Path: simple/cmd")
		}
		Eventually(listInstantiated, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

		By("querying the chaincode")
		adminPeer = components.Peer()
		adminPeer.LogLevel = "debug"
		adminPeer.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner = adminPeer.QueryChaincode(w.Deployment.Chaincode.Name, w.Deployment.Channel, `{"Args":["query","a"]}`)
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("100"))

		By("invoking the chaincode")
		adminRunner = adminPeer.InvokeChaincode(w.Deployment.Chaincode.Name, w.Deployment.Channel, `{"Args":["invoke","a","b","10"]}`, w.Deployment.Orderer)
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say("Chaincode invoke successful. result: status:200"))

		By("querying the chaincode again")
		adminRunner = adminPeer.QueryChaincode(w.Deployment.Chaincode.Name, w.Deployment.Channel, `{"Args":["query","a"]}`)
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("90"))

		By("updating the channel")
		adminPeer = components.Peer()
		adminPeer.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner = adminPeer.UpdateChannel(filepath.Join(testDir, "Org1_anchors_update_tx.pb"), w.Deployment.Channel, w.Deployment.Orderer)
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say("Successfully submitted channel update"))
	})
})

func copyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0775)
	Expect(err).NotTo(HaveOccurred())
}

func copyDir(src, dest string) {
	os.MkdirAll(dest, 0755)
	objects, err := ioutil.ReadDir(src)
	for _, obj := range objects {
		srcfileptr := src + "/" + obj.Name()
		destfileptr := dest + "/" + obj.Name()
		if obj.IsDir() {
			copyDir(srcfileptr, destfileptr)
		} else {
			copyFile(srcfileptr, destfileptr)
		}
	}
	Expect(err).NotTo(HaveOccurred())
}

func execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	Eventually(p.Ready()).Should(BeClosed())
	Eventually(p.Wait(), 10*time.Second).Should(Receive(&err))
	return err
}

func copyPeerConfigs(peerOrgs []world.PeerOrgConfig, rootPath string) {
	for _, peerOrg := range peerOrgs {
		for peer := 0; peer < peerOrg.PeerCount; peer++ {
			peerDir := fmt.Sprintf("%s_%d", peerOrg.Domain, peer)
			if _, err := os.Stat(filepath.Join(rootPath, peerDir)); os.IsNotExist(err) {
				err := os.Mkdir(filepath.Join(rootPath, peerDir), 0755)
				Expect(err).NotTo(HaveOccurred())
			}
			copyFile(filepath.Join("testdata", fmt.Sprintf("%s-core.yaml", peerDir)),
				filepath.Join(rootPath, peerDir, "core.yaml"))
		}
	}
}

// compilePlugin compiles the plugin of the given type and returns the path for the plugin file
func compilePlugin(pluginType string) string {
	pluginFilePath := filepath.Join("testdata", "plugins", pluginType, "plugin.so")
	cmd := exec.Command(
		"go",
		append([]string{
			"build", "-buildmode=plugin", "-o", pluginFilePath,
			fmt.Sprintf("github.com/hyperledger/fabric/integration/pluggable/testdata/plugins/%s", pluginType),
		})...,
	)
	cmd.Run()
	Expect(pluginFilePath).To(BeARegularFile())
	return pluginFilePath
}
