/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigTx", func() {
	var (
		client  *docker.Client
		testDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "configtx")
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

	It("creates channels and updates them using fabric-config/configtx", func() {
		orderer := network.Orderer("orderer")
		org1peer0 := network.Peer("Org1", "peer0")

		By("setting up the channel")
		channel := configtx.Channel{
			Consortium: "SampleConsortium",
			Application: configtx.Application{
				Organizations: []configtx.Organization{
					{
						Name: "Org1",
					},
					{
						Name: "Org2",
					},
				},
				Capabilities: []string{"V1_3"},
				ACLs:         map[string]string{"event/Block": "/Channel/Application/Readers"},
				Policies: map[string]configtx.Policy{
					configtx.ReadersPolicyKey: {
						Type: configtx.ImplicitMetaPolicyType,
						Rule: "ANY Readers",
					},
					configtx.WritersPolicyKey: {
						Type: configtx.ImplicitMetaPolicyType,
						Rule: "ANY Writers",
					},
					configtx.AdminsPolicyKey: {
						Type: configtx.ImplicitMetaPolicyType,
						Rule: "MAJORITY Admins",
					},
					configtx.EndorsementPolicyKey: {
						Type: configtx.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
					configtx.LifecycleEndorsementPolicyKey: {
						Type: configtx.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
				},
			},
		}

		channelID := "testchannel"
		createChannelUpdate, err := configtx.NewMarshaledCreateChannelTx(channel, channelID)
		Expect(err).NotTo(HaveOccurred())

		envelope, err := configtx.NewEnvelope(createChannelUpdate)
		Expect(err).NotTo(HaveOccurred())
		envBytes, err := proto.Marshal(envelope)
		Expect(err).NotTo(HaveOccurred())
		channelCreateTxPath := network.CreateChannelTxPath("testchannel")
		err = ioutil.WriteFile(channelCreateTxPath, envBytes, 0o644)
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
		testPeers := network.PeersWithChannel("testchannel")
		network.JoinChannel("testchannel", orderer, testPeers...)

		By("getting the current channel config")
		org2peer0 := network.Peer("Org2", "peer0")
		channelConfig := nwo.GetConfig(network, org2peer0, orderer, "testchannel")
		c := configtx.New(channelConfig)

		By("updating orderer channel configuration")
		o := c.Orderer()
		oConfig, err := o.Configuration()
		Expect(err).NotTo(HaveOccurred())
		oConfig.BatchTimeout = 2 * time.Second
		err = o.SetConfiguration(oConfig)
		Expect(err).NotTo(HaveOccurred())
		host, port := OrdererHostPort(network, orderer)
		err = o.Organization(orderer.Organization).SetEndpoint(configtx.Address{Host: host, Port: port + 1})
		Expect(err).NotTo(HaveOccurred())

		By("computing the config update")
		configUpdate, err := c.ComputeMarshaledUpdate("testchannel")
		Expect(err).NotTo(HaveOccurred())

		By("creating a detached signature for the orderer")
		signingIdentity := configtx.SigningIdentity{
			Certificate: parseCertificate(network.OrdererUserCert(orderer, "Admin")),
			PrivateKey:  parsePrivateKey(network.OrdererUserKey(orderer, "Admin")),
			MSPID:       network.Organization(orderer.Organization).MSPID,
		}
		signature, err := signingIdentity.CreateConfigSignature(configUpdate)
		Expect(err).NotTo(HaveOccurred())

		By("creating a signed config update envelope with the orderer's detached signature")
		configUpdateEnvelope, err := configtx.NewEnvelope(configUpdate, signature)
		Expect(err).NotTo(HaveOccurred())
		err = signingIdentity.SignEnvelope(configUpdateEnvelope)
		Expect(err).NotTo(HaveOccurred())

		currentBlockNumber := nwo.CurrentConfigBlockNumber(network, org2peer0, orderer, "testchannel")

		By("submitting the channel config update")
		resp, err := ordererclient.Broadcast(network, orderer, configUpdateEnvelope)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Equal(common.Status_SUCCESS))

		ccb := func() uint64 { return nwo.CurrentConfigBlockNumber(network, org2peer0, orderer, "testchannel") }
		Eventually(ccb, network.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))

		By("ensuring the active channel config matches the submitted config")
		updatedChannelConfig := nwo.GetConfig(network, org2peer0, orderer, "testchannel")
		Expect(proto.Equal(c.UpdatedConfig(), updatedChannelConfig)).To(BeTrue())

		By("checking the current application capabilities")
		c = configtx.New(updatedChannelConfig)
		a := c.Application()
		capabilities, err := a.Capabilities()
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities).To(HaveLen(1))
		Expect(capabilities).To(ContainElement("V1_3"))

		By("enabling V2_0 application capabilities")
		err = a.AddCapability("V2_0")
		Expect(err).NotTo(HaveOccurred())

		By("checking the application capabilities after update")
		capabilities, err = a.Capabilities()
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities).To(HaveLen(2))
		Expect(capabilities).To(ContainElements("V1_3", "V2_0"))

		By("computing the config update")
		configUpdate, err = c.ComputeMarshaledUpdate("testchannel")
		Expect(err).NotTo(HaveOccurred())

		By("creating detached signatures for each peer")
		signingIdentities := make([]configtx.SigningIdentity, len(testPeers))
		signatures := make([]*common.ConfigSignature, len(testPeers))
		for i, p := range testPeers {
			signingIdentity := configtx.SigningIdentity{
				Certificate: parseCertificate(network.PeerUserCert(p, "Admin")),
				PrivateKey:  parsePrivateKey(network.PeerUserKey(p, "Admin")),
				MSPID:       network.Organization(p.Organization).MSPID,
			}
			signingIdentities[i] = signingIdentity
			signature, err := signingIdentity.CreateConfigSignature(configUpdate)
			Expect(err).NotTo(HaveOccurred())
			signatures[i] = signature
		}

		By("creating a signed config update envelope with the detached peer signatures")
		configUpdateEnvelope, err = configtx.NewEnvelope(configUpdate, signatures...)
		Expect(err).NotTo(HaveOccurred())
		err = signingIdentities[0].SignEnvelope(configUpdateEnvelope)
		Expect(err).NotTo(HaveOccurred())

		currentBlockNumber = nwo.CurrentConfigBlockNumber(network, org2peer0, orderer, "testchannel")

		By("submitting the channel config update")
		resp, err = ordererclient.Broadcast(network, orderer, configUpdateEnvelope)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Equal(common.Status_SUCCESS))

		ccb = func() uint64 { return nwo.CurrentConfigBlockNumber(network, org2peer0, orderer, "testchannel") }
		Eventually(ccb, network.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))

		By("ensuring the active channel config matches the submitted config")
		updatedChannelConfig = nwo.GetConfig(network, org2peer0, orderer, "testchannel")
		Expect(proto.Equal(c.UpdatedConfig(), updatedChannelConfig)).To(BeTrue())

		By("adding the anchor peer for each org")
		for _, peer := range network.AnchorsForChannel("testchannel") {
			By("getting the current channel config")
			channelConfig = nwo.GetConfig(network, peer, orderer, "testchannel")
			c = configtx.New(channelConfig)
			peerOrg := c.Application().Organization(peer.Organization)

			By("adding the anchor peer for " + peer.Organization)
			host, port := PeerHostPort(network, peer)
			err = peerOrg.AddAnchorPeer(configtx.Address{Host: host, Port: port})
			Expect(err).NotTo(HaveOccurred())

			By("computing the config update")
			configUpdate, err = c.ComputeMarshaledUpdate("testchannel")
			Expect(err).NotTo(HaveOccurred())

			By("creating a detached signature")
			signingIdentity := configtx.SigningIdentity{
				Certificate: parseCertificate(network.PeerUserCert(peer, "Admin")),
				PrivateKey:  parsePrivateKey(network.PeerUserKey(peer, "Admin")),
				MSPID:       network.Organization(peer.Organization).MSPID,
			}
			signature, err := signingIdentity.CreateConfigSignature(configUpdate)
			Expect(err).NotTo(HaveOccurred())

			By("creating a signed config update envelope with the detached peer signature")
			configUpdateEnvelope, err = configtx.NewEnvelope(configUpdate, signature)
			Expect(err).NotTo(HaveOccurred())
			err = signingIdentity.SignEnvelope(configUpdateEnvelope)
			Expect(err).NotTo(HaveOccurred())

			currentBlockNumber = nwo.CurrentConfigBlockNumber(network, peer, orderer, "testchannel")

			By("submitting the channel config update for " + peer.Organization)
			resp, err = ordererclient.Broadcast(network, orderer, configUpdateEnvelope)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			ccb = func() uint64 { return nwo.CurrentConfigBlockNumber(network, peer, orderer, "testchannel") }
			Eventually(ccb, network.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))

			By("ensuring the active channel config matches the submitted config")
			updatedChannelConfig = nwo.GetConfig(network, peer, orderer, "testchannel")
			Expect(proto.Equal(c.UpdatedConfig(), updatedChannelConfig)).To(BeTrue())
		}
	})
})

// parsePrivateKey loads the PEM-encoded private key at the specified path.
func parsePrivateKey(path string) crypto.PrivateKey {
	pkBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(pkBytes)
	privateKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return privateKey
}

// parseCertificate loads the PEM-encoded x509 certificate at the specified
// path.
func parseCertificate(path string) *x509.Certificate {
	certBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(certBytes)
	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return cert
}
