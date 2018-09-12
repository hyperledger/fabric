/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/discovery"
	pm "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	yaml "gopkg.in/yaml.v2"
)

var _ = Describe("DiscoveryService", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process
		orderer *nwo.Orderer
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-sd")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		configBytes, err := ioutil.ReadFile(filepath.Join("testdata", "network.yaml"))
		Expect(err).NotTo(HaveOccurred())

		var networkConfig *nwo.Config
		err = yaml.Unmarshal(configBytes, &networkConfig)
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(networkConfig, testDir, client, 35000+1000*GinkgoParallelNode(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready()).Should(BeClosed())

		orderer = network.Orderer("orderer")
		network.CreateAndJoinChannel(orderer, "testchannel")
		network.UpdateChannelAnchors(orderer, "testchannel")
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), time.Minute).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("discovers channel information", func() {
		org1Peer0 := network.Peer("org1", "peer0")
		org2Peer0 := network.Peer("org2", "peer0")
		org3Peer0 := network.Peer("org3", "peer0")

		By("discovering endorsers when missing chaincode")
		endorsers := commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc",
		}
		sess, err := network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("installing and instantiating chaincode on org1.peer0")
		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "1.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR (AND ('Org1MSP.member','Org2MSP.member'), AND ('Org1MSP.member','Org3MSP.member'), AND ('Org2MSP.member','Org3MSP.member'))`,
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode, org1Peer0)

		By("discovering endorsers for chaincode that has not been installed to enough orgs to satisy endorsement policy")
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("installing chaincode to enough organizations to satisfy the endorsement policy")
		nwo.InstallChaincode(network, chaincode, org2Peer0)

		By("discovering endorsers for chaincode that has been installed to some orgs")
		de := discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), time.Minute).Should(ConsistOf(
			[]DiscoveredPeer{toDiscoveredPeer(network, org1Peer0)},
			[]DiscoveredPeer{toDiscoveredPeer(network, org2Peer0)},
		))
		discovered := de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("installing chaincode to all orgs")
		nwo.InstallChaincode(network, chaincode, org3Peer0)

		By("discovering endorsers for chaincode that has been installed to all orgs")
		Eventually(endorsersByGroups(de), time.Minute).Should(ConsistOf(
			[]DiscoveredPeer{toDiscoveredPeer(network, org1Peer0)},
			[]DiscoveredPeer{toDiscoveredPeer(network, org2Peer0)},
			[]DiscoveredPeer{toDiscoveredPeer(network, org3Peer0)},
		))

		By("upgrading chaincode and adding a collections config")
		chaincode.Name = "mycc"
		chaincode.Version = "2.0"
		chaincode.CollectionsConfig = filepath.Join("testdata", "collections_config1.json")
		nwo.UpgradeChaincode(network, "testchannel", orderer, chaincode, org1Peer0, org2Peer0, org3Peer0)

		By("discovering endorsers for chaincode with a private collection")
		endorsers.Collection = "mycc:collectionMarbles"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), time.Minute).Should(ConsistOf(
			[]DiscoveredPeer{toDiscoveredPeer(network, org1Peer0)},
			[]DiscoveredPeer{toDiscoveredPeer(network, org2Peer0)},
		))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("installing chaincode to all peers")
		nwo.DeployChaincode(network, "testchannel", orderer, nwo.Chaincode{
			Name:    "mycc2",
			Version: "1.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member', 'Org2MSP.member', 'Org3MSP.member')`,
		})

		By("discovering endorsers for chaincode that has been installed to all peers")
		endorsers.Collection = ""
		endorsers.Chaincode = "mycc2"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), time.Minute).Should(ConsistOf(
			ConsistOf(toDiscoveredPeer(network, org1Peer0), toDiscoveredPeer(network, network.Peer("org1", "peer1"))),
			ConsistOf(toDiscoveredPeer(network, org2Peer0), toDiscoveredPeer(network, network.Peer("org2", "peer1"))),
			ConsistOf(toDiscoveredPeer(network, org3Peer0), toDiscoveredPeer(network, network.Peer("org3", "peer1"))),
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1), uint32(1)))

		By("changing the channel policy")
		currentConfig := nwo.GetConfigBlock(network, network.Peer("org3", "peer0"), orderer, "testchannel")
		updatedConfig := proto.Clone(currentConfig).(*common.Config)
		updatedConfig.ChannelGroup.Groups["Application"].Groups["org3"].Policies["Writers"].Policy.Value = utils.MarshalOrPanic(cauthdsl.SignedByMspAdmin("Org3MSP"))
		nwo.UpdateConfig(network, orderer, "testchannel", currentConfig, updatedConfig, network.Peer("org3", "peer0"))

		By("trying to discover peers as an org 3 member")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org3Peer0, "User1"),
			UserKey:   network.PeerUserKey(org3Peer0, "User1"),
			MSPID:     network.Organization(org3Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org3Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc",
		}
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`access denied`))
	})

	It("discovers peer membership", func() {
		org1Peer0 := network.Peer("org1", "peer0")

		By("discovering peers")
		Eventually(discoverPeers(network, org1Peer0, "User1", "testchannel"), time.Minute).Should(ConsistOf(
			toDiscoveredPeer(network, network.Peer("org1", "peer0")),
			toDiscoveredPeer(network, network.Peer("org1", "peer1")),
			toDiscoveredPeer(network, network.Peer("org2", "peer0")),
			toDiscoveredPeer(network, network.Peer("org2", "peer1")),
			toDiscoveredPeer(network, network.Peer("org3", "peer0")),
			toDiscoveredPeer(network, network.Peer("org3", "peer1")),
		))

		By("installing and instantiating chaincode on a peer")
		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "1.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode, org1Peer0)

		By("discovering peers after installing and instantiating chaincode on a peer")
		dp := discoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc"), time.Minute).Should(HaveLen(1))
		peersWithCC := peersWithChaincode(dp, "mycc")()
		Expect(peersWithCC).To(ConsistOf(toDiscoveredPeer(network, org1Peer0, "mycc")))
	})

	It("discovers network configuration information", func() {
		org1Peer0 := network.Peer("org1", "peer0")

		By("retrieving the configuration")
		config := commands.Config{
			UserCert: network.PeerUserCert(org1Peer0, "User1"),
			UserKey:  network.PeerUserKey(org1Peer0, "User1"),
			MSPID:    network.Organization(org1Peer0.Organization).MSPID,
			Server:   network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:  "testchannel",
		}
		sess, err := network.Discover(config)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(0))

		By("unmarshaling the response")
		discoveredConfig := &discovery.ConfigResult{}
		err = json.Unmarshal(sess.Out.Contents(), &discoveredConfig)
		Expect(err).NotTo(HaveOccurred())

		By("validating the membership data")
		Expect(discoveredConfig.Msps).To(HaveLen(len(network.Organizations)))
		for _, o := range network.Orderers {
			org := network.Organization(o.Organization)
			mspConfig, err := msp.GetVerifyingMspConfig(network.OrdererOrgMSPDir(org), org.MSPID, "bccsp")
			Expect(err).NotTo(HaveOccurred())
			Expect(discoveredConfig.Msps[org.MSPID]).To(Equal(unmarshalFabricMSPConfig(mspConfig)))
		}
		for _, p := range network.Peers {
			org := network.Organization(p.Organization)
			mspConfig, err := msp.GetVerifyingMspConfig(network.PeerOrgMSPDir(org), org.MSPID, "bccsp")
			Expect(err).NotTo(HaveOccurred())
			Expect(discoveredConfig.Msps[org.MSPID]).To(Equal(unmarshalFabricMSPConfig(mspConfig)))
		}

		By("validating  the orderers")
		Expect(discoveredConfig.Orderers).To(HaveLen(len(network.Orderers)))
		for _, orderer := range network.Orderers {
			ordererMSPID := network.Organization(orderer.Organization).MSPID
			Expect(discoveredConfig.Orderers[ordererMSPID].Endpoint).To(ConsistOf(
				&discovery.Endpoint{Host: "127.0.0.1", Port: uint32(network.OrdererPort(orderer, nwo.ListenPort))},
			))
		}
	})
})

type DiscoveredPeer struct {
	MSPID      string
	Endpoint   string
	Identity   string
	Chaincodes []string
}

func discoverPeers(n *nwo.Network, p *nwo.Peer, user, channelName string) func() []DiscoveredPeer {
	return func() []DiscoveredPeer {
		peers := commands.Peers{
			UserCert: n.PeerUserCert(p, user),
			UserKey:  n.PeerUserKey(p, user),
			MSPID:    n.Organization(p.Organization).MSPID,
			Server:   n.PeerAddress(p, nwo.ListenPort),
			Channel:  channelName,
		}
		sess, err := n.Discover(peers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit(0))

		discovered := []DiscoveredPeer{}
		err = json.Unmarshal(sess.Out.Contents(), &discovered)
		Expect(err).NotTo(HaveOccurred())
		return discovered
	}
}

type ChaincodeEndorsers struct {
	Chaincode         string
	EndorsersByGroups map[string][]DiscoveredPeer
	Layouts           []*discovery.Layout
}

func discoverEndorsers(n *nwo.Network, command commands.Endorsers) func() []ChaincodeEndorsers {
	return func() []ChaincodeEndorsers {
		sess, err := n.Discover(command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return nil
		}

		discovered := []ChaincodeEndorsers{}
		err = json.Unmarshal(sess.Out.Contents(), &discovered)
		Expect(err).NotTo(HaveOccurred())
		return discovered
	}
}

func endorsersByGroups(discover func() []ChaincodeEndorsers) func() map[string][]DiscoveredPeer {
	return func() map[string][]DiscoveredPeer {
		discovered := discover()
		if len(discovered) == 1 {
			return discovered[0].EndorsersByGroups
		}
		return map[string][]DiscoveredPeer{}
	}
}

func peersWithChaincode(discover func() []DiscoveredPeer, ccName string) func() []DiscoveredPeer {
	return func() []DiscoveredPeer {
		peers := []DiscoveredPeer{}
		for _, p := range discover() {
			for _, cc := range p.Chaincodes {
				if cc == ccName {
					peers = append(peers, p)
				}
			}
		}
		return peers
	}
}

func toDiscoveredPeer(n *nwo.Network, p *nwo.Peer, chaincodes ...string) DiscoveredPeer {
	peerCert, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())

	return DiscoveredPeer{
		MSPID:      n.Organization(p.Organization).MSPID,
		Endpoint:   fmt.Sprintf("127.0.0.1:%d", n.PeerPort(p, nwo.ListenPort)),
		Identity:   string(peerCert),
		Chaincodes: chaincodes,
	}
}

func unmarshalFabricMSPConfig(c *pm.MSPConfig) *pm.FabricMSPConfig {
	fabricConfig := &pm.FabricMSPConfig{}
	err := proto.Unmarshal(c.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())
	return fabricConfig
}
