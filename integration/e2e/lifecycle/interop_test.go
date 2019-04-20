/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Release interoperability", func() {
	var (
		client  *docker.Client
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "nwo")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("solo network", func() {
		var network *nwo.Network
		var process ifrit.Process

		BeforeEach(func() {
			soloBytes, err := ioutil.ReadFile("solo.yaml")
			Expect(err).NotTo(HaveOccurred())

			var config *nwo.Config
			err = yaml.Unmarshal(soloBytes, &config)
			Expect(err).NotTo(HaveOccurred())

			network = nwo.New(config, tempDir, client, StartPort(), components)

			// Generate config and bootstrap the network
			network.GenerateConfigTree()
			network.Bootstrap()

			// Start all of the fabric processes
			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		AfterEach(func() {
			// Shutdown processes and cleanup
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
			network.Cleanup()
		})

		It("deploys and executes chaincode (simple), upgrades the channel application capabilities to V2_0 and uses _lifecycle to update the endorsement policy", func() {
			By("deploying the chaincode using LSCC on a channel with V1_4 application capabilities")
			orderer := network.Orderer("orderer0")
			peer := network.Peer("org1", "peer2")

			chaincode := nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1ExampleCom.member','Org2ExampleCom.member')`,
			}

			network.CreateAndJoinChannels(orderer)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, 100)

			By("enabling V2_0 application capabilities")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("org1", "peer1"), network.Peer("org2", "peer1"))

			By("ensuring that the chaincode is still operational after the upgrade")
			RunQueryInvokeQuery(network, orderer, peer, 90)

			By("restarting the network from persistence")
			RestartNetwork(&process, network)

			By("ensuring that the chaincode is still operational after the upgrade and restart")
			RunQueryInvokeQuery(network, orderer, peer, 80)

			By("attempting to invoke the chaincode without sufficient endorsements")
			sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
				ChannelID: "testchannel",
				Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
				Name:      "mycc",
				Ctor:      `{"Args":["invoke","a","b","10"]}`,
				PeerAddresses: []string{
					network.PeerAddress(network.Peer("org1", "peer1"), nwo.ListenPort),
				},
				WaitForEvent: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

			By("upgrading the chaincode definition using _lifecycle")
			chaincode = nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Lang:            "golang",
				PackageFile:     filepath.Join(tempDir, "simplecc.tar.gz"),
				SignaturePolicy: `OR ('Org1ExampleCom.member','Org2ExampleCom.member')`,
				Sequence:        "1",
				InitRequired:    false,
				Label:           "my_simple_chaincode",
			}
			nwo.DeployChaincodeNewLifecycle(network, "testchannel", orderer, chaincode)

			By("querying/invoking/querying the chaincode with the new definition")
			RunQueryInvokeQueryWithAddresses(network, orderer, peer, 70, network.PeerAddress(network.Peer("org1", "peer2"), nwo.ListenPort))

			By("restarting the network from persistence")
			RestartNetwork(&process, network)

			By("querying/invoking/querying the chaincode with the new definition again")
			RunQueryInvokeQueryWithAddresses(network, orderer, peer, 60, network.PeerAddress(network.Peer("org1", "peer2"), nwo.ListenPort))
		})
	})
})
