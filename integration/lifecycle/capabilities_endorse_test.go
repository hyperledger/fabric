/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
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
	"gopkg.in/yaml.v2"
)

var _ = Describe("Lifecycle with Channel v3_0 capabilities and ed25519 identities", func() {
	var (
		client  *docker.Client
		testDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		networkConfig := nwo.MultiNodeEtcdRaft()
		networkConfig.Peers = append(
			networkConfig.Peers,
			&nwo.Peer{
				Name:         "peer1",
				Organization: "Org1",
			},
		)

		network = nwo.New(networkConfig, testDir, client, StartPort(), components)
	})

	AfterEach(func() {
		// Shutdown processes and cleanup
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("invoke chaincode after upgrading Channel to V3_0 and add ed25519 peer", func() {
		network.GenerateConfigTree()
		changePeerOrOrdererToEd25519(network, "OrdererOrg", "orderer3")
		changePeerOrOrdererToEd25519(network, "Org1", "peer1")
		network.Bootstrap()

		By("starting peers' and orderers' runners")
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		org1Peer0 := network.Peer("Org1", "peer0")
		org2Peer0 := network.Peer("Org2", "peer0")
		orderer := network.Orderer("orderer1")

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

		By("setting up the channel with v3_0 capabilities and without the ed25519 peer")
		nwo.EnableChannelCapabilities(network, "testchannel", "V3_0", true, orderer, []*nwo.Orderer{orderer},
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)

		By("joining the ed25519 peer to the channel")
		ed25519Peer := network.Peer("Org1", "peer1")
		ed25519Peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: false},
		}
		network.JoinChannel("testchannel", orderer, ed25519Peer)

		By("installing chaincode mycc on ed25519 peer")
		nwo.PackageAndInstallChaincode(network, chaincode, ed25519Peer)

		By("invoking the chaincode with the ed25519 endorser and send the transaction to the ed25519 orderer")
		endorsers := []*nwo.Peer{
			ed25519Peer,
			network.Peer("Org2", "peer0"),
		}
		ed25519Orderer := network.Orderer("orderer3")
		RunQueryInvokeQuery(network, ed25519Orderer, "mycc", 90, endorsers...)
	})

	It("deploy chaincode in a Channel V3_0 and downgrade Channel to V2_0", func() {
		network.GenerateConfigTree()
		changePeerOrOrdererToEd25519(network, "Org1", "peer1")
		network.Bootstrap()

		By("starting peers' and orderers' runners")
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		orderer := network.Orderer("orderer1")
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

		By("setting up the channel with v3_0 capabilities and without the ed25519 peer")
		network.CreateAndJoinChannels(orderer)
		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
		nwo.EnableChannelCapabilities(network, "testchannel", "V3_0", false, orderer, []*nwo.Orderer{orderer},
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

		By("downgrading the channel capabilities back to v2_0")
		nwo.EnableChannelCapabilities(network, "testchannel", "V2_0", false, orderer, []*nwo.Orderer{orderer},
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)

		By("invoking the chaincode again, but expecting a failure")
		RunInvokeAndExpectFailure(network, orderer, "My_1st-Chaincode", "(ENDORSEMENT_POLICY_FAILURE)", endorsers...)
	})
})

func changePeerOrOrdererToEd25519(network *nwo.Network, orgName, entityname string) {
	genConfigFile, _ := os.Open(network.CryptoConfigPath())
	defer genConfigFile.Close()

	data, err := ioutil.ReadAll(genConfigFile)
	if err != nil {
		panic("Could no read crypto configuration")
	}

	configData := string(data)
	config := &nwo.CryptogenConfig{}

	err = yaml.Unmarshal([]byte(configData), &config)

	var orgSpecRef *nwo.OrgSpec = nil

	for i, orgSpec := range config.OrdererOrgs {
		if orgSpec.Name == orgName {
			orgSpecRef = &config.OrdererOrgs[i]
			break
		}
	}

	if orgSpecRef == nil {
		for i, orgSpec := range config.PeerOrgs {
			if orgSpec.Name == orgName {
				orgSpecRef = &config.PeerOrgs[i]
				break
			}
		}
	}

	for i, spec := range orgSpecRef.Specs {
		if spec.Hostname == entityname {
			orgSpecRef.Specs[i].PublicKeyAlgorithm = "ed25519"
			break
		}
	}

	data, err = yaml.Marshal(config)

	_ = ioutil.WriteFile(network.CryptoConfigPath(), data, fs.ModeExclusive)
}
