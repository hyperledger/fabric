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
	"github.com/hyperledger/fabric/protos/token"
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
	)

	BeforeEach(func() {
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
			Skip("Skipping token e2e test until token transaction is enabled after v1.4")
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer1")

			By("submitting a token transaction")
			RunTokenTransactionSubmit(network, orderer, peer)
		})
	})
})

func RunTokenTransactionSubmit(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) {
	user := "User1"
	mspDir := n.PeerUserMSPDir(peer, user)
	mspId := "Org1MSP"

	ordererAddr := n.OrdererAddress(orderer, nwo.ListenPort)
	ordererTlsRootCertFile := filepath.Join(n.OrdererLocalTLSDir(orderer), "ca.crt")

	peerAddr := n.PeerAddress(peer, nwo.ListenPort)
	peerTlsRootCertFile := filepath.Join(n.PeerLocalTLSDir(peer), "ca.crt")

	ordererCfg := tokenclient.ConnectionConfig{
		Address:         ordererAddr,
		TlsRootCertFile: ordererTlsRootCertFile,
	}

	commitPeerCfg := tokenclient.ConnectionConfig{
		Address:         peerAddr,
		TlsRootCertFile: peerTlsRootCertFile,
	}

	config := &tokenclient.ClientConfig{
		ChannelId:     "testchannel",
		MspDir:        mspDir,
		MspId:         mspId,
		TlsEnabled:    true,
		OrdererCfg:    ordererCfg,
		CommitPeerCfg: commitPeerCfg,
	}

	txSubmitter, err := tokenclient.NewTxSubmitter(config)
	Expect(err).NotTo(HaveOccurred())

	mockTokenTx := &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainImport{
					PlainImport: &token.PlainImport{
						Outputs: []*token.PlainOutput{{
							Owner:    []byte("new-owner"),
							Type:     "ABC123",
							Quantity: 111,
						}},
					}}}}}
	mockTokenTxBytes, err := proto.Marshal(mockTokenTx)
	Expect(err).NotTo(HaveOccurred())

	_, txEnvelope, err := txSubmitter.CreateTxEnvelope(mockTokenTxBytes)
	Expect(err).NotTo(HaveOccurred())
	committed, _, err := txSubmitter.SubmitTransaction(txEnvelope, 60)
	Expect(err).NotTo(HaveOccurred())
	Expect(committed).To(Equal(true))
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
