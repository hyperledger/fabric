/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	peercommon "github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	tk "github.com/hyperledger/fabric/token"
	tokenclient "github.com/hyperledger/fabric/token/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Token EndToEnd", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process

		tokensToIssue            []*token.TokenToIssue
		expectedTokenTransaction *token.TokenTransaction
	)

	BeforeEach(func() {
		// prepare data for issue tokens
		tokensToIssue = []*token.TokenToIssue{
			{Recipient: []byte("test-owner"), Type: "ABC123", Quantity: 119},
		}

		// expected token transaction returned by prover peer
		expectedTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainImport{
						PlainImport: &token.PlainImport{
							Outputs: []*token.PlainOutput{
								{Owner: []byte("test-owner"), Type: "ABC123", Quantity: 119},
							},
						},
					},
				},
			},
		}

		var err error
		testDir, err = ioutil.TempDir("", "token-e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
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

	Describe("basic solo network for token transaction e2e", func() {
		BeforeEach(func() {
			var err error
			network = nwo.New(nwo.BasicSolo(), testDir, client, 30000, components)
			network.GenerateConfigTree()

			// update configtx with fabtoken capability
			err = updateConfigtx(network)
			Expect(err).NotTo(HaveOccurred())

			network.Bootstrap()

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())
		})

		It("executes a basic solo network and submits token transaction", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer1")

			By("creating a new client")
			config := getClientConfig(network, peer, orderer, "testchannel", "User1", "Org1MSP")
			signingIdentity, err := getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			Expect(err).NotTo(HaveOccurred())
			tClient, err := tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())

			By("issuing tokens")
			RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)
		})
	})
})

func RunIssueRequest(c *tokenclient.Client, tokens []*token.TokenToIssue, expectedTokenTx *token.TokenTransaction) {
	envelope, txid, ordererStatus, committed, err := c.Issue(tokens, 30*time.Second)
	Expect(err).NotTo(HaveOccurred())
	Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
	Expect(committed).To(Equal(true))

	payload := common.Payload{}
	err = proto.Unmarshal(envelope.Payload, &payload)

	tokenTx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, tokenTx)
	Expect(proto.Equal(tokenTx, expectedTokenTx)).To(BeTrue())

	tokenTxid, err := tokenclient.GetTransactionID(envelope)
	Expect(err).NotTo(HaveOccurred())
	Expect(tokenTxid).To(Equal(txid))
}

func getClientConfig(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channelId, user, mspID string) *tokenclient.ClientConfig {
	mspDir := n.PeerUserMSPDir(peer, user)
	peerAddr := n.PeerAddress(peer, nwo.ListenPort)
	peerTLSRootCertFile := filepath.Join(n.PeerLocalTLSDir(peer), "ca.crt")
	ordererAddr := n.OrdererAddress(orderer, nwo.ListenPort)
	ordererTLSRootCertFile := filepath.Join(n.OrdererLocalTLSDir(orderer), "ca.crt")

	config := tokenclient.ClientConfig{
		ChannelID: "testchannel",
		MSPInfo: tokenclient.MSPInfo{
			MSPConfigPath: mspDir,
			MSPID:         mspID,
			MSPType:       "bccsp",
		},
		Orderer: tokenclient.ConnectionConfig{
			Address:         ordererAddr,
			TLSEnabled:      true,
			TLSRootCertFile: ordererTLSRootCertFile,
		},
		CommitterPeer: tokenclient.ConnectionConfig{
			Address:         peerAddr,
			TLSEnabled:      true,
			TLSRootCertFile: peerTLSRootCertFile,
		},
		ProverPeer: tokenclient.ConnectionConfig{
			Address:         peerAddr,
			TLSEnabled:      true,
			TLSRootCertFile: peerTLSRootCertFile,
		},
	}

	return &config
}

func getSigningIdentity(mspConfigPath, mspID, mspType string) (tk.SigningIdentity, error) {
	peercommon.InitCrypto(mspConfigPath, mspID, mspType)
	signingIdentity, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}
	return signingIdentity, nil
}

// update configtx.yaml with V1_4_FABTOKEN_EXPERIMENTAL: true
func updateConfigtx(network *nwo.Network) error {
	filepath := network.ConfigTxConfigPath()
	input, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}

	// update the CAPABILITY_PLACEHOLDER to enable fabtoken capability
	output := bytes.Replace(input, []byte("CAPABILITY_PLACEHOLDER: false"), []byte("V1_4_FABTOKEN_EXPERIMENTAL: true"), -1)
	return ioutil.WriteFile(filepath, output, 0644)
}
