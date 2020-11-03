/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/discovery"
	pm "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("DiscoveryService", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		processes []ifrit.Process
		orderer   *nwo.Orderer
		org1Peer0 *nwo.Peer
		org2Peer0 *nwo.Peer
		org3Peer0 *nwo.Peer
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-sd")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.BasicSolo()
		config.RemovePeer("Org1", "peer1")
		config.RemovePeer("Org2", "peer1")
		Expect(config.Peers).To(HaveLen(2))

		// add org3 with one peer (to generate cryptogen and configtx files)
		config.Organizations = append(config.Organizations, &nwo.Organization{
			Name:          "Org3",
			MSPID:         "Org3MSP",
			Domain:        "org3.example.com",
			EnableNodeOUs: true,
			Users:         2,
			CA:            &nwo.CA{Hostname: "ca"},
		})
		config.Consortiums[0].Organizations = append(config.Consortiums[0].Organizations, "Org3")
		config.Profiles[1].Organizations = append(config.Profiles[1].Organizations, "Org3")
		config.Peers = append(config.Peers,
			&nwo.Peer{
				Name:         "peer0",
				Organization: "Org3",
				Channels: []*nwo.PeerChannel{
					{Name: "testchannel", Anchor: true},
				},
			},
		)

		network = nwo.New(config, testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		// initially remove Org3 peer0 from the network and later add it back to test joinbysnapshot
		peers := []*nwo.Peer{}
		for _, p := range network.Peers {
			if p.Organization != "Org3" {
				peers = append(peers, p)
			}
		}
		network.Peers = peers

		networkRunner := network.NetworkGroupRunner()
		process := ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		processes = append(processes, process)

		orderer = network.Orderer("orderer")
		network.CreateAndJoinChannel(orderer, "testchannel")

		org1Peer0 = network.Peer("Org1", "peer0")
		org2Peer0 = network.Peer("Org2", "peer0")
	})

	AfterEach(func() {
		for _, process := range processes {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("discovers network configuration even without anchor peers present", func() {
		chaincodeWhenNoAnchorPeers := nwo.Chaincode{
			Name:    "noanchorpeersjustyet",
			Version: "1.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member')`,
		}
		By("Deploying chaincode before anchor peers are defined in the channel")
		nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincodeWhenNoAnchorPeers, org1Peer0)

		endorsersForChaincodeBeforeAnchorPeersExist := commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: chaincodeWhenNoAnchorPeers.Name,
		}
		discoveryQuery := discoverEndorsers(network, endorsersForChaincodeBeforeAnchorPeersExist)
		Eventually(discoveryQuery, network.EventuallyTimeout).Should(BeEquivalentTo(
			[]ChaincodeEndorsers{
				{
					Chaincode: chaincodeWhenNoAnchorPeers.Name,
					EndorsersByGroups: map[string][]nwo.DiscoveredPeer{
						"G0": {network.DiscoveredPeer(org1Peer0)},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{"G0": 1},
						},
					},
				},
			},
		))
	})

	It("discovers network configuration, endorsers, and peer membership", func() {
		By("Updating anchor peers")
		network.UpdateChannelAnchors(orderer, "testchannel")

		//
		// bootstrapping a peer from snapshot
		//

		By("generating a snapshot at current block number on org1Peer0")
		blockNum := nwo.GetLedgerHeight(network, org1Peer0, "testchannel") - 1
		submitSnapshotRequest(network, "testchannel", 0, org1Peer0, "Snapshot request submitted successfully")

		By("verifying snapshot completed on org1Peer0")
		verifyNoPendingSnapshotRequest(network, org1Peer0, "testchannel")
		snapshotDir := filepath.Join(network.PeerDir(org1Peer0), "filesystem", "snapshots", "completed", "testchannel", strconv.Itoa(blockNum))

		By("adding peer org3Peer0 to the network")
		org3Peer0 = &nwo.Peer{
			Name:         "peer0",
			Organization: "Org3",
			Channels: []*nwo.PeerChannel{
				{Name: "testchannel", Anchor: true},
			},
		}
		network.Peers = append(network.Peers, org3Peer0)

		By("starting peer org3Peer0")
		peerRunner := network.PeerRunner(org3Peer0)
		process := ifrit.Invoke(peerRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		processes = append(processes, process)

		By("joining peer org3Peer0 to channel by a snapshot")
		joinBySnapshot(network, orderer, org3Peer0, "testchannel", snapshotDir)

		//
		// retrieving configuration and validating membership
		//

		By("retrieving the configuration from org1Peer0")
		discoveredConfig := discoverConfiguration(network, org1Peer0)

		By("retrieving the configuration from org3Peer0")
		discoveredConfig2 := discoverConfiguration(network, org1Peer0)

		By("comparing configuration from org1Peer0 and org3Peer0")
		Expect(proto.Equal(discoveredConfig, discoveredConfig2)).To(BeTrue())

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

		By("validating the orderers")
		Expect(discoveredConfig.Orderers).To(HaveLen(len(network.Orderers)))
		for _, orderer := range network.Orderers {
			ordererMSPID := network.Organization(orderer.Organization).MSPID
			Expect(discoveredConfig.Orderers[ordererMSPID].Endpoint).To(ConsistOf(
				&discovery.Endpoint{Host: "127.0.0.1", Port: uint32(network.OrdererPort(orderer, nwo.ListenPort))},
			))
		}

		//
		// discovering peers and endorsers
		//

		By("discovering peers before deploying any user chaincodes")
		Eventually(nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "_lifecycle"),
			network.DiscoveredPeer(org2Peer0, "_lifecycle"),
			network.DiscoveredPeer(org3Peer0, "_lifecycle"),
		))

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
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("installing and instantiating chaincode on org1.peer0")
		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "1.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR (AND ('Org1MSP.member','Org2MSP.member'), AND ('Org1MSP.member','Org3MSP.member'), AND ('Org2MSP.member','Org3MSP.member'))`,
		}
		nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode, org1Peer0)

		By("discovering peers after installing and instantiating chaincode using org1's peer")
		dp := nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc"), network.EventuallyTimeout).Should(HaveLen(1))
		peersWithCC := peersWithChaincode(dp, "mycc")()
		Expect(peersWithCC).To(ConsistOf(network.DiscoveredPeer(org1Peer0, "_lifecycle", "mycc")))

		By("discovering endorsers for chaincode that has not been installed to enough orgs to satisfy endorsement policy")
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc"`))

		By("installing chaincode to enough organizations to satisfy the endorsement policy")
		nwo.InstallChaincodeLegacy(network, chaincode, org2Peer0)

		By("discovering peers after installing chaincode to org2's peer")
		dp = nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc"), network.EventuallyTimeout).Should(HaveLen(2))
		peersWithCC = peersWithChaincode(dp, "mycc")()
		Expect(peersWithCC).To(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "_lifecycle", "mycc"),
			network.DiscoveredPeer(org2Peer0, "_lifecycle", "mycc"),
		))

		By("discovering endorsers for chaincode that has been installed to org1 and org2")
		de := discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))
		discovered := de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("installing chaincode to all orgs")
		nwo.InstallChaincodeLegacy(network, chaincode, org3Peer0)

		By("discovering peers after installing and instantiating chaincode all org peers")
		dp = nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc"), network.EventuallyTimeout).Should(HaveLen(3))
		peersWithCC = peersWithChaincode(dp, "mycc")()
		Expect(peersWithCC).To(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "_lifecycle", "mycc"),
			network.DiscoveredPeer(org2Peer0, "_lifecycle", "mycc"),
			network.DiscoveredPeer(org3Peer0, "_lifecycle", "mycc"),
		))

		By("discovering endorsers for chaincode that has been installed to all orgs")
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(3))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))
		Expect(discovered[0].Layouts[1].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))
		Expect(discovered[0].Layouts[2].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("upgrading chaincode and adding a collections config")
		chaincode.Name = "mycc"
		chaincode.Version = "2.0"
		chaincode.CollectionsConfig = filepath.Join("testdata", "collections_config_org1_org2.json")
		nwo.UpgradeChaincodeLegacy(network, "testchannel", orderer, chaincode, org1Peer0, org2Peer0, org3Peer0)

		By("discovering endorsers for chaincode with a private collection")
		endorsers.Collection = "mycc:collectionMarbles"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		endorsers.Collection = "mycc:collectionMarbles"
		endorsers.NoPrivateReads = []string{"mycc"}
		de = discoverEndorsers(network, endorsers)
		By("discovering endorsers for a blind write with a collection consists of all possible peers")
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))

		By("changing the channel policy")
		currentConfig := nwo.GetConfig(network, org3Peer0, orderer, "testchannel")
		updatedConfig := proto.Clone(currentConfig).(*common.Config)
		updatedConfig.ChannelGroup.Groups["Application"].Groups["Org3"].Policies["Writers"].Policy.Value = protoutil.MarshalOrPanic(policydsl.SignedByMspAdmin("Org3MSP"))
		nwo.UpdateConfig(network, orderer, "testchannel", currentConfig, updatedConfig, true, org3Peer0)

		By("trying to discover endorsers as an org3 admin")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org3Peer0, "Admin"),
			UserKey:   network.PeerUserKey(org3Peer0, "Admin"),
			MSPID:     network.Organization(org3Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org3Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc",
		}
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			ConsistOf(network.DiscoveredPeer(org1Peer0)),
			ConsistOf(network.DiscoveredPeer(org2Peer0)),
			ConsistOf(network.DiscoveredPeer(org3Peer0)),
		))

		By("trying to discover endorsers as an org3 member")
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
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`access denied`))

		By("discovering endorsers for _lifecycle system chaincode before enabling V2_0 capabilities")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "_lifecycle",
		}
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"_lifecycle"`))

		//
		// using _lifecycle
		//

		By("enabling V2_0 application capabilities on the channel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer0, org2Peer0, org3Peer0)

		By("ensuring peers are still discoverable after enabling V2_0 application capabilities")
		dp = nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc"), network.EventuallyTimeout).Should(HaveLen(3))
		peersWithCC = peersWithChaincode(dp, "mycc")()
		Expect(peersWithCC).To(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "_lifecycle", "mycc"),
			network.DiscoveredPeer(org2Peer0, "_lifecycle", "mycc"),
			network.DiscoveredPeer(org3Peer0, "_lifecycle", "mycc"),
		))

		By("ensuring endorsers for mycc are still discoverable after upgrading to V2_0 application capabilities")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc",
		}
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))

		By("ensuring endorsers for mycc's collection are still discoverable after upgrading to V2_0 application capabilities")
		endorsers.Collection = "mycc:collectionMarbles"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))

		By("discovering endorsers for _lifecycle system chaincode")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "_lifecycle",
		}
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(3))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))
		Expect(discovered[0].Layouts[1].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))
		Expect(discovered[0].Layouts[2].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("discovering endorsers when missing chaincode")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org1Peer0, "User1"),
			UserKey:   network.PeerUserKey(org1Peer0, "User1"),
			MSPID:     network.Organization(org1Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org1Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc-lifecycle",
		}
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`failed constructing descriptor for chaincodes:<name:"mycc-lifecycle"`))

		By("deploying chaincode using org1 and org2")
		chaincodePath := components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		chaincode = nwo.Chaincode{
			Name:                "mycc-lifecycle",
			Version:             "1.0",
			Lang:                "binary",
			PackageFile:         filepath.Join(testDir, "simplecc.tar.gz"),
			Path:                chaincodePath,
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_prebuilt_chaincode",
		}

		By("packaging chaincode")
		nwo.PackageChaincodeBinary(chaincode)

		By("installing chaincode to org1.peer0 and org2.peer0")
		nwo.InstallChaincode(network, chaincode, org1Peer0, org2Peer0)

		By("approving chaincode definition for org1 and org2")
		for _, org := range []string{"Org1", "Org2"} {
			nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.PeersInOrg(org)...)
		}

		By("committing chaincode definition using org1.peer0 and org2.peer0")
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, org1Peer0, org1Peer0, org2Peer0)
		nwo.InitChaincode(network, "testchannel", orderer, chaincode, org1Peer0, org2Peer0)

		By("discovering endorsers for chaincode that has been installed to some orgs")
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("installing chaincode to all orgs")
		nwo.InstallChaincode(network, chaincode, org3Peer0)

		By("discovering endorsers for chaincode that has been installed to all orgs but not yet approved by org3")
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))

		By("ensuring peers are only discoverable that have the chaincode installed and approved")
		dp = nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc-lifecycle"), network.EventuallyTimeout).Should(HaveLen(2))
		peersWithCC = peersWithChaincode(dp, "mycc-lifecycle")()
		Expect(peersWithCC).To(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "mycc-lifecycle", "_lifecycle", "mycc"),
			network.DiscoveredPeer(org2Peer0, "mycc-lifecycle", "_lifecycle", "mycc"),
		))

		By("approving chaincode definition for org3")
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.PeersInOrg("Org3")...)

		By("discovering endorsers for chaincode that has been installed and approved by all orgs")
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))

		By("discovering peers for chaincode that has been installed and approved by all orgs")
		dp = nwo.DiscoverPeers(network, org1Peer0, "User1", "testchannel")
		Eventually(peersWithChaincode(dp, "mycc-lifecycle"), network.EventuallyTimeout).Should(HaveLen(3))
		peersWithCC = peersWithChaincode(dp, "mycc-lifecycle")()
		Expect(peersWithCC).To(ConsistOf(
			network.DiscoveredPeer(org1Peer0, "mycc-lifecycle", "_lifecycle", "mycc"),
			network.DiscoveredPeer(org2Peer0, "mycc-lifecycle", "_lifecycle", "mycc"),
			network.DiscoveredPeer(org3Peer0, "mycc-lifecycle", "_lifecycle", "mycc"),
		))

		By("updating the chaincode definition to sequence 2 to add a collections config")
		chaincode.Sequence = "2"
		chaincode.CollectionsConfig = filepath.Join("testdata", "collections_config_org1_org2.json")
		for _, org := range []string{"Org1", "Org2"} {
			nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.PeersInOrg(org)...)
		}

		By("committing the new chaincode definition using org1 and org2")
		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, []*nwo.Organization{network.Organization("Org1"), network.Organization("Org2")}, org1Peer0, org2Peer0, org3Peer0)
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, org1Peer0, org1Peer0, org2Peer0)

		By("discovering endorsers for sequence 2 that has only been approved by org1 and org2")
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))

		By("approving the chaincode definition at sequence 2 by org3")
		maxLedgerHeight := nwo.GetMaxLedgerHeight(network, "testchannel", org1Peer0, org2Peer0, org3Peer0)
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.PeersInOrg("Org3")...)
		nwo.WaitUntilEqualLedgerHeight(network, "testchannel", maxLedgerHeight+1, org1Peer0, org2Peer0, org3Peer0)

		By("discovering endorsers for sequence 2 that has been approved by all orgs")
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org3Peer0)},
		))

		By("discovering endorsers for chaincode with a private collection")
		endorsers.Collection = "mycc-lifecycle:collectionMarbles"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org1Peer0)},
			[]nwo.DiscoveredPeer{network.DiscoveredPeer(org2Peer0)},
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("upgrading a legacy chaincode for all peers")
		nwo.DeployChaincode(network, "testchannel", orderer, nwo.Chaincode{
			Name:              "mycc",
			Version:           "2.0",
			Lang:              "binary",
			PackageFile:       filepath.Join(testDir, "simplecc.tar.gz"),
			Path:              chaincodePath,
			SignaturePolicy:   `AND ('Org1MSP.member', 'Org2MSP.member', 'Org3MSP.member')`,
			Sequence:          "1",
			CollectionsConfig: filepath.Join("testdata", "collections_config_org1_org2_org3.json"),
			Label:             "my_prebuilt_chaincode",
		})

		By("discovering endorsers for chaincode that has been installed to all peers")
		endorsers.Chaincode = "mycc"
		endorsers.Collection = ""
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			ConsistOf(network.DiscoveredPeer(org1Peer0)),
			ConsistOf(network.DiscoveredPeer(org2Peer0)),
			ConsistOf(network.DiscoveredPeer(org3Peer0)),
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1), uint32(1)))

		By("discovering endorsers for a collection without collection EP, using chaincode EP")
		endorsers.Collection = "mycc:collectionMarbles"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			ConsistOf(network.DiscoveredPeer(org1Peer0)),
			ConsistOf(network.DiscoveredPeer(org2Peer0)),
			ConsistOf(network.DiscoveredPeer(org3Peer0)),
		))

		By("discovering endorsers for a collection with collection EP, using collection EP")
		endorsers.Collection = "mycc:collectionDetails"
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			ConsistOf(network.DiscoveredPeer(org1Peer0)),
			ConsistOf(network.DiscoveredPeer(org2Peer0)),
		))
		discovered = de()
		Expect(discovered).To(HaveLen(1))
		Expect(discovered[0].Layouts).To(HaveLen(1))
		Expect(discovered[0].Layouts[0].QuantitiesByGroup).To(ConsistOf(uint32(1), uint32(1)))

		By("trying to discover endorsers as an org3 admin")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org3Peer0, "Admin"),
			UserKey:   network.PeerUserKey(org3Peer0, "Admin"),
			MSPID:     network.Organization(org3Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org3Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc",
		}
		de = discoverEndorsers(network, endorsers)
		Eventually(endorsersByGroups(de), network.EventuallyTimeout).Should(ConsistOf(
			ConsistOf(network.DiscoveredPeer(org1Peer0)),
			ConsistOf(network.DiscoveredPeer(org2Peer0)),
			ConsistOf(network.DiscoveredPeer(org3Peer0)),
		))

		By("trying to discover endorsers as an org3 member")
		endorsers = commands.Endorsers{
			UserCert:  network.PeerUserCert(org3Peer0, "User1"),
			UserKey:   network.PeerUserKey(org3Peer0, "User1"),
			MSPID:     network.Organization(org3Peer0.Organization).MSPID,
			Server:    network.PeerAddress(org3Peer0, nwo.ListenPort),
			Channel:   "testchannel",
			Chaincode: "mycc-lifecycle",
		}
		sess, err = network.Discover(endorsers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`access denied`))
	})
})

type ChaincodeEndorsers struct {
	Chaincode         string
	EndorsersByGroups map[string][]nwo.DiscoveredPeer
	Layouts           []*discovery.Layout
}

func discoverEndorsers(n *nwo.Network, command commands.Endorsers) func() []ChaincodeEndorsers {
	return func() []ChaincodeEndorsers {
		sess, err := n.Discover(command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return nil
		}

		discovered := []ChaincodeEndorsers{}
		err = json.Unmarshal(sess.Out.Contents(), &discovered)
		Expect(err).NotTo(HaveOccurred())
		return discovered
	}
}

func endorsersByGroups(discover func() []ChaincodeEndorsers) func() map[string][]nwo.DiscoveredPeer {
	return func() map[string][]nwo.DiscoveredPeer {
		discovered := discover()
		if len(discovered) == 1 {
			return discovered[0].EndorsersByGroups
		}
		return map[string][]nwo.DiscoveredPeer{}
	}
}

func peersWithChaincode(discover func() []nwo.DiscoveredPeer, ccName string) func() []nwo.DiscoveredPeer {
	return func() []nwo.DiscoveredPeer {
		peers := []nwo.DiscoveredPeer{}
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

func unmarshalFabricMSPConfig(c *pm.MSPConfig) *pm.FabricMSPConfig {
	fabricConfig := &pm.FabricMSPConfig{}
	err := proto.Unmarshal(c.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())
	return fabricConfig
}

func discoverConfiguration(n *nwo.Network, peer *nwo.Peer) *discovery.ConfigResult {
	config := commands.Config{
		UserCert: n.PeerUserCert(peer, "User1"),
		UserKey:  n.PeerUserKey(peer, "User1"),
		MSPID:    n.Organization(peer.Organization).MSPID,
		Server:   n.PeerAddress(peer, nwo.ListenPort),
		Channel:  "testchannel",
	}
	sess, err := n.Discover(config)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	By("unmarshaling the response")
	discoveredConfig := &discovery.ConfigResult{}
	err = json.Unmarshal(sess.Out.Contents(), &discoveredConfig)
	Expect(err).NotTo(HaveOccurred())

	return discoveredConfig
}

func submitSnapshotRequest(n *nwo.Network, channel string, blockNum int, peer *nwo.Peer, expectedMsg string) {
	sess, err := n.PeerAdminSession(peer, commands.SnapshotSubmitRequest{
		ChannelID:   channel,
		BlockNumber: strconv.Itoa(blockNum),
		ClientAuth:  n.ClientAuthRequired,
		PeerAddress: n.PeerAddress(peer, nwo.ListenPort),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(expectedMsg))
}

func verifyNoPendingSnapshotRequest(n *nwo.Network, peer *nwo.Peer, channelID string) {
	cmd := commands.SnapshotListPending{
		ChannelID:   channelID,
		ClientAuth:  n.ClientAuthRequired,
		PeerAddress: n.PeerAddress(peer, nwo.ListenPort),
	}
	checkPending := func() []byte {
		sess, err := n.PeerAdminSession(peer, cmd)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		return sess.Buffer().Contents()
	}
	Eventually(checkPending, n.EventuallyTimeout, 10*time.Second).Should(ContainSubstring("Successfully got pending snapshot requests: []\n"))
}

func joinBySnapshot(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channelID string, snapshotDir string) {
	n.JoinChannelBySnapshot(snapshotDir, peer)

	By("calling JoinBySnapshotStatus until joinbysnapshot is completed")
	checkStatus := func() []byte { return n.JoinBySnapshotStatus(peer) }
	Eventually(checkStatus, n.EventuallyTimeout, 10*time.Second).Should(ContainSubstring("No joinbysnapshot operation is in progress"))

	By("waiting for the peer to have the same ledger height")
	channelHeight := nwo.GetMaxLedgerHeight(n, channelID, n.PeersWithChannel(channelID)...)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, channelHeight, peer)
}
