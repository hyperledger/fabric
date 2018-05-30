/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger/fabric/integration/world"
)

var _ = Describe("EndToEnd", func() {
	var w *world.World
	var deployment world.Deployment

	BeforeEach(func() {
		w = world.GenerateBasicConfig("solo", 1, 2, testDir, components)
		deployment = world.Deployment{
			Channel: "testchannel",
			Chaincode: world.Chaincode{
				Name:     "mycc",
				Version:  "0.0",
				Path:     filepath.Join("github.com", "hyperledger", "fabric", "integration", "chaincode", "simple", "cmd"),
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Policy:   `OR ('Org1MSP.member','Org2MSP.member')`,
			Orderer:  "127.0.0.1:7050",
		}
		w.SetupWorld(deployment)
	})

	AfterEach(func() {
		if w != nil {
			w.Close(deployment)
		}
	})

	It("executes a basic solo network with specified plugins", func() {
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

		By("querying the chaincode")
		adminPeer := components.Peer()
		adminPeer.LogLevel = "debug"
		adminPeer.ConfigDir = filepath.Join(testDir, "peer0.org1.example.com")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner := adminPeer.QueryChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["query","a"]}`)
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("100"))

		By("invoking the chaincode")
		adminRunner = adminPeer.InvokeChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["invoke","a","b","10"]}`, deployment.Orderer)
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say("Chaincode invoke successful. result: status:200"))

		By("querying the chaincode again")
		adminRunner = adminPeer.QueryChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["query","a"]}`)
		execute(adminRunner)
		Eventually(adminRunner.Buffer()).Should(gbytes.Say("90"))

		By("updating the channel")
		adminPeer = components.Peer()
		adminPeer.ConfigDir = filepath.Join(testDir, "peer0.org1.example.com")
		adminPeer.MSPConfigPath = filepath.Join(testDir, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
		adminRunner = adminPeer.UpdateChannel(filepath.Join(testDir, "Org1_anchors_update_tx.pb"), deployment.Channel, deployment.Orderer)
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say("Successfully submitted channel update"))
	})
})

func execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	Eventually(p.Ready()).Should(BeClosed())
	Eventually(p.Wait(), 10*time.Second).Should(Receive(&err))
	return err
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
