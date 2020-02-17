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

	"github.com/hyperledger/fabric/integration/chaincode/kvexecutor"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/tedsuo/ifrit"
)

var _ bool = Describe("PrivateData implicit collection", func() {
	var (
		network       *nwo.Network
		process       ifrit.Process
		orderer       *nwo.Orderer
		testChaincode chaincode
	)

	BeforeEach(func() {
		By("setting up the network")
		network = initThreeOrgsSetup(false)

		By("setting the pull retry threshold to 0 on all peers")
		// set pull retry threshold to 0 to disable pulling
		for _, p := range network.Peers {
			core := network.ReadPeerConfig(p)
			core.Peer.Gossip.PvtData.PullRetryThreshold = 0
			network.WritePeerConfig(p, core)
		}

		By("starting the network")
		process, orderer = startNetwork(network)

		By("deploying new lifecycle chaincode")
		testChaincode = chaincode{
			Chaincode: nwo.Chaincode{
				Name:        "kvexecutor",
				Version:     "1.0",
				Path:        components.Build("github.com/hyperledger/fabric/integration/chaincode/kvexecutor/cmd"),
				Lang:        "binary",
				PackageFile: filepath.Join(network.RootDir, "kvexcutor.tar.gz"),
				Label:       "kvexcutor",
				Sequence:    "1",
			},
			isLegacy: false,
		}
		nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peers...)
		deployChaincode(network, orderer, testChaincode)
	})

	AfterEach(func() {
		testCleanup(network, process)
	})

	It("disseminates implicit collection data owned by the peer's org ", func() {
		peer1 := network.Peer("Org1", "peer0")
		peer2 := network.Peer("Org2", "peer0")

		By("writing private data to org1's and org2's implicit collections")
		writeInput := []kvexecutor.KVData{
			{Collection: "_implicit_org_Org1MSP", Key: "org1_key1", Value: "org1_value1"},
			{Collection: "_implicit_org_Org2MSP", Key: "org2_key1", Value: "org2_value1"},
		}
		writeImplicitCollection(network, orderer, testChaincode.Name, writeInput, peer1, peer2)

		By("querying org1.peer0 for _implicit_org_Org1MSP collection data, expecting pvtdata")
		readInput1 := []kvexecutor.KVData{{Collection: "_implicit_org_Org1MSP", Key: "org1_key1"}}
		expectedMsg1, err := json.Marshal(writeInput[:1])
		Expect(err).NotTo(HaveOccurred())
		readImplicitCollection(network, network.Peer("Org1", "peer0"), testChaincode.Name, readInput1, string(expectedMsg1), true)

		By("querying org1.peer1 for _implicit_org_Org1MSP collection data, expecting pvtdata")
		readImplicitCollection(network, network.Peer("Org1", "peer1"), testChaincode.Name, readInput1, string(expectedMsg1), true)

		By("querying org2.peer0 for _implicit_org_Org1MSP collection data, expecting error")
		readImplicitCollection(network, network.Peer("Org2", "peer0"), testChaincode.Name, readInput1,
			"private data matching public hash version is not available", false)

		By("querying org2.peer1 for _implicit_org_Org1MSP collection data, expecting error")
		readImplicitCollection(network, network.Peer("Org2", "peer1"), testChaincode.Name, readInput1,
			"private data matching public hash version is not available", false)

		By("querying org2.peer0 for _implicit_org_Org2MSP collection data, expecting pvtdata")
		readInput2 := []kvexecutor.KVData{{Collection: "_implicit_org_Org2MSP", Key: "org2_key1"}}
		expectedMsg2, err := json.Marshal(writeInput[1:])
		Expect(err).NotTo(HaveOccurred())
		readImplicitCollection(network, network.Peer("Org2", "peer0"), testChaincode.Name, readInput2, string(expectedMsg2), true)

		By("querying org2.peer1 for _implicit_org_Org2MSP collection data, expecting pvtdata")
		readImplicitCollection(network, network.Peer("Org2", "peer1"), testChaincode.Name, readInput2, string(expectedMsg2), true)

		By("querying org1.peer0 for _implicit_org_Org2MSP collection data, expecting error")
		readImplicitCollection(network, network.Peer("Org1", "peer0"), testChaincode.Name, readInput2,
			"private data matching public hash version is not available", false)

		By("querying org1.peer1 for _implicit_org_Org2MSP collection data, expecting error")
		readImplicitCollection(network, network.Peer("Org1", "peer1"), testChaincode.Name, readInput2,
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
