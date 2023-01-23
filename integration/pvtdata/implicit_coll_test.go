/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdata

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric/integration/chaincode/kvexecutor"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ bool = Describe("Pvtdata dissemination for implicit collection", func() {
	var (
		network                     *nwo.Network
		ordererProcess, peerProcess ifrit.Process
		orderer                     *nwo.Orderer
		testChaincode               chaincode
	)

	BeforeEach(func() {
		By("setting up the network")
		network = initThreeOrgsSetup(false)

		By("disabling pvtdata pull/dissemination/reconciliation on all peers except for enabling dissemination on org1 peers")
		for _, p := range network.Peers {
			core := network.ReadPeerConfig(p)
			core.Peer.Gossip.PvtData.PullRetryThreshold = 0
			core.Peer.Gossip.PvtData.ReconciliationEnabled = false
			// set timeout and reconnect interval to prevent test flake
			core.Peer.Gossip.DialTimeout = 5 * time.Second
			core.Peer.Gossip.ConnTimeout = 5 * time.Second
			core.Peer.Gossip.ReconnectInterval = 7 * time.Second
			if p.Organization == "Org1" {
				// enable dissemination on org1 peers
				core.Peer.Gossip.PvtData.ImplicitCollDisseminationPolicy.RequiredPeerCount = 1
				core.Peer.Gossip.PvtData.ImplicitCollDisseminationPolicy.MaxPeerCount = 3
				core.Peer.Gossip.PvtData.PushAckTimeout = 10 * time.Second
			} else {
				// disable dissemination on non-org1 peers
				core.Peer.Gossip.PvtData.ImplicitCollDisseminationPolicy.RequiredPeerCount = 0
				core.Peer.Gossip.PvtData.ImplicitCollDisseminationPolicy.MaxPeerCount = 0
			}
			network.WritePeerConfig(p, core)
		}

		By("starting the network")
		ordererProcess, peerProcess, orderer = startNetwork(network)

		By("deploying new lifecycle chaincode")
		testChaincode = chaincode{
			Chaincode: nwo.Chaincode{
				Name:        "kvexecutor",
				Version:     "1.0",
				Path:        components.Build("github.com/hyperledger/fabric/integration/chaincode/kvexecutor/cmd"),
				Lang:        "binary",
				PackageFile: filepath.Join(network.RootDir, "kvexecutor.tar.gz"),
				Label:       "kvexecutor",
				Sequence:    "1",
			},
			isLegacy: false,
		}
		nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)

		By("verifying org1.peer0 discovers all peers to ensure that gossip has been initialized")
		discoverAllPeers(network, network.Peer("Org1", "peer0"), channelID, 3, 5*time.Second)

		By("verifying org1.peer1 discovers all peers to ensure that gossip has been initialized")
		discoverAllPeers(network, network.Peer("Org1", "peer1"), channelID, 3, 5*time.Second)

		By("deploying chaincode to all peers")
		deployChaincode(network, orderer, testChaincode)
	})

	AfterEach(func() {
		testCleanup(network, ordererProcess, peerProcess)
	})

	It("disseminates pvtdata of implicit collection for the peer's own org but not implicit collection for another org", func() {
		org1peer0 := network.Peer("Org1", "peer0")
		org1peer1 := network.Peer("Org1", "peer1")
		org2peer0 := network.Peer("Org2", "peer0")
		org2peer1 := network.Peer("Org2", "peer1")

		By("writing private data to org1's and org2's implicit collections")
		writeInput := []kvexecutor.KVData{
			{Collection: "_implicit_org_Org1MSP", Key: "org1_key1", Value: "org1_value1"},
			{Collection: "_implicit_org_Org2MSP", Key: "org2_key1", Value: "org2_value1"},
		}
		writeImplicitCollection(network, orderer, testChaincode.Name, writeInput, org1peer0, org2peer0)

		By("querying org1.peer0 for _implicit_org_Org1MSP collection data, expecting pvtdata")
		readInput1 := []kvexecutor.KVData{{Collection: "_implicit_org_Org1MSP", Key: "org1_key1"}}
		expectedMsg1, err := json.Marshal(writeInput[:1])
		Expect(err).NotTo(HaveOccurred())
		readImplicitCollection(network, org1peer0, testChaincode.Name, readInput1, string(expectedMsg1), true)

		// org1.peer1 should have _implicit_org_Org1MSP pvtdata because dissemination is enabled on org1 peers
		By("querying org1.peer1 for _implicit_org_Org1MSP collection data, expecting pvtdata")
		readImplicitCollection(network, org1peer1, testChaincode.Name, readInput1, string(expectedMsg1), true)

		By("querying org2.peer0 for _implicit_org_Org1MSP collection data, expecting error")
		readImplicitCollection(network, org2peer0, testChaincode.Name, readInput1,
			"private data matching public hash version is not available", false)

		By("querying org2.peer1 for _implicit_org_Org1MSP collection data, expecting error")
		readImplicitCollection(network, org2peer1, testChaincode.Name, readInput1,
			"private data matching public hash version is not available", false)

		By("querying org2.peer0 for _implicit_org_Org2MSP collection data, expecting pvtdata")
		readInput2 := []kvexecutor.KVData{{Collection: "_implicit_org_Org2MSP", Key: "org2_key1"}}
		expectedMsg2, err := json.Marshal(writeInput[1:])
		Expect(err).NotTo(HaveOccurred())
		readImplicitCollection(network, org2peer0, testChaincode.Name, readInput2, string(expectedMsg2), true)

		// org2.peer1 should have no _implicit_org_Org2MSP pvtdata because pull/dissemination/reconciliation are disabled on org2 peers
		By("querying org2.peer1 for _implicit_org_Org2MSP collection data, expecting error")
		readImplicitCollection(network, org2peer1, testChaincode.Name, readInput2,
			"private data matching public hash version is not available", false)

		By("querying org1.peer0 for _implicit_org_Org2MSP collection data, expecting error")
		readImplicitCollection(network, org1peer0, testChaincode.Name, readInput2,
			"private data matching public hash version is not available", false)

		By("querying org1.peer1 for _implicit_org_Org2MSP collection data, expecting error")
		readImplicitCollection(network, org1peer1, testChaincode.Name, readInput2,
			"private data matching public hash version is not available", false)
	})
})

func writeImplicitCollection(n *nwo.Network, orderer *nwo.Orderer, chaincodeName string, writeInput []kvexecutor.KVData, peers ...*nwo.Peer) {
	writeInputBytes, err := json.Marshal(writeInput)
	Expect(err).NotTo(HaveOccurred())
	writeInputBase64 := base64.StdEncoding.EncodeToString(writeInputBytes)

	peerAddresses := make([]string, 0)
	for _, peer := range peers {
		peerAddresses = append(peerAddresses, n.PeerAddress(peer, nwo.ListenPort))
	}
	command := commands.ChaincodeInvoke{
		ChannelID:     channelID,
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		Name:          chaincodeName,
		Ctor:          fmt.Sprintf(`{"Args":["readWriteKVs","%s","%s"]}`, "", writeInputBase64),
		PeerAddresses: peerAddresses,
		WaitForEvent:  true,
	}
	invokeChaincode(n, peers[0], command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peers[0], channelID), n.Peers...)
}

func readImplicitCollection(n *nwo.Network, peer *nwo.Peer, chaincodeName string, readInput []kvexecutor.KVData, expectedMsg string, expectSuccess bool) {
	readInputBytes, err := json.Marshal(readInput)
	Expect(err).NotTo(HaveOccurred())
	readInputBase64 := base64.StdEncoding.EncodeToString(readInputBytes)

	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readWriteKVs","%s","%s"]}`, readInputBase64, ""),
	}
	queryChaincode(n, peer, command, expectedMsg, expectSuccess)
}

func invokeChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func queryChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeQuery, expectedMessage string, expectSuccess bool) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMessage))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedMessage))
	}
}

func discoverAllPeers(n *nwo.Network, peer *nwo.Peer, channelID string, retries int, retryInterval time.Duration) {
	var discoveredPeers []nwo.DiscoveredPeer
	numPeers := len(n.Peers)
	for i := 0; i < retries; i++ {
		discoveredPeers = nwo.DiscoverPeers(n, peer, "User1", channelID)()
		if len(discoveredPeers) == numPeers || i == retries-1 {
			break
		}
		time.Sleep(retryInterval)
	}
	Expect(discoveredPeers).To(HaveLen(numPeers))
}
