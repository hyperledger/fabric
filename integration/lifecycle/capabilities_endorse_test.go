/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Lifecycle with Channel V2_4 capabilities and ed25519 identities", func() {
	var (
		client    *docker.Client
		testDir   string
		network   *nwo.Network
		processes = []ifrit.Process{}
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.Ed25519Solo(), testDir, client, StartPort(), components)
	})

	AfterEach(func() {
		// Shutdown processes and cleanup
		for _, p := range processes {
			p.Signal(syscall.SIGTERM)
			Eventually(p.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("invoke chaincode with an endorser without the binaries for Channel V2_4 capabilities", func() {
		network.GenerateConfigTree()
		changeOnePeerToEd25519(network)
		network.Bootstrap()

		By("getting default runners for the orderers and peers except Org1.peer0")
		runners := []*ginkgomon.Runner{
			network.OrdererRunner(network.Orderer("orderer")),
			network.PeerRunner(network.Peer("Org1", "peer1")),
			network.PeerRunner(network.Peer("Org1", "peer2")),
			network.PeerRunner(network.Peer("Org2", "peer0")),
		}

		By("getting fabric v2.4.5 peer binary for peer Org1.peer0")
		runners = append(runners, network.OldPeerRunner(network.Peer("Org1", "peer0"), "2.4.5"))

		By("starting the runners")
		for _, runner := range runners {
			p := ifrit.Invoke(runner)
			processes = append(processes, p)
			Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		org1Peer0 := network.Peer("Org1", "peer0")
		org2Peer0 := network.Peer("Org2", "peer0")
		orderer := network.Orderer("orderer")

		By("creating and joining channels")
		network.CreateAndJoinChannels(orderer)
		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")
		By("enabling new lifecycle capabilities")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

		chaincode := nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Lang:            "golang",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `OR ('Org1MSP.peer', 'Org2MSP.peer')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_simple_chaincode",
		}

		By("deploying the chaincode")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("querying and invoking chaincode")
		RunQueryInvokeQuery(network, orderer, "mycc", 100, org1Peer0, org2Peer0)

		By("setting up the channel with v2_4 capabilities and without the ed25519 peer")
		nwo.EnableChannelCapabilities(network, "testchannel", "V2_4", true, orderer, []*nwo.Orderer{orderer},
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)

		By("failing to invoke chaincode with Org1.peer0's endorsement and expecting listening port shutdown")
		endorsers := []*nwo.Peer{
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		}
		errorMessage := fmt.Sprintf("transport: error while dialing: dial tcp %s: connect: connection refused",
			network.PeerAddress(endorsers[0], nwo.ListenPort))
		RunInvokeAndExpectFailure(network, orderer, "mycc", errorMessage, endorsers...)

		By("joining the ed25519 peer to the channel")
		ed25519Peer := network.Peer("Org1", "peer1")
		ed25519Peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: false},
		}
		network.JoinChannel("testchannel", orderer, ed25519Peer)

		By("installing chaincode mycc on ed25519 peer")
		nwo.PackageAndInstallChaincode(network, chaincode, ed25519Peer)

		By("invoking the chaincode with the ed25519 endorser")
		endorsers = []*nwo.Peer{
			ed25519Peer,
			network.Peer("Org2", "peer0"),
		}
		RunQueryInvokeQuery(network, orderer, "mycc", 90, endorsers...)
	})

	It("deploys and executes chaincode using _lifecycle with Channel v2_4 capabilities", func() {
		network.GenerateConfigTree()
		changeOnePeerToEd25519(network)
		network.Bootstrap()

		By("getting default runners for the orderers and peers except Org1.peer0")
		runners := []*ginkgomon.Runner{
			network.OrdererRunner(network.Orderer("orderer")),
			network.PeerRunner(network.Peer("Org1", "peer0")),
			network.PeerRunner(network.Peer("Org1", "peer1")),
			network.PeerRunner(network.Peer("Org1", "peer2")),
			network.PeerRunner(network.Peer("Org2", "peer0")),
		}

		By("starting the runners")
		for _, runner := range runners {
			p := ifrit.Invoke(runner)
			processes = append(processes, p)
			Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		orderer := network.Orderer("orderer")
		testPeers := network.PeersWithChannel("testchannel")

		chaincodePath := components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		chaincode := nwo.Chaincode{
			Name:                "My_1st-Chaincode",
			Version:             "Version-0.0",
			Path:                chaincodePath,
			Lang:                "binary",
			PackageFile:         filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_simple_chaincode",
		}

		By("setting up the channel with v2_4 capabilities and without the ed25519 peer")
		network.CreateAndJoinChannels(orderer)
		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
		nwo.EnableChannelCapabilities(network, "testchannel", "V2_4", false, orderer, []*nwo.Orderer{orderer},
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)

		By("joining the ed25519 peer to the channel")
		ed25519Peer := network.Peer("Org1", "peer1")
		ed25519Peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: false},
		}
		network.JoinChannel("testchannel", orderer, ed25519Peer)
		testPeers = append(testPeers, ed25519Peer)

		By("deploying the chaincode")
		nwo.PackageChaincodeBinary(chaincode)
		chaincode.SetPackageIDFromPackageFile()

		nwo.InstallChaincode(network, chaincode, testPeers...)

		By("verifying the installed chaincode package matches the one that was submitted")
		sess, err := network.PeerAdminSession(testPeers[0], commands.ChaincodeGetInstalledPackage{
			PackageID:       chaincode.PackageID,
			OutputDirectory: testDir,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		fileBytes, err := ioutil.ReadFile(chaincode.PackageFile)
		Expect(err).NotTo(HaveOccurred())
		fileBytesFromPeer, err := ioutil.ReadFile(filepath.Join(network.RootDir, chaincode.PackageID+".tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(fileBytesFromPeer).To(Equal(fileBytes))

		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, testPeers...)

		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), testPeers...)
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, testPeers[0], testPeers...)
		nwo.InitChaincode(network, "testchannel", orderer, chaincode, testPeers...)

		By("invoking the chaincode with the ed25519 endorser")
		endorsers := []*nwo.Peer{
			network.Peer("Org1", "peer1"),
			network.Peer("Org2", "peer0"),
		}
		RunQueryInvokeQuery(network, orderer, "My_1st-Chaincode", 100, endorsers...)

		By("setting the channel capabilities back to v2_0")
		nwo.EnableChannelCapabilities(network, "testchannel", "V2_0", false, orderer, []*nwo.Orderer{orderer},
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)

		By("invoking the chaincode again, but expecting a failure")
		RunInvokeAndExpectFailure(network, orderer, "My_1st-Chaincode", "(ENDORSEMENT_POLICY_FAILURE)", endorsers...)
	})
})

func copyFile(src, dst string) (int64, error) {
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	err = os.Remove(dst)
	if err != nil {
		return 0, err
	}
	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func changeOnePeerToEd25519(network *nwo.Network) {
	genConfigFile, _ := os.Open(network.CryptoConfigPath())
	defer genConfigFile.Close()

	data, err := ioutil.ReadAll(genConfigFile)
	if err != nil {
		panic("Could no read crypto configuration")
	}

	configData := string(data)
	config := &nwo.CryptogenConfig{}

	err = yaml.Unmarshal([]byte(configData), &config)

	for i, spec := range config.PeerOrgs[0].Specs {
		if spec.Hostname == "peer1" {
			config.PeerOrgs[0].Specs[i].PublicKeyAlgorithm = "ed25519"
			break
		}
	}

	data, err = yaml.Marshal(config)

	_ = ioutil.WriteFile(network.CryptoConfigPath(), data, fs.ModeExclusive)
}
