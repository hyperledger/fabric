/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package marblechaincodeutil

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// AddMarble invokes marbles_private chaincode to add a marble
func AddMarble(n *nwo.Network, orderer *nwo.Orderer, channelID, chaincodeName, marbleDetails string, peer *nwo.Peer) {
	marbleDetailsBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDetails))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      `{"Args":["initMarble"]}`,
		Transient: fmt.Sprintf(`{"marble":"%s"}`, marbleDetailsBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

// DeleteMarble invokes marbles_private chaincode to delete a marble
func DeleteMarble(n *nwo.Network, orderer *nwo.Orderer, channelID, chaincodeName, marbleDelete string, peer *nwo.Peer) {
	marbleDeleteBase64 := base64.StdEncoding.EncodeToString([]byte(marbleDelete))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      `{"Args":["delete"]}`,
		Transient: fmt.Sprintf(`{"marble_delete":"%s"}`, marbleDeleteBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

// PurgeMarble invokes marbles_private chaincode to purge a marble
func PurgeMarble(n *nwo.Network, orderer *nwo.Orderer, channelID, chaincodeName, marblePurge string, peer *nwo.Peer) {
	marblePurgeBase64 := base64.StdEncoding.EncodeToString([]byte(marblePurge))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      `{"Args":["purge"]}`,
		Transient: fmt.Sprintf(`{"marble_purge":"%s"}`, marblePurgeBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

// SetMarblePolicy invokes marbles_private chaincode to update a marble's state-based endorsement policy
func SetMarblePolicy(n *nwo.Network, orderer *nwo.Orderer, channelID, chaincodeName, marblePolicy string, peer *nwo.Peer) {
	marblePolicyBase64 := base64.StdEncoding.EncodeToString([]byte(marblePolicy))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      `{"Args":["setStateBasedEndorsementPolicy"]}`,
		Transient: fmt.Sprintf(`{"marble_ep":"%s"}`, marblePolicyBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

// TransferMarble invokes marbles_private chaincode to transfer marble's ownership
func TransferMarble(n *nwo.Network, orderer *nwo.Orderer, channelID, chaincodeName, marbleOwner string, peer *nwo.Peer) {
	marbleOwnerBase64 := base64.StdEncoding.EncodeToString([]byte(marbleOwner))

	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      `{"Args":["transferMarble"]}`,
		Transient: fmt.Sprintf(`{"marble_owner":"%s"}`, marbleOwnerBase64),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.Peers...)
}

// AssertGetMarblesByRange asserts that
func AssertGetMarblesByRange(n *nwo.Network, channelID, chaincodeName, marbleRange, expectedMsg string, peer *nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarblesByRange", %s]}`, marbleRange)
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, true, peer)
}

// AssertPresentInCollectionM asserts that the private data for given marble is present in collection
// 'readMarble' at the given peers
func AssertPresentInCollectionM(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`"docType":"marble","name":"%s"`, marbleName)
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, true, peerList...)
}

// AssertPresentInCollectionMPD asserts that the private data for given marble is present
// in collection 'readMarblePrivateDetails' at the given peers
func AssertPresentInCollectionMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`"docType":"marblePrivateDetails","name":"%s"`, marbleName)
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, true, peerList...)
}

// CheckPresentInCollectionM checks then number of peers that have the private data for given marble
// in collection 'readMarble'
func CheckPresentInCollectionM(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) int {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`{"docType":"marble","name":"%s"`, marbleName)
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}
	present := 0
	for _, peer := range peerList {
		sess, err := n.PeerUserSession(peer, "User1", command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		if bytes.Contains(sess.Buffer().Contents(), []byte(expectedMsg)) {
			present++
		}
	}
	return present
}

// CheckPresentInCollectionMPD checks the number of peers that have the private data for given marble
// in collection 'readMarblePrivateDetails'
func CheckPresentInCollectionMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) int {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`{"docType":"marblePrivateDetails","name":"%s"`, marbleName)
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}
	present := 0
	for _, peer := range peerList {
		sess, err := n.PeerUserSession(peer, "User1", command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		if bytes.Contains(sess.Buffer().Contents(), []byte(expectedMsg)) {
			present++
		}
	}
	return present
}

// AssertNotPresentInCollectionM asserts that the private data for given marble is NOT present
// in collection 'readMarble' at the given peers
func AssertNotPresentInCollectionM(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := "private data matching public hash version is not available"
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, false, peerList...)
}

// AssertNotPresentInCollectionMPD asserts that the private data for given marble is NOT present
// in collection 'readMarblePrivateDetails' at the given peers
func AssertNotPresentInCollectionMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "private data matching public hash version is not available"
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, false, peerList...)
}

// AssertDoesNotExistInCollectionM asserts that the private data for given marble
// does not exist in collection 'readMarble' (i.e., is never created/has been deleted/has been purged)
func AssertDoesNotExistInCollectionM(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := "Marble does not exist"
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, false, peerList...)
}

// AssertDoesNotExistInCollectionMPD asserts that the private data for given marble
// does not exist in collection 'readMarblePrivateDetails' (i.e., is never created/has been deleted/has been purged)
func AssertDoesNotExistInCollectionMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "Marble private details does not exist"
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, false, peerList...)
}

// AssertOwnershipInCollectionM asserts that the private data for given marble is present
// in collection 'readMarble' at the given peers
func AssertOwnershipInCollectionM(n *nwo.Network, channelID, chaincodeName, marbleName, owner string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName)
	expectedMsg := fmt.Sprintf(`"owner":"%s"`, owner)
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, true, peerList...)
}

// AssertNoReadAccessToCollectionMPD asserts that the orgs of the given peers do not have
// read access to private data for the collection readMarblePrivateDetails
func AssertNoReadAccessToCollectionMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName)
	expectedMsg := "tx creator does not have read access permission"
	queryChaincodePerPeer(n, query, channelID, chaincodeName, expectedMsg, false, peerList...)
}

func queryChaincodePerPeer(n *nwo.Network, query, channelID, chaincodeName, expectedMsg string, expectSuccess bool, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}
	for _, peer := range peerList {
		queryChaincode(n, peer, command, expectedMsg, expectSuccess)
	}
}

// AssertMarblesPrivateHashM asserts that getMarbleHash is accessible from all peers that has the chaincode instantiated
func AssertMarblesPrivateHashM(n *nwo.Network, channelID, chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarbleHash","%s"]}`, marbleName)
	verifyPvtdataHash(n, query, channelID, chaincodeName, peerList, expectedBytes)
}

// AssertMarblesPrivateDetailsHashMPD asserts that getMarblePrivateDetailsHash is accessible from all peers that has the chaincode instantiated
func AssertMarblesPrivateDetailsHashMPD(n *nwo.Network, channelID, chaincodeName, marbleName string, expectedBytes []byte, peerList []*nwo.Peer) {
	query := fmt.Sprintf(`{"Args":["getMarblePrivateDetailsHash","%s"]}`, marbleName)
	verifyPvtdataHash(n, query, channelID, chaincodeName, peerList, expectedBytes)
}

// AssertInvokeChaincodeFails asserts that a chaincode invoke fails with a specified error message
func AssertInvokeChaincodeFails(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke, expectedMessage string) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say(expectedMessage))
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

// verifyPvtdataHash verifies the private data hash matches the expected bytes.
// Cannot reuse verifyAccess because the hash bytes are not valid utf8 causing gbytes.Say to fail.
func verifyPvtdataHash(n *nwo.Network, query, channelID, chaincodeName string, peers []*nwo.Peer, expected []byte) {
	command := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      query,
	}

	for _, peer := range peers {
		sess, err := n.PeerUserSession(peer, "User1", command)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		actual := sess.Buffer().Contents()
		// verify actual bytes contain expected bytes - cannot use equal because session may contain extra bytes
		Expect(bytes.Contains(actual, expected)).To(Equal(true))
	}
}
