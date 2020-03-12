/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/pkg/config"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Config", func() {
	var (
		client  *docker.Client
		testDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "config")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)

		// Generate config
		network.GenerateConfigTree()

		// bootstrap the network
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}

		os.RemoveAll(testDir)
	})

	It("creates channels and updates them using pkg/config", func() {
		orderer := network.Orderer("orderer")
		testPeers := network.PeersWithChannel("testchannel")
		org1peer0 := network.Peer("Org1", "peer0")

		By("setting up the channel")
		channel := config.Channel{
			ChannelID:  "testchannel",
			Consortium: "SampleConsortium",
			Application: config.Application{
				Organizations: []config.Organization{
					{
						Name: "Org1",
					},
					{
						Name: "Org2",
					},
				},
				Capabilities: map[string]bool{"V1_3": true},
				ACLs:         map[string]string{"event/Block": "/Channel/Application/Readers"},
				Policies: map[string]config.Policy{
					config.ReadersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Readers",
					},
					config.WritersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Writers",
					},
					config.AdminsPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Admins",
					},
					config.EndorsementPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
					config.LifecycleEndorsementPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
				},
			},
		}

		envelope, err := config.NewCreateChannelTx(channel)
		Expect(err).NotTo(HaveOccurred())
		envBytes, err := proto.Marshal(envelope)
		Expect(err).NotTo(HaveOccurred())
		channelCreateTxPath := network.CreateChannelTxPath("testchannel")
		err = ioutil.WriteFile(channelCreateTxPath, envBytes, 0644)
		Expect(err).NotTo(HaveOccurred())

		By("creating the channel")
		createChannel := func() int {
			sess, err := network.PeerAdminSession(org1peer0, commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(orderer, nwo.ListenPort),
				File:        channelCreateTxPath,
				OutputBlock: "/dev/null",
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			return sess.Wait(network.EventuallyTimeout).ExitCode()
		}
		Eventually(createChannel, network.EventuallyTimeout).Should(Equal(0))

		By("joining all peers to the channel")
		network.JoinChannel("testchannel", orderer, testPeers...)

		By("adding the anchor peer for each org")
		for _, peer := range network.AnchorsForChannel("testchannel") {
			By("getting the current channel config")
			channelConfig := nwo.GetConfig(network, peer, orderer, "testchannel")
			updatedChannelConfig := proto.Clone(channelConfig).(*cb.Config)

			By("adding the anchor peer for " + peer.Organization)
			host, port := peerHostPort(network, peer)
			err = config.AddAnchorPeer(updatedChannelConfig, peer.Organization, config.AnchorPeer{Host: host, Port: port})
			Expect(err).NotTo(HaveOccurred())

			By("computing the config update")
			configUpdate, err := config.ComputeUpdate(channelConfig, updatedChannelConfig, "testchannel")
			Expect(err).NotTo(HaveOccurred())

			By("creating a detached signature")
			signingIdentity := config.SigningIdentity{
				Certificate: parsePeerX509Certificate(network, peer),
				PrivateKey:  parsePeerPrivateKey(network, peer, "Admin"),
				MSPID:       network.Organization(peer.Organization).MSPID,
			}
			signature, err := config.SignConfigUpdate(configUpdate, signingIdentity)
			Expect(err).NotTo(HaveOccurred())

			By("creating a signed config update envelope with the detached signature")
			configUpdateEnvelope, err := config.CreateSignedConfigUpdateEnvelope(configUpdate, signingIdentity, signature)
			configUpdateBytes, err := proto.Marshal(configUpdateEnvelope)
			Expect(err).NotTo(HaveOccurred())
			tempFile, err := ioutil.TempFile("", "add-anchor-peer")
			Expect(err).NotTo(HaveOccurred())
			tempFile.Close()
			defer os.Remove(tempFile.Name())
			err = ioutil.WriteFile(tempFile.Name(), configUpdateBytes, 0644)
			Expect(err).NotTo(HaveOccurred())

			currentBlockNumber := nwo.CurrentConfigBlockNumber(network, peer, orderer, "testchannel")

			By("submitting the channel config update")
			sess, err := network.PeerAdminSession(peer, commands.ChannelUpdate{
				ChannelID:  "testchannel",
				Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
				File:       tempFile.Name(),
				ClientAuth: network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess.Err).To(gbytes.Say("Successfully submitted channel update"))

			ccb := func() uint64 { return nwo.CurrentConfigBlockNumber(network, peer, orderer, "testchannel") }
			Eventually(ccb, network.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))

			By("ensuring the active channel config matches the submitted config")
			finalChannelConfig := nwo.GetConfig(network, peer, orderer, "testchannel")
			configUpdate, err = config.ComputeUpdate(updatedChannelConfig, finalChannelConfig, "testchannel")
			Expect(configUpdate).To(BeNil())
			Expect(err).To(MatchError("failed to compute update: no differences detected between original and updated config"))
		}
	})
})

func parsePeerX509Certificate(n *nwo.Network, p *nwo.Peer) *x509.Certificate {
	certBytes, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(certBytes)
	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return cert
}

func parsePeerPrivateKey(n *nwo.Network, p *nwo.Peer, user string) crypto.PrivateKey {
	pkBytes, err := ioutil.ReadFile(n.PeerUserKey(p, user))
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(pkBytes)
	privateKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return privateKey
}

func peerHostPort(n *nwo.Network, p *nwo.Peer) (string, int) {
	host, port, err := net.SplitHostPort(n.PeerAddress(p, nwo.ListenPort))
	Expect(err).NotTo(HaveOccurred())
	portInt, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())
	return host, portInt
}
