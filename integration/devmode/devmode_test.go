/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package devmode

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Devmode", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		process          ifrit.Process
		chaincode        nwo.Chaincode
		chaincodeRunner  *ginkgomon.Runner
		chaincodeProcess ifrit.Process
		org1peer0        *nwo.Peer
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "devmode")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member')`,
		}

		network = nwo.New(devModeSolo, testDir, client, BasePort(), components)
		network.TLSEnabled = false
		org1peer0 = network.Peer("Org1", "peer0")
		org1peer0.DevMode = true

		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if chaincodeProcess != nil {
			chaincodeProcess.Signal(syscall.SIGTERM)
		}

		if network != nil {
			network.Cleanup()
		}

		os.RemoveAll(testDir)
	})

	It("executes chaincode in dev mode", func() {
		channelName := "testchannel"
		chaincodeID := chaincode.Name + ":" + chaincode.Version
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		network.CreateAndJoinChannel(orderer, channelName)

		By("building the chaincode")
		chaincodePath, err := gexec.Build(chaincode.Path)
		Expect(err).ToNot(HaveOccurred())

		By("running the chaincode")
		peerChaincodeAddress := network.PeerAddress(org1peer0, nwo.ChaincodePort)
		cmd := exec.Command(chaincodePath, "--peer.address", peerChaincodeAddress)
		cmd.Env = append(cmd.Env, "CORE_CHAINCODE_ID_NAME="+chaincodeID, "DEVMODE_ENABLED=true")
		chaincodeRunner = ginkgomon.New(ginkgomon.Config{
			Name:              "chaincode",
			Command:           cmd,
			StartCheck:        `starting up in devmode...`,
			StartCheckTimeout: 15 * time.Second,
		})
		chaincodeProcess = ifrit.Invoke(chaincodeRunner)
		Eventually(chaincodeProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("installing the chaincode")
		nwo.InstallChaincode(network, chaincode, org1peer0)

		By("Instantiating the chaincode")
		nwo.InstantiateChaincode(network, channelName, orderer, chaincode, org1peer0, org1peer0)

		By("Querying and invoking chaincode built and started manually")
		RunQueryInvokeQuery(network, orderer, org1peer0, channelName, 100)
		Eventually(chaincodeRunner).Should(gbytes.Say("invoking in devmode"))
	})
})

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string, queryValue int) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(strconv.Itoa(queryValue)))

	By("invoke the chaincode")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode again")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(strconv.Itoa(queryValue - 10)))
}

var devModeSolo = &nwo.Config{
	Organizations: []*nwo.Organization{{
		Name:          "OrdererOrg",
		MSPID:         "OrdererMSP",
		Domain:        "example.com",
		EnableNodeOUs: false,
		Users:         0,
		CA:            &nwo.CA{Hostname: "ca"},
	}, {
		Name:          "Org1",
		MSPID:         "Org1MSP",
		Domain:        "org1.example.com",
		EnableNodeOUs: true,
		Users:         2,
		CA:            &nwo.CA{Hostname: "ca"},
	}},
	Consortiums: []*nwo.Consortium{{
		Name: "SampleConsortium",
		Organizations: []string{
			"Org1",
		},
	}},
	Consensus: &nwo.Consensus{
		Type: "solo",
	},
	SystemChannel: &nwo.SystemChannel{
		Name:    "systemchannel",
		Profile: "OneOrgOrdererGenesis",
	},
	Orderers: []*nwo.Orderer{
		{Name: "orderer", Organization: "OrdererOrg"},
	},
	Channels: []*nwo.Channel{
		{Name: "testchannel", Profile: "OneOrgChannel"},
	},
	Peers: []*nwo.Peer{{
		Name:         "peer0",
		Organization: "Org1",
		Channels: []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: true},
		},
	}},
	Profiles: []*nwo.Profile{{
		Name:     "OneOrgOrdererGenesis",
		Orderers: []string{"orderer"},
	}, {
		Name:          "OneOrgChannel",
		Consortium:    "SampleConsortium",
		Organizations: []string{"Org1"},
	}},
}
