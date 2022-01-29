/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("all shim APIs for non-private data", func() {
	var (
		setup     *setup
		helper    *marblesTestHelper
		chaincode nwo.Chaincode
	)

	BeforeEach(func() {
		setup = initThreeOrgsSetup()
		helper = &marblesTestHelper{
			networkHelper: &networkHelper{
				Network:   setup.network,
				orderer:   setup.orderer,
				peers:     setup.peers,
				testDir:   setup.testDir,
				channelID: setup.channelID,
			},
		}

		chaincode = nwo.Chaincode{
			Name:            "marbles",
			Version:         "0.0",
			Path:            "github.com/hyperledger/fabric/integration/chaincode/marbles/cmdwithindexspecs",
			Lang:            "golang",
			PackageFile:     filepath.Join(setup.testDir, "marbles.tar.gz"),
			Label:           "marbles",
			SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			Sequence:        "1",
		}
	})

	AfterEach(func() {
		setup.cleanup()
	})

	// assertMarbleAPIs invoke marble APIs to add/transfer/delete marbles and get marbles without rich queries.
	// These APIs are applicable to both levelDB and CouchDB.
	assertMarbleAPIs := func(ccName string, peer *nwo.Peer) {
		height := helper.getLedgerHeight(peer)

		By("adding six marbles, marble-0 to marble-5")
		for i := 0; i <= 5; i++ {
			helper.invokeMarblesChaincode(ccName, peer, "initMarble", fmt.Sprintf("marble-%d", i), "blue", "35", "tom")
			helper.waitUntilAllPeersEqualLedgerHeight(height + i + 1)
		}

		By("getting marbles by range")
		expectedQueryResult := newMarbleQueryResult(1, 4, "blue", 35, "tom")
		helper.assertQueryMarbles(ccName, peer, expectedQueryResult, "getMarblesByRange", "marble-1", "marble-5")

		By("transferring marble-0 to jerry")
		helper.invokeMarblesChaincode(ccName, peer, "transferMarble", "marble-0", "jerry")

		By("verifying new owner of marble-0 after transfer")
		expectedResult := newMarble("marble-0", "blue", 35, "jerry")
		helper.assertMarbleExists(ccName, peer, expectedResult, "marble-0")

		By("deleting marble-0")
		helper.invokeMarblesChaincode(ccName, peer, "delete", "marble-0")

		By("verifying deletion of marble-0")
		helper.assertMarbleDoesNotExist(peer, ccName, "marble-0")

		By("transferring marbles by color")
		helper.invokeMarblesChaincode(ccName, peer, "transferMarblesBasedOnColor", "blue", "jerry")

		By("verifying new owner after transfer by color")
		for i := 1; i <= 5; i++ {
			name := fmt.Sprintf("marble-%d", i)
			expectedResult = newMarble(name, "blue", 35, "jerry")
			helper.assertMarbleExists(ccName, peer, expectedResult, name)
		}

		By("getting history for marble-0")
		expectedHistoryResult := []*marbleHistoryResult{
			{IsDelete: "true"},
			{IsDelete: "false", Value: newMarble("marble-0", "blue", 35, "jerry")},
			{IsDelete: "false", Value: newMarble("marble-0", "blue", 35, "tom")},
		}
		helper.assertGetHistoryForMarble(ccName, peer, expectedHistoryResult, "marble-0")
	}

	// assertMarbleAPIs verifies marbles APIs using rich queries and pagination that are only applicable to CouchDB.
	assertMarbleAPIsRichQueries := func(ccName string, peer *nwo.Peer) {
		By("querying marbles by owner")
		expectedQueryResult := newMarbleQueryResult(1, 5, "blue", 35, "jerry")
		helper.assertQueryMarbles(ccName, peer, expectedQueryResult, "queryMarblesByOwner", "jerry")

		By("quering marbles by search criteria")
		helper.assertQueryMarbles(ccName, peer, expectedQueryResult, "queryMarbles", `{"selector":{"color":"blue"}}`)

		By("quering marbles by range with pagination size 3, 1st call")
		bookmark := ""
		expectedQueryResult = newMarbleQueryResult(1, 3, "blue", 35, "jerry")
		bookmark = helper.assertQueryMarblesWithPagination(ccName, peer, expectedQueryResult,
			"getMarblesByRangeWithPagination", "marble-1", "marble-6", "3", bookmark)

		By("quering marbles by range with pagination size 3, 2nd call")
		expectedQueryResult = newMarbleQueryResult(4, 5, "blue", 35, "jerry")
		bookmark = helper.assertQueryMarblesWithPagination(ccName, peer, expectedQueryResult,
			"getMarblesByRangeWithPagination", "marble-1", "marble-6", "3", bookmark)

		By("quering marbles by range with pagination size 3, 3rd call should return no marble")
		expectedQueryResult = make([]*marbleQueryResult, 0)
		helper.assertQueryMarblesWithPagination(ccName, peer, expectedQueryResult,
			"getMarblesByRangeWithPagination", "marble-1", "marble-6", "3", bookmark)

		By("quering marbles by search criteria with pagination size 10, 1st call")
		bookmark = ""
		expectedQueryResult = newMarbleQueryResult(1, 5, "blue", 35, "jerry")
		bookmark = helper.assertQueryMarblesWithPagination(ccName, peer, expectedQueryResult,
			"queryMarblesWithPagination", `{"selector":{"owner":"jerry"}}`, "10", bookmark)

		By("quering marbles by search criteria with pagination size 10, 2nd call should return no marble")
		expectedQueryResult = make([]*marbleQueryResult, 0)
		helper.assertQueryMarblesWithPagination(ccName, peer, expectedQueryResult,
			"queryMarblesWithPagination", `{"selector":{"owner":"jerry"}}`, "10", bookmark)
	}

	When("levelDB is used as stateDB", func() {
		It("calls marbles APIs", func() {
			peer := setup.network.Peer("Org2", "peer0")

			By("deploying new lifecycle chaincode")
			nwo.EnableCapabilities(setup.network, setup.channelID, "Application", "V2_0", setup.orderer, setup.peers...)
			helper.deployChaincode(chaincode)

			By("verifying marbles chaincode APIs")
			assertMarbleAPIs(chaincode.Name, peer)
		})
	})

	When("CouchDB is used as stateDB", func() {
		var couchProcess ifrit.Process

		BeforeEach(func() {
			By("stopping peers")
			setup.stopPeers()

			By("configuring a peer with couchdb")
			// configure only one of the peers (Org2, peer0) to use couchdb.
			// Note that we do not support a channel with mixed DBs.
			// However, for testing, it would be fine to use couchdb for one
			// peer and sending all the couchdb related test queries to this peer
			couchDB := &runner.CouchDB{}
			couchProcess = ifrit.Invoke(couchDB)
			Eventually(couchProcess.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
			Consistently(couchProcess.Wait()).ShouldNot(Receive())
			couchAddr := couchDB.Address()
			peer := setup.network.Peer("Org2", "peer0")
			core := setup.network.ReadPeerConfig(peer)
			core.Ledger.State.StateDatabase = "CouchDB"
			core.Ledger.State.CouchDBConfig.CouchDBAddress = couchAddr
			setup.network.WritePeerConfig(peer, core)

			By("restarting peers with couchDB")
			setup.startPeers()
		})

		AfterEach(func() {
			couchProcess.Signal(syscall.SIGTERM)
			Eventually(couchProcess.Wait(), setup.network.EventuallyTimeout).Should(Receive())
		})

		It("calls marbles APIs", func() {
			peer := setup.network.Peer("Org2", "peer0")

			By("deploying new lifecycle chaincode")
			nwo.EnableCapabilities(setup.network, setup.channelID, "Application", "V2_0", setup.orderer, setup.peers...)
			helper.deployChaincode(chaincode)

			By("verifying marbles chaincode APIs")
			assertMarbleAPIs(chaincode.Name, peer)

			By("verifying marbles rich queries")
			assertMarbleAPIsRichQueries(chaincode.Name, peer)
		})
	})
})

// marble is the struct to unmarshal the response bytes returned from getMarble API
type marble struct {
	ObjectType string `json:"docType"` // docType is "marble"
	Name       string `json:"name"`
	Color      string `json:"color"`
	Size       int    `json:"size"`
	Owner      string `json:"owner"`
}

// marbleQueryResult is the struct to unmarshal the response bytes returned from marbles query APIs
type marbleQueryResult struct {
	Key    string  `json:"Key"`
	Record *marble `json:"Record"`
}

type metadata struct {
	RecordsCount string `json:"RecordsCount"`
	Bookmark     string `json:"Bookmark"`
}

// marbleQueryResult is the struct to unmarshal the metadata bytes returned from marbles pagination APIs
type paginationMetadata struct {
	ResponseMetadata *metadata `json:"ResponseMetadata"`
}

// marbleHistoryResult is the struct to unmarshal the response bytes returned from marbles history API
type marbleHistoryResult struct {
	TxId      string  `json:"TxId"`
	Value     *marble `json:"Value"`
	Timestamp string  `json:"Timestamp"`
	IsDelete  string  `json:"IsDelete"`
}

// newMarble creates a marble object for the given parameters
func newMarble(name, color string, size int, owner string) *marble {
	return &marble{"marble", name, color, size, owner}
}

// newMarbleQueryResult creates a slice of marbleQueryResult for the marbles based on startIndex and endIndex.
// Both startIndex and endIndex are inclusive
func newMarbleQueryResult(startIndex, endIndex int, color string, size int, owner string) []*marbleQueryResult {
	expectedResult := make([]*marbleQueryResult, 0)
	for i := startIndex; i <= endIndex; i++ {
		name := fmt.Sprintf("marble-%d", i)
		item := marbleQueryResult{Key: name, Record: newMarble(name, color, size, owner)}
		expectedResult = append(expectedResult, &item)
	}
	return expectedResult
}

// marblesTestHelper implements helper methods to call marbles chaincode APIs and verify results
type marblesTestHelper struct {
	*networkHelper
}

// invokeMarblesChaincode invokes marbles APIs such as initMarble, transfer and delete.
func (th *marblesTestHelper) invokeMarblesChaincode(chaincodeName string, peer *nwo.Peer, funcAndArgs ...string) {
	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      prepareChaincodeInvokeArgs(funcAndArgs...),
		PeerAddresses: []string{
			th.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	th.invokeChaincode(peer, command)
	nwo.WaitUntilEqualLedgerHeight(th.Network, th.channelID, nwo.GetLedgerHeight(th.Network, peer, th.channelID), th.peers...)
}

// assertMarbleExists asserts that the marble exists and matches the expected result
func (th *marblesTestHelper) assertMarbleExists(chaincodeName string, peer *nwo.Peer, expectedResult *marble, marbleName string) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	sess, err := th.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, th.EventuallyTimeout).Should(gexec.Exit(0))
	result := &marble{}
	err = json.Unmarshal(sess.Out.Contents(), result)
	Expect(err).NotTo(HaveOccurred())
	Expect(result).To(Equal(expectedResult))
}

// assertMarbleDoesNotExist asserts that the marble does not exist
func (th *marblesTestHelper) assertMarbleDoesNotExist(peer *nwo.Peer, chaincodeName, marbleName string) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	th.queryChaincode(peer, command, "Marble does not exist", false)
}

// assertQueryMarbles queries the chaincode and verifies the result based on the function and arguments,
// including range queries and rich queries.
func (th *marblesTestHelper) assertQueryMarbles(chaincodeName string, peer *nwo.Peer, expectedResult []*marbleQueryResult, funcAndArgs ...string) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      prepareChaincodeInvokeArgs(funcAndArgs...),
	}
	sess, err := th.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, th.EventuallyTimeout).Should(gexec.Exit(0))
	results := make([]*marbleQueryResult, 0)
	err = json.Unmarshal(sess.Out.Contents(), &results)
	Expect(err).NotTo(HaveOccurred())
	Expect(results).To(Equal(expectedResult))
}

// assertQueryMarbles queries the chaincode with pagination and verifies the result based on the function and arguments,
// including range queries and rich queries.
func (th *marblesTestHelper) assertQueryMarblesWithPagination(chaincodeName string, peer *nwo.Peer, expectedResult []*marbleQueryResult, funcAndArgs ...string) string {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      prepareChaincodeInvokeArgs(funcAndArgs...),
	}
	sess, err := th.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, th.EventuallyTimeout).Should(gexec.Exit(0))

	// response bytes contains 2 json arrays: [{"Key":...}][{"ResponseMetadata": ...}]
	responseBytes := sess.Out.Contents()
	index := bytes.LastIndex(responseBytes, []byte("]["))

	// unmarshal and verify response result
	results := make([]*marbleQueryResult, 0)
	err = json.Unmarshal(responseBytes[:index+1], &results)
	Expect(err).NotTo(HaveOccurred())
	Expect(results).To(Equal(expectedResult))

	// unmarshal ResponseMetadata and return bookmark to the caller for next call
	respMetadata := make([]*paginationMetadata, 0)
	err = json.Unmarshal(responseBytes[index+1:], &respMetadata)
	Expect(err).NotTo(HaveOccurred())
	Expect(respMetadata).To(HaveLen(1))

	return respMetadata[0].ResponseMetadata.Bookmark
}

// assertGetHistoryForMarble queries the history for a specific marble and verifies the result
func (th *marblesTestHelper) assertGetHistoryForMarble(chaincodeName string, peer *nwo.Peer, expectedResult []*marbleHistoryResult, marbleName string) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["getHistoryForMarble","%s"]}`, marbleName),
	}
	sess, err := th.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, th.EventuallyTimeout).Should(gexec.Exit(0))

	// unmarshal bytes and verify history
	results := make([]*marbleHistoryResult, 0)
	err = json.Unmarshal(sess.Out.Contents(), &results)
	Expect(err).NotTo(HaveOccurred())
	Expect(results).To(HaveLen(len(expectedResult)))
	for i, result := range results {
		Expect(result.IsDelete).To(Equal(expectedResult[i].IsDelete))
		if result.IsDelete == "true" {
			Expect(result.Value).To(BeNil())
			continue
		}
		Expect(result.Value).To(Equal(expectedResult[i].Value))
	}
}
