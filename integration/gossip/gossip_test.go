/*
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package gossip

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Gossip State Transfer and Membership", func() {
	var (
		testDir     string
		network     *nwo.Network
		nwprocs     *networkProcesses
		chaincode   nwo.Chaincode
		channelName string
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "gossip-statexfer")
		Expect(err).NotTo(HaveOccurred())

		dockerClient, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		channelName = "testchannel"
		network = nwo.New(nwo.FullEtcdRaft(), testDir, dockerClient, StartPort(), components)
		network.GenerateConfigTree()

		nwprocs = &networkProcesses{
			network:       network,
			peerRunners:   map[string]*ginkgomon.Runner{},
			peerProcesses: map[string]ifrit.Process{},
		}

		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "1.0",
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: "OR('Org1MSP.member','Org2MSP.member')",
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_prebuilt_chaincode",
		}
	})

	AfterEach(func() {
		if nwprocs != nil {
			nwprocs.terminateAll()
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("syncs blocks from the peer via state transfer when no orderer is available", func() {
		//  modify peer config to enable state transfer on all peers, and configure leaders as follows:
		//  Org1: leader election
		//  Org2: no leader election
		//      peer0: follower
		//      peer1: leader
		for _, peer := range network.Peers {
			if peer.Organization == "Org1" {
				if peer.Name == "peer0" {
					core := network.ReadPeerConfig(peer)
					core.Peer.Gossip.State.Enabled = true
					core.Peer.Gossip.UseLeaderElection = true
					core.Peer.Gossip.OrgLeader = false
					network.WritePeerConfig(peer, core)
				}
				if peer.Name == "peer1" {
					core := network.ReadPeerConfig(peer)
					core.Peer.Gossip.State.Enabled = true
					core.Peer.Gossip.UseLeaderElection = true
					core.Peer.Gossip.OrgLeader = false
					core.Peer.Gossip.Bootstrap = fmt.Sprintf("127.0.0.1:%d", network.ReservePort())
					network.WritePeerConfig(peer, core)
				}
			}
			if peer.Organization == "Org2" {
				core := network.ReadPeerConfig(peer)
				core.Peer.Gossip.State.Enabled = true
				core.Peer.Gossip.UseLeaderElection = false
				core.Peer.Gossip.OrgLeader = peer.Name == "peer1"
				network.WritePeerConfig(peer, core)
			}
		}

		network.Bootstrap()
		orderer := network.Orderer("orderer")
		nwprocs.ordererRunner = network.OrdererRunner(orderer)
		nwprocs.ordererProcess = ifrit.Invoke(nwprocs.ordererRunner)
		Eventually(nwprocs.ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
		peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

		By("bringing up all four peers")
		startPeers(nwprocs, false, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

		channelparticipation.JoinOrdererAppChannel(network, "testchannel", orderer, nwprocs.ordererRunner)

		By("joining all peers to channel")
		network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

		By("enabling V2_5 application capabilities on the channel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_5", orderer, network.Peers...)

		// base peer will be used for chaincode interactions
		basePeerForTransactions := peer0Org1

		By("packaging chaincode")
		nwo.PackageChaincodeBinary(chaincode)

		By("installing chaincode to org1.peer0")
		nwo.InstallChaincode(network, chaincode, peer0Org1, peer0Org2)

		By("approving chaincode definition for org1")
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, peer0Org1)
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, peer0Org2)

		By("committing chaincode definition using org1.peer0")
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, peer0Org1, peer0Org1, peer0Org2)
		nwo.InitChaincode(network, "testchannel", orderer, chaincode, peer0Org1)

		By("verifying peer0Org1 discovers all the peers and the legacy chaincode before starting the tests")
		Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org1, "_lifecycle"),
			network.DiscoveredPeer(peer0Org2, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org2, "_lifecycle"),
		))

		By("STATE TRANSFER TEST 1: newly joined peers should receive blocks from the peers that are already up")

		// Note, a better test would be to bring orderer down before joining the two peers.
		// However, network.JoinChannel() requires orderer to be up so that genesis block can be fetched from orderer before joining peers.
		// Therefore, for now we've joined all four peers and stop the two peers that should be synced up.
		stopPeers(nwprocs, peer1Org1, peer1Org2)

		By("confirming peer0Org1 was elected to be a leader")
		expectedMsg := "Elected as a leader, starting delivery service for channel testchannel"
		Eventually(nwprocs.peerRunners[peer0Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

		sendTransactionsAndSyncUpPeers(nwprocs, orderer, basePeerForTransactions, channelName, peer1Org1, peer1Org2)

		By("STATE TRANSFER TEST 2: restarted peers should receive blocks from the peers that are already up")
		basePeerForTransactions = peer1Org1
		nwo.InstallChaincode(network, chaincode, basePeerForTransactions)

		By("verifying peer0Org1 discovers all the peers and the additional legacy chaincode installed on peer1Org1")
		Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org1, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer0Org2, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org2, "_lifecycle"),
		))

		By("stopping peer0Org1 (currently elected leader in Org1) and peer1Org2 (static leader in Org2)")
		stopPeers(nwprocs, peer0Org1, peer1Org2)

		By("confirming peer1Org1 was elected to be a leader")
		Eventually(nwprocs.peerRunners[peer1Org1.ID()].Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsg))

		// Note that with the static leader in Org2 down, the static follower peer0Org2 will also get blocks via state transfer
		// This effectively tests leader election as well, since the newly elected leader in Org1 (peer1Org1) will be the only peer
		// that receives blocks from orderer and will therefore serve as the provider of blocks to all other peers.
		sendTransactionsAndSyncUpPeers(nwprocs, orderer, basePeerForTransactions, channelName, peer0Org1, peer1Org2)

		By("verifying peer0Org1 can still discover all the peers and the legacy chaincode after it has been restarted")
		Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
			network.DiscoveredPeer(peer0Org1, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org1, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer0Org2, "mycc", "_lifecycle"),
			network.DiscoveredPeer(peer1Org2, "_lifecycle"),
		))
	})

	When("gossip connection is lost and restored", func() {
		var (
			orderer       *nwo.Orderer
			peerEndpoints = map[string]string{}
		)

		BeforeEach(func() {
			//  modify peer config
			for _, peer := range network.Peers {
				core := network.ReadPeerConfig(peer)
				core.Peer.Gossip.AliveTimeInterval = 1 * time.Second
				core.Peer.Gossip.AliveExpirationTimeout = 2 * core.Peer.Gossip.AliveTimeInterval
				core.Peer.Gossip.ReconnectInterval = 2 * time.Second
				core.Peer.Gossip.MsgExpirationFactor = 2
				core.Peer.Gossip.MaxConnectionAttempts = 10
				network.WritePeerConfig(peer, core)
				peerEndpoints[peer.ID()] = core.Peer.Address
			}

			network.Bootstrap()
			orderer = network.Orderer("orderer")
			nwprocs.ordererRunner = network.OrdererRunner(orderer)
			nwprocs.ordererProcess = ifrit.Invoke(nwprocs.ordererRunner)
			Eventually(nwprocs.ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("updates membership when peers in the same org are stopped and restarted", func() {
			peer0Org1 := network.Peer("Org1", "peer0")
			peer1Org1 := network.Peer("Org1", "peer1")

			By("bringing up all peers")
			startPeers(nwprocs, false, peer0Org1, peer1Org1)

			By("creating and joining a channel")
			channelparticipation.JoinOrdererAppChannel(network, "testchannel", orderer, nwprocs.ordererRunner)

			network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1)

			By("verifying peer1Org1 discovers all the peers before testing membership change on it")
			Eventually(nwo.DiscoverPeers(network, peer1Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
				network.DiscoveredPeer(peer0Org1, "_lifecycle"),
				network.DiscoveredPeer(peer1Org1, "_lifecycle"),
			))

			By("verifying membership change on peer1Org1 when an anchor peer in the same org is stopped and restarted")
			expectedMsgFromExpirationCallback := fmt.Sprintf("Do not remove bootstrap or anchor peer endpoint %s from membership", peerEndpoints[peer0Org1.ID()])
			assertPeerMembershipUpdate(network, peer1Org1, []*nwo.Peer{peer0Org1}, nwprocs, expectedMsgFromExpirationCallback)

			By("verifying peer0Org1 discovers all the peers before testing membership change on it")
			Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
				network.DiscoveredPeer(peer0Org1, "_lifecycle"),
				network.DiscoveredPeer(peer1Org1, "_lifecycle"),
			))

			By("verifying membership change on peer0Org1 when a non-anchor peer in the same org is stopped and restarted")
			expectedMsgFromExpirationCallback = fmt.Sprintf("Removing member: Endpoint: %s", peerEndpoints[peer1Org1.ID()])
			assertPeerMembershipUpdate(network, peer0Org1, []*nwo.Peer{peer1Org1}, nwprocs, expectedMsgFromExpirationCallback)
		})

		It("updates peer membership when peers in another org are stopped and restarted", func() {
			peer0Org1, peer1Org1 := network.Peer("Org1", "peer0"), network.Peer("Org1", "peer1")
			peer0Org2, peer1Org2 := network.Peer("Org2", "peer0"), network.Peer("Org2", "peer1")

			By("bringing up all peers")
			startPeers(nwprocs, false, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

			By("creating and joining a channel")
			channelparticipation.JoinOrdererAppChannel(network, "testchannel", orderer, nwprocs.ordererRunner)
			network.JoinChannel(channelName, orderer, peer0Org1, peer1Org1, peer0Org2, peer1Org2)

			By("verifying membership on peer1Org1")
			Eventually(nwo.DiscoverPeers(network, peer1Org1, "User1", "testchannel"), network.EventuallyTimeout).Should(ConsistOf(
				network.DiscoveredPeer(peer0Org1, "_lifecycle"),
				network.DiscoveredPeer(peer1Org1, "_lifecycle"),
				network.DiscoveredPeer(peer0Org2, "_lifecycle"),
				network.DiscoveredPeer(peer1Org2, "_lifecycle"),
			))

			By("stopping anchor peer peer0Org1 to have only one peer in org1")
			stopPeers(nwprocs, peer0Org1)

			By("verifying peer membership update when peers in another org are stopped and restarted")
			expectedMsgFromExpirationCallback := fmt.Sprintf("Do not remove bootstrap or anchor peer endpoint %s from membership", peerEndpoints[peer0Org2.ID()])
			assertPeerMembershipUpdate(network, peer1Org1, []*nwo.Peer{peer0Org2, peer1Org2}, nwprocs, expectedMsgFromExpirationCallback)
		})
	})

	It("updates membership for a peer with a renewed certificate", func() {
		network.Bootstrap()
		orderer := network.Orderer("orderer")
		nwprocs.ordererRunner = network.OrdererRunner(orderer)
		nwprocs.ordererProcess = ifrit.Invoke(nwprocs.ordererRunner)
		Eventually(nwprocs.ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peer0Org1 := network.Peer("Org1", "peer0")
		peer0Org2 := network.Peer("Org2", "peer0")

		By("bringing up a peer in each organization")
		startPeers(nwprocs, false, peer0Org1, peer0Org2)

		channelparticipation.JoinOrdererAppChannel(network, "testchannel", orderer, nwprocs.ordererRunner)

		By("joining peers to channel")
		network.JoinChannel(channelName, orderer, peer0Org1, peer0Org2)

		By("verifying membership of both peers")
		Eventually(nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"), 50*time.Second, 100*time.Millisecond).Should(ContainElements(network.DiscoveredPeer(peer0Org2, "_lifecycle")))

		By("stopping, renewing peer0Org2 certificate before expiration, and restarting")
		stopPeers(nwprocs, peer0Org2)
		renewPeerCertificate(network, peer0Org2, time.Now().Add(time.Minute))
		startPeers(nwprocs, false, peer0Org2)

		By("verifying membership after cert renewed")
		peer0Org1Runner := nwprocs.peerRunners[peer0Org1.ID()]
		Eventually(peer0Org1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Membership view has changed. peers went online"))
		/*
			// TODO - Replace membership log check with membership discovery check (not currently working since renewed cert signature doesn't always match expectations even though it is forced to be Low-S)
			Eventually(
				nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"),
				60*time.Second,
				100*time.Millisecond).
				Should(ContainElements(network.DiscoveredPeer(network.Peer("Org2", "peer0"), "_lifecycle")))
		*/

		By("waiting for cert to expire within a minute")
		Eventually(peer0Org1Runner.Err(), time.Minute*2).Should(gbytes.Say("gossipping peer identity expired"))

		By("stopping, renewing peer0Org2 certificate again after its expiration, restarting")
		stopPeers(nwprocs, peer0Org2)
		renewPeerCertificate(network, peer0Org2, time.Now().Add(time.Hour))
		startPeers(nwprocs, false, peer0Org2)

		By("verifying membership after cert expired and renewed again")
		Eventually(peer0Org1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Membership view has changed. peers went online"))

		/*
			// TODO - Replace membership log check with membership discovery check (not currently working since renewed cert signature doesn't always match expectations even though it is forced to be Low-S)
			Eventually(
				nwo.DiscoverPeers(network, peer0Org1, "User1", "testchannel"),
				60*time.Second,
				100*time.Millisecond).
				Should(ContainElements(network.DiscoveredPeer(network.Peer("Org2", "peer0"), "_lifecycle")))
		*/
	})
})

// renewPeerCertificate renews the certificate with a given expirationTime and re-writes it to the peer's signcert directory
func renewPeerCertificate(network *nwo.Network, peer *nwo.Peer, expirationTime time.Time) {
	peerDomain := network.Organization(peer.Organization).Domain

	peerCAKeyPath := filepath.Join(network.RootDir, "crypto", "peerOrganizations", peerDomain, "ca", "priv_sk")
	peerCAKey, err := os.ReadFile(peerCAKeyPath)
	Expect(err).NotTo(HaveOccurred())

	peerCACertPath := filepath.Join(network.RootDir, "crypto", "peerOrganizations", peerDomain, "ca", fmt.Sprintf("ca.%s-cert.pem", peerDomain))
	peerCACert, err := os.ReadFile(peerCACertPath)
	Expect(err).NotTo(HaveOccurred())

	peerCertPath := filepath.Join(network.PeerLocalMSPDir(peer), "signcerts", fmt.Sprintf("peer0.%s-cert.pem", peerDomain))
	peerCert, err := os.ReadFile(peerCertPath)
	Expect(err).NotTo(HaveOccurred())

	renewedCert, _ := expireCertificate(peerCert, peerCACert, peerCAKey, expirationTime)
	err = os.WriteFile(peerCertPath, renewedCert, 0o600)
	Expect(err).NotTo(HaveOccurred())
}

// expireCertificate re-creates and re-signs a certificate with a new expirationTime
func expireCertificate(certPEM, caCertPEM, caKeyPEM []byte, expirationTime time.Time) (expiredcertPEM []byte, earlyMadeCACertPEM []byte) {
	keyAsDER, _ := pem.Decode(caKeyPEM)
	caKeyWithoutType, err := x509.ParsePKCS8PrivateKey(keyAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())
	caKey := caKeyWithoutType.(*ecdsa.PrivateKey)

	caCertAsDER, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	certAsDER, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(certAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	cert.Raw = nil
	caCert.Raw = nil
	// The certificate was made 1 minute ago (1 hour doesn't work since cert will be before original CA cert NotBefore time)
	cert.NotBefore = time.Now().Add((-1) * time.Minute)
	// As well as the CA certificate
	caCert.NotBefore = time.Now().Add((-1) * time.Minute)
	// The certificate expires now
	cert.NotAfter = expirationTime

	// The CA creates and signs a temporary certificate
	tempCertBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, cert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	// Force the certificate to use Low-S signature to be compatible with the identities that Fabric uses

	// Parse the certificate to extract the TBS (to-be-signed) data
	tempParsedCert, err := x509.ParseCertificate(tempCertBytes)
	Expect(err).NotTo(HaveOccurred())

	// Hash the TBS data
	hash := sha256.Sum256(tempParsedCert.RawTBSCertificate)

	// Sign the hash using forceLowS
	r, s, err := forceLowS(caKey, hash[:])
	Expect(err).NotTo(HaveOccurred())

	// Encode the signature (DER format)
	signature := append(r.Bytes(), s.Bytes()...)

	// Replace the signature in the certificate with the low-s signature
	tempParsedCert.Signature = signature

	// Re-encode the certificate with the low-s signature
	certBytes, err := x509.CreateCertificate(rand.Reader, tempParsedCert, caCert, cert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	// The CA signs its own certificate
	caCertBytes, err := x509.CreateCertificate(rand.Reader, caCert, caCert, caCert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	expiredcertPEM = pem.EncodeToMemory(&pem.Block{Bytes: certBytes, Type: "CERTIFICATE"})
	earlyMadeCACertPEM = pem.EncodeToMemory(&pem.Block{Bytes: caCertBytes, Type: "CERTIFICATE"})
	return
}

// forceLowS ensures the ECDSA signature's S value is low
func forceLowS(priv *ecdsa.PrivateKey, hash []byte) (r, s *big.Int, err error) {
	r, s, err = ecdsa.Sign(rand.Reader, priv, hash)
	Expect(err).NotTo(HaveOccurred())

	curveOrder := priv.Curve.Params().N
	halfOrder := new(big.Int).Rsh(curveOrder, 1) // curveOrder / 2

	// If s is greater than half the order, replace it with curveOrder - s
	if s.Cmp(halfOrder) > 0 {
		s.Sub(curveOrder, s)
	}

	return r, s, nil
}

func runTransactions(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, chaincodeName string, channelID string) {
	for i := 0; i < 5; i++ {
		sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
			ChannelID: channelID,
			Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
			Name:      chaincodeName,
			Ctor:      `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: []string{
				n.PeerAddress(peer, nwo.ListenPort),
			},
			WaitForEvent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
	}
}

// networkProcesses holds references to the network, its runners, and processes.
type networkProcesses struct {
	network *nwo.Network

	ordererRunner  *ginkgomon.Runner
	ordererProcess ifrit.Process

	peerRunners   map[string]*ginkgomon.Runner
	peerProcesses map[string]ifrit.Process
}

func (n *networkProcesses) terminateAll() {
	if n.ordererProcess != nil {
		n.ordererProcess.Signal(syscall.SIGTERM)
		Eventually(n.ordererProcess.Wait(), n.network.EventuallyTimeout).Should(Receive())
	}
	for _, process := range n.peerProcesses {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), n.network.EventuallyTimeout).Should(Receive())
	}
}

func startPeers(n *networkProcesses, forceStateTransfer bool, peersToStart ...*nwo.Peer) {
	env := []string{"FABRIC_LOGGING_SPEC=info:gossip.state=debug:gossip.discovery=debug"}

	// Setting CORE_PEER_GOSSIP_STATE_CHECKINTERVAL to 200ms (from default of 10s) will ensure that state transfer happens quickly,
	// before blocks are gossipped through normal mechanisms
	if forceStateTransfer {
		env = append(env, "CORE_PEER_GOSSIP_STATE_CHECKINTERVAL=200ms")
	}

	for _, peer := range peersToStart {
		runner := n.network.PeerRunner(peer, env...)
		process := ifrit.Invoke(runner)
		Eventually(process.Ready(), n.network.EventuallyTimeout).Should(BeClosed())

		n.peerProcesses[peer.ID()] = process
		n.peerRunners[peer.ID()] = runner
	}
}

func stopPeers(n *networkProcesses, peersToStop ...*nwo.Peer) {
	for _, peer := range peersToStop {
		id := peer.ID()
		proc := n.peerProcesses[id]
		proc.Signal(syscall.SIGTERM)
		Eventually(proc.Wait(), n.network.EventuallyTimeout).Should(Receive())
		delete(n.peerProcesses, id)
	}
}

func assertPeersLedgerHeight(n *nwo.Network, peersToSyncUp []*nwo.Peer, expectedVal int, channelID string) {
	for _, peer := range peersToSyncUp {
		Eventually(func() int {
			return nwo.GetLedgerHeight(n, peer, channelID)
		}, n.EventuallyTimeout).Should(Equal(expectedVal))
	}
}

// send transactions, stop orderering server, then start peers to ensure they received blcoks via state transfer
func sendTransactionsAndSyncUpPeers(n *networkProcesses, orderer *nwo.Orderer, basePeer *nwo.Peer, channelName string, peersToSyncUp ...*nwo.Peer) {
	By("creating transactions")
	runTransactions(n.network, orderer, basePeer, "mycc", channelName)
	basePeerLedgerHeight := nwo.GetLedgerHeight(n.network, basePeer, channelName)

	By("stopping orderer")
	n.ordererProcess.Signal(syscall.SIGTERM)
	Eventually(n.ordererProcess.Wait(), n.network.EventuallyTimeout).Should(Receive())
	n.ordererProcess = nil

	By("starting the peers contained in the peersToSyncUp list")
	startPeers(n, true, peersToSyncUp...)

	By("ensuring the peers are synced up")
	assertPeersLedgerHeight(n.network, peersToSyncUp, basePeerLedgerHeight, channelName)

	By("restarting orderer")
	n.ordererRunner = n.network.OrdererRunner(orderer)
	n.ordererProcess = ifrit.Invoke(n.ordererRunner)
	Eventually(n.ordererProcess.Ready(), n.network.EventuallyTimeout).Should(BeClosed())
	Eventually(n.ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: [0-9] -> "))
}

// assertPeerMembershipUpdate stops and restart peersToRestart and verify peer membership
func assertPeerMembershipUpdate(network *nwo.Network, peer *nwo.Peer, peersToRestart []*nwo.Peer, nwprocs *networkProcesses, expectedMsgFromExpirationCallback string) {
	stopPeers(nwprocs, peersToRestart...)

	// timeout is the same amount of time as it takes to remove a message from the aliveMsgStore, and add a second as buffer
	core := network.ReadPeerConfig(peer)
	timeout := core.Peer.Gossip.AliveExpirationTimeout*time.Duration(core.Peer.Gossip.MsgExpirationFactor) + time.Second
	By("verifying peer membership after all other peers are stopped")
	Eventually(nwo.DiscoverPeers(network, peer, "User1", "testchannel"), timeout, 100*time.Millisecond).Should(ConsistOf(
		network.DiscoveredPeer(peer, "_lifecycle"),
	))

	By("verifying expected log message from expiration callback")
	runner := nwprocs.peerRunners[peer.ID()]
	Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say(expectedMsgFromExpirationCallback))

	By("restarting peers")
	startPeers(nwprocs, false, peersToRestart...)

	By("verifying peer membership, expected to discover restarted peers")
	expectedPeers := make([]nwo.DiscoveredPeer, len(peersToRestart)+1)
	expectedPeers[0] = network.DiscoveredPeer(peer, "_lifecycle")
	for i, p := range peersToRestart {
		expectedPeers[i+1] = network.DiscoveredPeer(p, "_lifecycle")
	}
	timeout = 3 * core.Peer.Gossip.ReconnectInterval
	Eventually(nwo.DiscoverPeers(network, peer, "User1", "testchannel"), timeout, 100*time.Millisecond).Should(ConsistOf(expectedPeers))
}
