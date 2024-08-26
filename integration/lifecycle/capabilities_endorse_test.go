/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/raft"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Lifecycle with Channel v3_0 capabilities and ed25519 identities", func() {
	var (
		client       *docker.Client
		testDir      string
		network      *nwo.Network
		ordererProcs []ifrit.Process
		peerProcs    []ifrit.Process
		channelID    string
		chaincode    nwo.Chaincode
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
		channelID = "testchannel"

		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.peer', 'Org2MSP.peer')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_simple_chaincode",
		}
	})

	AfterEach(func() {
		// Shutdown processes and cleanup
		allProcs := append(ordererProcs, peerProcs...)
		for _, process := range allProcs {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("invoke chaincode after upgrading Channel to V3_0 and add ed25519 peer and orderer", func() {
		networkConfig := nwo.MultiNodeEtcdRaft()
		networkConfig.Peers = append(
			networkConfig.Peers,
			&nwo.Peer{
				Name:         "peer1",
				Organization: "Org1",
			},
		)

		network = nwo.New(networkConfig, testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		org1Peer0 := network.Peer("Org1", "peer0")
		org2Peer0 := network.Peer("Org2", "peer0")
		orderer1 := network.Orderer("orderer1")
		orderer2 := network.Orderer("orderer2")
		orderer3 := network.Orderer("orderer3")

		By("starting ECDSA peers' and orderers' runners")
		org1p0Runner := network.PeerRunner(org1Peer0)
		org2p0Runner := network.PeerRunner(org2Peer0)
		org1p0Proc := ifrit.Invoke(org1p0Runner)
		org2p0Proc := ifrit.Invoke(org2p0Runner)
		peerProcs = []ifrit.Process{org1p0Proc, org2p0Proc}

		Eventually(org1p0Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		Eventually(org2p0Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

		o1Runner := network.OrdererRunner(orderer1)
		o2Runner := network.OrdererRunner(orderer2)
		o3Runner := network.OrdererRunner(orderer3)
		ordererRunners := []*ginkgomon_v2.Runner{o1Runner, o2Runner, o3Runner}

		o1Proc := ifrit.Invoke(o1Runner)
		o2Proc := ifrit.Invoke(o2Runner)
		o3Proc := ifrit.Invoke(o3Runner)
		ordererProcs = []ifrit.Process{o1Proc, o2Proc, o3Proc}

		Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("joining ECDSA orderers to testchannel")
		channelparticipation.JoinOrderersAppChannelCluster(network, channelID, orderer1, orderer2, orderer3)

		By("waiting for raft leader on testchannel")
		raft.FindLeader(ordererRunners)

		By("joining ECDSA peers to testchannel")
		network.JoinChannel(channelID, orderer1, org1Peer0, org2Peer0)
		network.VerifyMembership(network.PeersWithChannel(channelID), channelID)

		By("enabling V2_0 lifecycle capabilities on testchannel")
		nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer1, org1Peer0, org2Peer0)

		By("deploying the chaincode")
		nwo.DeployChaincode(network, channelID, orderer1, chaincode)

		By("querying and invoking chaincode")
		RunQueryInvokeQuery(network, orderer1, "mycc", 100, org1Peer0, org2Peer0)

		By("enabling V3_0 lifecycle capabilities on testchannel, which supports ed25519")
		nwo.EnableChannelCapabilities(network, channelID, "V3_0", true, orderer1, []*nwo.Orderer{orderer1},
			org1Peer0,
			org2Peer0,
		)

		By("Killing orderer3")
		o3Proc.Signal(syscall.SIGTERM)
		Eventually(o3Proc.Wait(), network.EventuallyTimeout).Should(Receive())

		By("Giving ed25519 certificate and key to orderer3")
		giveEd25519CertAndKeyForEntity(network, orderer3)

		By("Starting ed25519 orderer")
		o3Runner = network.OrdererRunner(orderer3)
		o3Proc = ifrit.Invoke(o3Runner)
		Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		ordererProcs[2] = o3Proc

		By("Waiting for orderer3 to know the raft leader")
		raft.FindLeader([]*ginkgomon_v2.Runner{o3Runner})

		By("starting the ed25519 peer")
		org1Ed25519Peer := network.Peer("Org1", "peer1")
		giveEd25519CertAndKeyForEntity(network, org1Ed25519Peer)

		ed25519PeerRunner := network.PeerRunner(org1Ed25519Peer)
		ed25519PeerProc := ifrit.Invoke(ed25519PeerRunner)
		Eventually(ed25519PeerProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		peerProcs = append(peerProcs, ed25519PeerProc)

		By("joining the ed25519 peer to testchannel")
		org1Ed25519Peer.Channels = []*nwo.PeerChannel{
			{Name: channelID, Anchor: false},
		}
		network.JoinChannel(channelID, orderer1, org1Ed25519Peer)

		By("waiting for the new peer to have the same ledger height")
		channelHeight := nwo.GetMaxLedgerHeight(network, channelID, org1Peer0)
		nwo.WaitUntilEqualLedgerHeight(network, channelID, channelHeight, org1Ed25519Peer)

		By("installing chaincode mycc on ed25519 peer")
		nwo.PackageAndInstallChaincode(network, chaincode, org1Ed25519Peer)

		By("querying the chaincode using ed25519 peer to ensure it is in the same state")
		QueryChaincode(network, "mycc", org1Ed25519Peer, 90)

		By("invoking the chaincode with the ed25519 endorser and send the transaction to the ed25519 orderer")
		endorsers := []*nwo.Peer{
			org1Ed25519Peer,
			org2Peer0,
		}
		RunQueryInvokeQuery(network, orderer3, "mycc", 90, endorsers...)

		By("ensuring all peers are in the same state")
		QueryChaincode(network, "mycc", org1Peer0, 80)
		QueryChaincode(network, "mycc", org2Peer0, 80)
		QueryChaincode(network, "mycc", org1Ed25519Peer, 80)
	})

	It("deploy chaincode in a Channel V3_0 and downgrade Channel to V2_0", func() {
		networkConfig := nwo.BasicEtcdRaft()
		networkConfig.Peers = append(
			networkConfig.Peers,
			&nwo.Peer{
				Name:         "peer1",
				Organization: "Org1",
			},
		)
		network = nwo.New(networkConfig, testDir, client, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		orderer := network.Orderer("orderer")
		org1Peer0 := network.Peer("Org1", "peer0")
		org2Peer0 := network.Peer("Org2", "peer0")
		org1Ed25519Peer := network.Peer("Org1", "peer1")

		By("starting peers' and orderers' runners")
		org1p0Runner := network.PeerRunner(org1Peer0)
		org2p0Runner := network.PeerRunner(org2Peer0)
		org1p0Proc := ifrit.Invoke(org1p0Runner)
		org2p0Proc := ifrit.Invoke(org2p0Runner)
		Eventually(org1p0Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		Eventually(org2p0Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		peerProcs = []ifrit.Process{org1p0Proc, org2p0Proc}

		ordererRunner := network.OrdererRunner(orderer)
		ordererProc := ifrit.Invoke(ordererRunner)
		Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		ordererProcs = []ifrit.Process{ordererProc}

		By("joining orderer to testchannel")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, channelID, orderer, ordererRunner)
		network.VerifyMembership(network.PeersWithChannel(channelID), channelID)

		By("setting up the channel with v3_0 capabilities and without the ed25519 peer")
		nwo.EnableCapabilities(network, channelID, "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
		nwo.EnableChannelCapabilities(network, channelID, "V3_0", false, orderer, []*nwo.Orderer{orderer},
			org1Peer0,
			org2Peer0,
		)

		By("deploying the chaincode")
		nwo.DeployChaincode(network, channelID, orderer, chaincode, org1Peer0, org2Peer0)

		By("Giving ed25519 certificate and key to org1Peer0")
		giveEd25519CertAndKeyForEntity(network, org1Ed25519Peer)

		By("starting the ed25519 peer")
		ed25519PeerRunner := network.PeerRunner(org1Ed25519Peer)
		ed25519PeerProcess := ifrit.Invoke(ed25519PeerRunner)
		Eventually(ed25519PeerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		peerProcs = append(peerProcs, ed25519PeerProcess)

		By("joining the ed25519 peer to testchannel")
		org1Ed25519Peer.Channels = []*nwo.PeerChannel{
			{Name: channelID, Anchor: false},
		}
		network.JoinChannel(channelID, orderer, org1Ed25519Peer)

		By("waiting for the new peer to have the same ledger height")
		channelHeight := nwo.GetMaxLedgerHeight(network, channelID, org1Peer0)
		nwo.WaitUntilEqualLedgerHeight(network, channelID, channelHeight, org1Ed25519Peer)

		By("installing chaincode mycc on ed25519 peer")
		nwo.PackageAndInstallChaincode(network, chaincode, org1Ed25519Peer)

		By("invoking the chaincode with the ed25519 endorser")
		endorsers := []*nwo.Peer{
			org1Ed25519Peer,
			org2Peer0,
		}
		RunQueryInvokeQuery(network, orderer, "mycc", 100, endorsers...)

		By("downgrading the channel capabilities back to v2_0")
		nwo.EnableChannelCapabilities(network, channelID, "V2_0", false, orderer, []*nwo.Orderer{orderer},
			org1Peer0,
			org2Peer0,
		)

		By("invoking the chaincode again, but expecting a failure")
		RunInvokeAndExpectFailure(network, orderer, "mycc", "(ENDORSEMENT_POLICY_FAILURE)", endorsers...)
	})
})

func giveEd25519CertAndKeyForEntity(network *nwo.Network, entitiy interface{}) {
	var certPath, keyPath, caCertPath, caKeyPath string
	if peer, ok := entitiy.(*nwo.Peer); ok {
		certPath = network.PeerCert(peer)
		caCertPath = network.PeerCACert(peer)
		domain := network.Organization(peer.Organization).Domain
		caKeyPath = filepath.Join(network.RootDir, "crypto", "peerOrganizations", domain, "ca", "priv_sk")
	} else if orderer, ok := entitiy.(*nwo.Orderer); ok {
		certPath = network.OrdererCert(orderer)
		domain := network.Organization(orderer.Organization).Domain
		caCertPath = network.OrdererCACert(orderer)
		caKeyPath = filepath.Join(network.RootDir, "crypto", "ordererOrganizations", domain, "ca", "priv_sk")
	} else {
		Fail("Invalid entity type")
	}

	keyPath = filepath.Join(certPath, "..", "..", "keystore", "priv_sk")

	entityCert := getX509Certificate(certPath)
	caCert := getX509Certificate(caCertPath)
	caKey := getPrivateKey(caKeyPath)

	newPub, newPriv, err := ed25519.GenerateKey(rand.Reader)
	Expect(err).NotTo(HaveOccurred(), "Could not generate the new key")

	// The CA signs the certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, entityCert, caCert, newPub, caKey)
	Expect(err).NotTo(HaveOccurred())

	newCertPemBytes := pem.EncodeToMemory(&pem.Block{Bytes: certBytes, Type: "CERTIFICATE"})
	Expect(os.WriteFile(certPath, newCertPemBytes, fs.ModeExclusive)).NotTo(HaveOccurred())

	newPrivBytes, err := x509.MarshalPKCS8PrivateKey(newPriv)
	Expect(err).NotTo(HaveOccurred())
	newPrivPemBytes := pem.EncodeToMemory(&pem.Block{Bytes: newPrivBytes, Type: "PRIVATE KEY"})
	Expect(os.WriteFile(keyPath, newPrivPemBytes, fs.ModeExclusive)).NotTo(HaveOccurred())
}

func getX509Certificate(certPath string) *x509.Certificate {
	cert, err := os.ReadFile(certPath)
	Expect(err).NotTo(HaveOccurred())
	der, _ := pem.Decode(cert)
	Expect(der).NotTo(BeNil())
	x509Cert, err := x509.ParseCertificate(der.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return x509Cert
}

func getPrivateKey(keyPath string) crypto.PrivateKey {
	key, err := os.ReadFile(keyPath)
	Expect(err).NotTo(HaveOccurred())
	der, _ := pem.Decode(key)
	Expect(der).NotTo(BeNil())
	priv, err := x509.ParsePKCS8PrivateKey(der.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return priv
}
