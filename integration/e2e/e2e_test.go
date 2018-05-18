/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/tedsuo/ifrit"
)

var _ = Describe("EndToEnd", func() {
	var (
		client *docker.Client
		w      world.World
	)

	BeforeEach(func() {
		var err error

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
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
			Eventually(localProc.Wait(), 5*time.Second).Should(Receive())
			localProc.Signal(syscall.SIGKILL)
			Eventually(localProc.Wait(), 5*time.Second).Should(Receive())
		}

		// Remove any started networks
		if w.Network != nil {
			client.RemoveNetwork(w.Network.Name)
		}
	})

	It("executes a basic solo network with 2 orgs", func() {
		w = world.GenerateBasicConfig("solo", 1, 2, testDir, components)

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
		err := w.SetupChannel()
		Expect(err).NotTo(HaveOccurred())

		By("querying the chaincode")
		adminPeer := components.Peer()
		adminPeer.LogLevel = "debug"
		adminPeer.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner := adminPeer.QueryChaincode(w.Deployment.Chaincode.Name, w.Deployment.Channel, `{"Args":["query","a"]}`)
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

	It("executes a basic kafka network with 2 orgs", func() {
		By("generating files to bootstrap the network")
		w = world.GenerateBasicConfig("kafka", 2, 2, testDir, components)
		setupWorld(&w)

		By("querying the chaincode")
		adminPeer := components.Peer()
		adminPeer.LogLevel = "debug"
		adminPeer.ConfigDir = filepath.Join(testDir, "org1.example.com_0")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner := adminPeer.QueryChaincode(w.Deployment.Chaincode.Name, w.Deployment.Channel, `{"Args":["query","a"]}`)
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("100"))

	})
})

func execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	Eventually(p.Ready()).Should(BeClosed())
	Eventually(p.Wait(), 30*time.Second).Should(Receive(&err))
	return err
}
