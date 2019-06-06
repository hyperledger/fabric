/*
Copyright IBM Corp All Rights Reserved.

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

var _ = Describe("Lifecycle", func() {
	var (
		client  *docker.Client
		tempDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "nwo")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

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

		os.RemoveAll(tempDir)
	})

	It("deploys and executes chaincode (simple) using _lifecycle and upgrades it", func() {
		orderer := network.Orderer("orderer0")
		testPeers := network.PeersWithChannel("testchannel")
		org1peer2 := network.Peer("org1", "peer2")

		chaincode := nwo.Chaincode{
			Name:                "mycc",
			Version:             "0.0",
			Path:                "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Lang:                "golang",
			PackageFile:         filepath.Join(tempDir, "simplecc.tar.gz"),
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			Policy:              `AND ('Org1ExampleCom.member','Org2ExampleCom.member')`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_simple_chaincode",
		}

		network.CreateAndJoinChannels(orderer)

		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("org1", "peer1"), network.Peer("org2", "peer1"))
		nwo.DeployChaincodeNewLifecycle(network, "testchannel", orderer, chaincode)

		RunQueryInvokeQuery(network, orderer, org1peer2, 100)

		By("setting a bad package ID to temporarily disable endorsements on org1")
		savedPackageID := chaincode.PackageID
		// note that in theory it should be sufficient to set it to an
		// empty string, but the ApproveChaincodeForMyOrgNewLifecycle
		// function fills the packageID field if empty
		chaincode.PackageID = "bad"
		nwo.ApproveChaincodeForMyOrgNewLifecycle(network, "testchannel", orderer, chaincode, org1peer2)

		By("querying the chaincode and expecting the invocation to fail")
		sess, err := network.PeerUserSession(org1peer2, "User1", commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "mycc",
			Ctor:      `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say("Error: endorsement failure during query. response: status:500 " +
			"message:\"make sure the chaincode mycc has been successfully defined on channel testchannel and try " +
			"again: chaincode definition for 'mycc' exists, but chaincode is not installed\""))

		By("setting the correct package ID to restore the chaincode")
		chaincode.PackageID = savedPackageID
		nwo.ApproveChaincodeForMyOrgNewLifecycle(network, "testchannel", orderer, chaincode, org1peer2)

		By("querying the chaincode and expecting the invocation to succeed")
		sess, err = network.PeerUserSession(org1peer2, "User1", commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "mycc",
			Ctor:      `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say("90"))

		By("upgrading the chaincode to sequence 2")
		chaincode.Sequence = "2"

		nwo.ApproveChaincodeForMyOrgNewLifecycle(network, "testchannel", orderer, chaincode, testPeers...)
		nwo.EnsureApproved(network, "testchannel", chaincode, network.PeerOrgs(), testPeers...)

		nwo.CommitChaincodeNewLifecycle(network, "testchannel", orderer, chaincode, testPeers[0], testPeers...)

		RunQueryInvokeQuery(network, orderer, testPeers[0], 90)
	})
})
