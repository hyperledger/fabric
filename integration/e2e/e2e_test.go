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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/hyperledger/fabric/integration/world"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("EndToEnd", func() {
	var w *world.World
	var deployment world.Deployment

	BeforeEach(func() {
		deployment = world.Deployment{
			Channel: "testchannel",
			Chaincode: world.Chaincode{
				Name:     "mycc",
				Version:  "0.0",
				Path:     "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Policy:   `OR ('Org1MSP.member','Org2MSP.member')`,
			Orderer:  "127.0.0.1:7050",
		}
	})

	AfterEach(func() {
		if w != nil {
			w.Close(deployment)
		}
	})

	Describe("basic solo network with 2 orgs", func() {
		BeforeEach(func() {
			w = world.GenerateBasicConfig("solo", 1, 2, testDir, components)
		})

		It("executes a basic solo network with 2 orgs", func() {
			By("generating files to bootstrap the network")
			w.BootstrapNetwork(deployment.Channel)
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
			err := w.SetupChannel(deployment, []string{"peer0.org1.example.com", "peer0.org2.example.com"})
			Expect(err).NotTo(HaveOccurred())

			RunQueryInvokeQuery(w, deployment)
		})
	})

	Describe("basic kaka network with 2 orgs", func() {
		BeforeEach(func() {
			w = world.GenerateBasicConfig("kafka", 2, 2, testDir, components)
			w.SetupWorld(deployment)
		})

		It("executes a basic kafka network with 2 orgs", func() {
			RunQueryInvokeQuery(w, deployment)
		})
	})
})

func RunQueryInvokeQuery(w *world.World, deployment world.Deployment) {
	By("querying the chaincode")
	adminPeer := components.Peer()
	adminPeer.LogLevel = "debug"
	adminPeer.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
	adminPeer.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	adminRunner := adminPeer.QueryChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["query","a"]}`)
	execute(adminRunner)
	Eventually(adminRunner.Buffer()).Should(gbytes.Say("100"))

	By("invoking the chaincode")
	adminRunner = adminPeer.InvokeChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["invoke","a","b","10"]}`, deployment.Orderer, "--waitForEvent")
	execute(adminRunner)
	Eventually(adminRunner.Err()).Should(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode again")
	adminRunner = adminPeer.QueryChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["query","a"]}`)
	execute(adminRunner)
	Eventually(adminRunner.Buffer()).Should(gbytes.Say("90"))

	By("updating the channel")
	adminPeer = components.Peer()
	adminPeer.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
	adminPeer.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	adminRunner = adminPeer.UpdateChannel(filepath.Join(w.Rootpath, "Org1_anchors_update_tx.pb"), deployment.Channel, deployment.Orderer)
	execute(adminRunner)
	Eventually(adminRunner.Err()).Should(gbytes.Say("Successfully submitted channel update"))
}

func execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	Eventually(p.Ready()).Should(BeClosed())
	Eventually(p.Wait(), 30*time.Second).Should(Receive(&err))
	return err
}

func copyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0775)
	Expect(err).NotTo(HaveOccurred())
}

func copyPeerConfigs(peerOrgs []world.PeerOrgConfig, rootPath string) {
	for _, peerOrg := range peerOrgs {
		for peer := 0; peer < peerOrg.PeerCount; peer++ {
			peerDir := fmt.Sprintf("peer%d.%s", peer, peerOrg.Domain)
			if _, err := os.Stat(filepath.Join(rootPath, peerDir)); os.IsNotExist(err) {
				err := os.Mkdir(filepath.Join(rootPath, peerDir), 0755)
				Expect(err).NotTo(HaveOccurred())
			}
			copyFile(
				filepath.Join("testdata", fmt.Sprintf("%s_%d-core.yaml", peerOrg.Domain, peer)),
				filepath.Join(rootPath, peerDir, "core.yaml"),
			)
		}
	}
}
