/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"encoding/json"
	"path/filepath"
	"strconv"

	token2 "github.com/hyperledger/fabric/token/cmd"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func IssueToken(n *nwo.Network, peer *nwo.Peer, o *nwo.Orderer, channelId, user, mspID, tokenType, tokenQuantity, tokenRecipient string) {
	config := getClientConfig(n, peer, o, channelId, user, mspID)
	jsonBytes, err := config.ToJSon()
	Expect(err).NotTo(HaveOccurred())

	sess, err := n.Token(commands.TokenIssue{
		Config:    string(jsonBytes),
		Channel:   channelId,
		MspPath:   n.PeerUserMSPDir(peer, user),
		MspID:     mspID,
		Type:      tokenType,
		Quantity:  tokenQuantity,
		Recipient: tokenRecipient,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	// Check output
	consoleOutput := string(sess.Out.Contents())
	Expect(consoleOutput).To(ContainSubstring("Orderer Status [SUCCESS]"))
	Expect(consoleOutput).To(ContainSubstring("Committed [true]"))
}

func ListTokens(n *nwo.Network, peer *nwo.Peer, o *nwo.Orderer, channelId, user, mspID string) []*token.UnspentToken {
	config := getClientConfig(n, peer, o, channelId, user, mspID)
	jsonBytes, err := config.ToJSon()
	Expect(err).NotTo(HaveOccurred())

	sess, err := n.Token(commands.TokenList{
		Config:  string(jsonBytes),
		Channel: channelId,
		MspPath: n.PeerUserMSPDir(peer, user),
		MspID:   mspID,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	// Extract Token Outputs
	tokens, err := token2.ExtractUnspentTokensFromOutput(string(sess.Out.Contents()))
	Expect(err).NotTo(HaveOccurred())

	return tokens
}

func TransferTokens(n *nwo.Network, peer *nwo.Peer, o *nwo.Orderer, channelId, user, mspID string, tokenIDs []*token.TokenId, shares []*token2.ShellRecipientShare) {
	config := getClientConfig(n, peer, o, channelId, user, mspID)
	jsonBytes, err := config.ToJSon()
	Expect(err).NotTo(HaveOccurred())

	tokenIDsJson, err := json.Marshal(tokenIDs)
	Expect(err).NotTo(HaveOccurred())

	sharesJson, err := json.Marshal(shares)
	Expect(err).NotTo(HaveOccurred())

	sess, err := n.Token(commands.TokenTransfer{
		Config:   string(jsonBytes),
		Channel:  channelId,
		MspPath:  n.PeerUserMSPDir(peer, user),
		MspID:    mspID,
		TokenIDs: string(tokenIDsJson),
		Shares:   string(sharesJson),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	// Check output
	consoleOutput := string(sess.Out.Contents())
	Expect(consoleOutput).To(ContainSubstring("Orderer Status [SUCCESS]"))
	Expect(consoleOutput).To(ContainSubstring("Committed [true]"))
}

func RedeemTokens(n *nwo.Network, peer *nwo.Peer, o *nwo.Orderer, channelId, user, mspID string, tokenIDs []*token.TokenId, quantity uint64) {
	config := getClientConfig(n, peer, o, channelId, user, mspID)
	jsonBytes, err := config.ToJSon()
	Expect(err).NotTo(HaveOccurred())

	tokenIDsJson, err := json.Marshal(tokenIDs)
	Expect(err).NotTo(HaveOccurred())

	sess, err := n.Token(commands.TokenRedeem{
		Config:   string(jsonBytes),
		Channel:  channelId,
		MspPath:  n.PeerUserMSPDir(peer, user),
		MspID:    mspID,
		TokenIDs: string(tokenIDsJson),
		Quantity: strconv.FormatUint(quantity, 10),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	// Check output
	consoleOutput := string(sess.Out.Contents())
	Expect(consoleOutput).To(ContainSubstring("Orderer Status [SUCCESS]"))
	Expect(consoleOutput).To(ContainSubstring("Committed [true]"))
}

func getClientConfig(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channelId, user, mspID string) *client.ClientConfig {
	mspDir := n.PeerUserMSPDir(peer, user)
	peerAddr := n.PeerAddress(peer, nwo.ListenPort)
	peerTLSRootCertFile := filepath.Join(n.PeerLocalTLSDir(peer), "ca.crt")
	ordererAddr := n.OrdererAddress(orderer, nwo.ListenPort)
	ordererTLSRootCertFile := filepath.Join(n.OrdererLocalTLSDir(orderer), "ca.crt")

	config := client.ClientConfig{
		ChannelID: channelId,
		MSPInfo: client.MSPInfo{
			MSPConfigPath: mspDir,
			MSPID:         mspID,
			MSPType:       "bccsp",
		},
		Orderer: client.ConnectionConfig{
			Address:         ordererAddr,
			TLSEnabled:      true,
			TLSRootCertFile: ordererTLSRootCertFile,
		},
		CommitterPeer: client.ConnectionConfig{
			Address:         peerAddr,
			TLSEnabled:      true,
			TLSRootCertFile: peerTLSRootCertFile,
		},
		ProverPeer: client.ConnectionConfig{
			Address:         peerAddr,
			TLSEnabled:      true,
			TLSRootCertFile: peerTLSRootCertFile,
		},
	}

	return &config
}
