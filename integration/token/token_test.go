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

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/pkg/errors"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/msp"
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
		expectedUnspentTokens    *token.UnspentTokens
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

		expectedUnspentTokens = &token.UnspentTokens{
			Tokens: []*token.TokenOutput{
				{
					Quantity: 119,
					Type:     "ABC123",
					Id:       []byte("ledger-id"),
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

			peer := network.Peer("Org1", "peer1")
			// Get recipient's identity
			recipient, err := getIdentity(network, peer, "User2", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			tokensToIssue[0].Recipient = recipient
			expectedTokenTransaction.GetPlainAction().GetPlainImport().Outputs[0].Owner = recipient

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

			By("list tokens")
			config = getClientConfig(network, peer, orderer, "testchannel", "User2", "Org1MSP")
			signingIdentity, err = getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			Expect(err).NotTo(HaveOccurred())
			tClient, err = tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())
			RunListTokens(tClient, expectedUnspentTokens)
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

func RunListTokens(c *tokenclient.Client, expectedUnspentTokens *token.UnspentTokens) []*token.TokenOutput {
	tokenOutputs, err := c.ListTokens()
	Expect(err).NotTo(HaveOccurred())

	// unmarshall CommandResponse and verify the result
	Expect(len(tokenOutputs)).To(Equal(len(expectedUnspentTokens.Tokens)))
	for i, token := range expectedUnspentTokens.Tokens {
		Expect(tokenOutputs[i].Type).To(Equal(token.Type))
		Expect(tokenOutputs[i].Quantity).To(Equal(token.Quantity))
	}
	return tokenOutputs
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

func getIdentity(n *nwo.Network, peer *nwo.Peer, user, mspId string) ([]byte, error) {
	orderer := n.Orderer("orderer")
	config := getClientConfig(n, peer, orderer, "testchannel", user, mspId)

	localMSP, err := LoadLocalMSPAt(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
	if err != nil {
		return nil, err
	}

	signer, err := localMSP.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	return creator, nil
}

func getSigningIdentity(mspConfigPath, mspID, mspType string) (tk.SigningIdentity, error) {
	mspInstance, err := LoadLocalMSPAt(mspConfigPath, mspID, mspType)
	if err != nil {
		return nil, err
	}

	signingIdentity, err := mspInstance.GetDefaultSigningIdentity()
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

// LoadLocalMSPAt loads an MSP whose configuration is stored at 'dir', and whose
// id and type are the passed as arguments.
func LoadLocalMSPAt(dir, id, mspType string) (msp.MSP, error) {
	if mspType != "bccsp" {
		return nil, errors.Errorf("invalid msp type, expected 'bccsp', got %s", mspType)
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, id)
	if err != nil {
		return nil, err
	}
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	if err != nil {
		return nil, err
	}
	thisMSP, err := msp.NewBccspMspWithKeyStore(msp.MSPv1_0, ks)
	if err != nil {
		return nil, err
	}
	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, err
	}
	return thisMSP, nil
}
