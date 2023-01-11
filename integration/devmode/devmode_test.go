/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package devmode

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Devmode", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
		chaincode                   nwo.Chaincode
		legacyChaincode             nwo.Chaincode
		chaincodeRunner             *ginkgomon.Runner
		chaincodeProcess            ifrit.Process
		channelName                 string
	)

	BeforeEach(func() {
		var err error
		channelName = "testchannel"
		testDir, err = ioutil.TempDir("", "devmode")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(devModeEtcdraft, testDir, client, StartPort(), components)

		network.TLSEnabled = false
		network.Peer("Org1", "peer0").DevMode = true

		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if chaincodeProcess != nil {
			chaincodeProcess.Signal(syscall.SIGTERM)
		}

		if network != nil {
			network.Cleanup()
		}

		os.RemoveAll(testDir)
	})

	It("executes chaincode in dev mode using legacy lifecycle", func() {
		legacyChaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member')`,
		}

		org1peer0 := network.Peer("Org1", "peer0")
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		By("building chaincode")
		chaincodeExecutePath := components.Build(legacyChaincode.Path)

		By("running the chaincode")
		legacyChaincodeID := legacyChaincode.Name + ":" + legacyChaincode.Version
		peerChaincodeAddress := network.PeerAddress(org1peer0, nwo.ChaincodePort)
		envs := []string{
			"CORE_PEER_TLS_ENABLED=false",
			"CORE_CHAINCODE_ID_NAME=" + legacyChaincodeID,
			"DEVMODE_ENABLED=true",
		}
		cmd := exec.Command(chaincodeExecutePath, "-peer.address", peerChaincodeAddress)
		cmd.Env = append(cmd.Env, envs...)
		chaincodeRunner = ginkgomon.New(ginkgomon.Config{
			Name:              "chaincode",
			Command:           cmd,
			StartCheckTimeout: 15 * time.Second,
			StartCheck:        "starting up in devmode...",
		})
		chaincodeProcess = ifrit.Invoke(chaincodeRunner)
		Eventually(chaincodeProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("installing the chaincode")
		nwo.InstallChaincodeLegacy(network, legacyChaincode, org1peer0)

		By("instantiating the chaincode")
		nwo.InstantiateChaincodeLegacy(network, channelName, orderer, legacyChaincode, org1peer0, org1peer0)

		By("querying and invoking the chaincode")
		RunQueryInvokeQuery(network, orderer, org1peer0, channelName, 100)
		Eventually(chaincodeRunner).Should(gbytes.Say("invoking in devmode"))
	})

	It("executes chaincode in dev mode", func() {
		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `OR ('Org1MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_prebuilt_chaincode",
		}

		org1peer0 := network.Peer("Org1", "peer0")
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		By("enabling V2_0 application capabilities")
		nwo.EnableCapabilities(network, channelName, "Application", "V2_0", orderer, org1peer0)

		By("running the chaincode")
		chaincodeID := chaincode.Name + ":" + chaincode.Version
		peerChaincodeAddress := network.PeerAddress(org1peer0, nwo.ChaincodePort)
		envs := []string{
			"CORE_PEER_TLS_ENABLED=false",
			"CORE_PEER_ADDRESS=" + peerChaincodeAddress,
			"CORE_CHAINCODE_ID_NAME=" + chaincodeID,
			"DEVMODE_ENABLED=true",
		}

		cmd := exec.Command(chaincode.Path, "-peer.address", peerChaincodeAddress)
		cmd.Env = append(cmd.Env, envs...)
		chaincodeRunner = ginkgomon.New(ginkgomon.Config{
			Name:              "chaincode",
			Command:           cmd,
			StartCheckTimeout: 15 * time.Second,
			StartCheck:        "starting up in devmode...",
		})
		chaincodeProcess = ifrit.Invoke(chaincodeRunner)
		Eventually(chaincodeProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("approving chaincode for orgs")
		ApproveChaincodeForMyOrg(network, channelName, orderer, chaincode, org1peer0)
		By("committing the chaincode definition")
		nwo.CheckCommitReadinessUntilReady(network, channelName, chaincode, network.PeerOrgs(), org1peer0)
		nwo.CommitChaincode(network, channelName, orderer, chaincode, org1peer0, org1peer0)
		By("initializing chaincode if required")
		if chaincode.InitRequired {
			nwo.InitChaincode(network, channelName, orderer, chaincode, org1peer0)
		}

		By("querying and invoking the chaincode")
		RunQueryInvokeQuery(network, orderer, org1peer0, channelName, 100)
		Eventually(chaincodeRunner.Buffer()).Should(gbytes.Say("invoking in devmode"))

		By("killing chaincode process")
		chaincodeProcess.Signal(syscall.SIGKILL)
		Eventually(chaincodeProcess.Wait(), network.EventuallyTimeout).Should(Receive())

		By("invoking chaincode after it has been killed, expecting it to fail")
		sess, err := network.PeerUserSession(org1peer0, "User1", commands.ChaincodeInvoke{
			ChannelID: channelName,
			Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
			Name:      "mycc",
			Ctor:      `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: []string{
				network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
			},
			WaitForEvent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))

		By("restarting the chaincode process")
		cmd = exec.Command(chaincode.Path, []string{"-peer.address", peerChaincodeAddress}...)
		cmd.Env = append(cmd.Env, envs...)
		chaincodeRunner = ginkgomon.New(ginkgomon.Config{
			Name:              "chaincode",
			Command:           cmd,
			StartCheckTimeout: 15 * time.Second,
			StartCheck:        "starting up in devmode...",
		})
		chaincodeProcess = ifrit.Invoke(chaincodeRunner)
		Eventually(chaincodeProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("querying and invoking the chaincode")
		RunQueryInvokeQuery(network, orderer, org1peer0, channelName, 90)
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

	By("invoking the chaincode")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
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

// ApproveChaincodeForMyOrg should only be used when devmode is enabled.
// It does not set PackageID form ChaincodeApproveForMyOrg command.
func ApproveChaincodeForMyOrg(n *nwo.Network, channel string, orderer *nwo.Orderer, chaincode nwo.Chaincode, peers ...*nwo.Peer) {
	approvedOrgs := map[string]bool{}
	for _, p := range peers {
		if _, ok := approvedOrgs[p.Organization]; !ok {
			sess, err := n.PeerAdminSession(p, commands.ChaincodeApproveForMyOrg{
				ChannelID:           channel,
				Orderer:             n.OrdererAddress(orderer, nwo.ListenPort),
				Name:                chaincode.Name,
				Version:             chaincode.Version,
				Sequence:            chaincode.Sequence,
				EndorsementPlugin:   chaincode.EndorsementPlugin,
				ValidationPlugin:    chaincode.ValidationPlugin,
				SignaturePolicy:     chaincode.SignaturePolicy,
				ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
				InitRequired:        chaincode.InitRequired,
				CollectionsConfig:   chaincode.CollectionsConfig,
				ClientAuth:          n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
			approvedOrgs[p.Organization] = true
			Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
		}
	}
}

var devModeEtcdraft = &nwo.Config{
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
	Consensus: &nwo.Consensus{
		Type:                        "etcdraft",
		BootstrapMethod:             "none",
		ChannelParticipationEnabled: true,
	},
	Orderers: []*nwo.Orderer{
		{Name: "orderer", Organization: "OrdererOrg"},
	},
	Channels: []*nwo.Channel{
		{
			Name:    "testchannel",
			Profile: "OneOrgChannel",
		},
	},
	Peers: []*nwo.Peer{{
		Name:         "peer0",
		Organization: "Org1",
		Channels: []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: true},
		},
	}},
	Profiles: []*nwo.Profile{{
		Name:          "OneOrgChannel",
		Consortium:    "SampleConsortium",
		Orderers:      []string{"orderer"},
		Organizations: []string{"Org1"},
	}},
}
