/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pluggable

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
)

var _ = Describe("EndToEnd", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process

		endorsementPluginPath string
		validationPluginPath  string
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "pluggable-suite")
		Expect(err).NotTo(HaveOccurred())

		// Compile plugins
		endorsementPluginPath = compilePlugin("endorsement")
		validationPluginPath = compilePlugin("validation")

		// Create directories for endorsement and validation activation
		dir := filepath.Join(testDir, "endorsement")
		err = os.Mkdir(dir, 0700)
		Expect(err).NotTo(HaveOccurred())
		SetEndorsementPluginActivationFolder(dir)

		dir = filepath.Join(testDir, "validation")
		err = os.Mkdir(dir, 0700)
		Expect(err).NotTo(HaveOccurred())
		SetValidationPluginActivationFolder(dir)

		// Speed up test by reducing the number of peers we bring up
		soloConfig := nwo.BasicSolo()
		soloConfig.RemovePeer("Org1", "peer1")
		soloConfig.RemovePeer("Org2", "peer1")
		Expect(soloConfig.Peers).To(HaveLen(2))

		// docker client
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(soloConfig, testDir, client, 33000, components)
		network.GenerateConfigTree()

		// modify config
		configurePlugins(network, endorsementPluginPath, validationPluginPath)

		// generate network config
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready()).Should(BeClosed())

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
		orderer := network.Orderer("orderer")
		network.CreateAndJoinChannel(orderer, "testchannel")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
	})

	AfterEach(func() {
		// stop the network
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait()).Should(Receive())

		// cleanup the network artifacts
		network.Cleanup()
		os.RemoveAll(testDir)

		// cleanup the compiled plugins
		os.Remove(endorsementPluginPath)
		os.Remove(validationPluginPath)
	})

	It("executes a basic solo network with specified plugins", func() {
		// Make sure plugins activated
		peerCount := len(network.Peers)
		activations := CountEndorsementPluginActivations()
		Expect(activations).To(Equal(peerCount))
		activations = CountValidationPluginActivations()
		Expect(activations).To(Equal(peerCount))

		RunQueryInvokeQuery(network, network.Orderer("orderer"), network.Peer("Org1", "peer0"))
	})
})

// compilePlugin compiles the plugin of the given type and returns the path for the plugin file
func compilePlugin(pluginType string) string {
	pluginFilePath := filepath.Join("testdata", "plugins", pluginType, "plugin.so")
	cmd := exec.Command(
		"go", "build", "-buildmode=plugin",
		"-o", pluginFilePath,
		fmt.Sprintf("github.com/hyperledger/fabric/integration/pluggable/testdata/plugins/%s", pluginType),
	)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))

	Expect(pluginFilePath).To(BeARegularFile())
	return pluginFilePath
}

func configurePlugins(network *nwo.Network, endorsement, validation string) {
	for _, p := range network.Peers {
		core := network.ReadPeerConfig(p)
		core.Peer.Handlers.Endorsers = fabricconfig.HandlerMap{
			"escc": fabricconfig.Handler{Name: "plugin-escc", Library: endorsement},
		}
		core.Peer.Handlers.Validators = fabricconfig.HandlerMap{
			"vscc": fabricconfig.Handler{Name: "plugin-vscc", Library: validation},
		}
		network.WritePeerConfig(p, core)
	}
}

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("100"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))
}
