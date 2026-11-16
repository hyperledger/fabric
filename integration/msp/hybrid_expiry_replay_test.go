/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/signer"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	dcli "github.com/moby/moby/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Pactway hybrid transaction expiry replay", func() {
	It("replays a valid transaction after its client certificate expires", func() {
		testDir, err := os.MkdirTemp("", "hybrid-expiry-replay")
		Expect(err).NotTo(HaveOccurred())
		oldProfile, oldProfileSet := os.LookupEnv("PACTWAY_HYBRID_PROFILE")
		oldDevelopment, oldDevelopmentSet := os.LookupEnv("PACTWAY_ISOLATED_DEVELOPMENT")
		Expect(os.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)).To(Succeed())
		Expect(os.Setenv("PACTWAY_ISOLATED_DEVELOPMENT", "true")).To(Succeed())

		var network *nwo.Network
		var ordererProcess, peerProcess, replayPeerProcess ifrit.Process
		DeferCleanup(func() {
			for _, process := range []ifrit.Process{replayPeerProcess, peerProcess, ordererProcess} {
				if process != nil {
					process.Signal(syscall.SIGTERM)
					Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
			if network != nil {
				network.Cleanup()
			}
			Expect(os.RemoveAll(testDir)).To(Succeed())
			restoreEnvironment("PACTWAY_HYBRID_PROFILE", oldProfile, oldProfileSet)
			restoreEnvironment("PACTWAY_ISOLATED_DEVELOPMENT", oldDevelopment, oldDevelopmentSet)
		})

		client, err := dcli.New(dcli.FromEnv)
		Expect(err).NotTo(HaveOccurred())
		network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, StartPort(), components)
		ordererOrg := network.Organization("OrdererOrg")
		ordererOrg.MSPID = "OrdererOrgMSP"
		ordererOrg.EnableNodeOUs = true
		network.Profiles[0].ChannelCapabilities = []string{"V3_0", capabilities.PactwayHybridCertificateV1}
		network.GenerateConfigTree()
		network.CreateDockerNetwork()
		sess, err := network.Cryptogen(commands.Generate{Config: network.CryptoConfigPath(), Output: network.CryptoPath()})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

		configDir, err := filepath.Abs(filepath.Join("..", "..", "sampleconfig"))
		Expect(err).NotTo(HaveOccurred())
		output := filepath.Join(testDir, "hybrid-profile")
		generateHybridNetwork(output, configDir)
		installHybridNetworkMSPs(network, output)
		for _, channel := range network.Channels {
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID: channel.Name, Profile: channel.Profile, ConfigPath: network.RootDir,
				OutputBlock: network.OutputBlockPath(channel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		}
		network.ConcatenateTLSCACertificates()

		orderer := network.Orderer("orderer")
		org1Peer := network.Peer("Org1", "peer0")
		org2Peer := network.Peer("Org2", "peer0")
		runner, startedOrderer, startedPeers := network.StartSingleOrdererNetwork(orderer.Name)
		ordererProcess, peerProcess = startedOrderer, startedPeers
		nwo.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, runner)
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer, org2Peer)

		chaincode := nwo.Chaincode{
			Name:            "hybrid-expiry-cc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "hybrid-expiry-cc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `OR ('Org1MSP.peer')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "hybrid_expiry_cc",
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		replayHeight := nwo.GetLedgerHeight(network, org2Peer, "testchannel")
		peerProcess.Signal(syscall.SIGTERM)
		Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		peerProcess = nil
		peerProcess = ifrit.Invoke(network.PeerRunner(org1Peer))
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		userMSPDir := filepath.Join(testDir, "expiring-user-msp")
		intermediateCertificate := filepath.Join(output, "public", "organizations", "Org1", "msp", "intermediatecerts", "intermediate.pem")
		intermediateKeyStore := filepath.Join(output, "private", "ca", "Org1", "intermediate", "keystore")
		notAfter := generateExpiringClientMSP(
			filepath.Join(output, "private", "organizations", "Org1", "users", "Client", "msp"),
			intermediateCertificate, intermediateKeyStore, userMSPDir, 2*time.Minute,
		)
		Expect(installMSPKeepingTLS(userMSPDir, network.PeerUserMSPDir(org1Peer, "User1"))).To(Succeed())
		invoke, err := network.PeerUserSession(org1Peer, "User1", commands.ChaincodeInvoke{
			ChannelID: "testchannel", Orderer: network.OrdererAddress(orderer, nwo.ListenPort),
			Name: chaincode.Name, Ctor: `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: []string{network.PeerAddress(org1Peer, nwo.ListenPort)},
			WaitForEvent:  true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(invoke, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(invoke.Err).To(gbytes.Say(`\Qcommitted with status (VALID)\E`))
		Expect(invoke.Err).To(gbytes.Say("Chaincode invoke successful"))
		height := nwo.GetLedgerHeight(network, org1Peer, "testchannel")
		Expect(height).To(BeNumerically(">", replayHeight))
		Expect(time.Now()).To(BeTemporally("<", notAfter))

		peerProcess.Signal(syscall.SIGTERM)
		Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		peerProcess = nil
		time.Sleep(time.Until(notAfter) + time.Second)
		Expect(time.Now()).To(BeTemporally(">", notAfter))

		replayPeerProcess = ifrit.Invoke(network.PeerRunner(org2Peer))
		Eventually(replayPeerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		Eventually(func() int {
			return nwo.GetLedgerHeight(network, org2Peer, "testchannel")
		}, network.EventuallyTimeout).Should(BeNumerically(">=", height))
		query, err := network.PeerUserSession(org2Peer, "Admin", commands.ChaincodeQuery{
			ChannelID: "testchannel", Name: chaincode.Name, Ctor: `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(query, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(strings.TrimSpace(string(query.Out.Contents()))).To(Equal("90"))
	})
})

type replayCertificateSigner struct {
	csp       bccsp.BCCSP
	key       bccsp.Key
	publicKey oqs.PublicKey
}

func (s replayCertificateSigner) PublicKey() oqs.PublicKey { return s.publicKey }
func (s replayCertificateSigner) SignHybrid(message []byte) ([]byte, error) {
	return s.csp.Sign(s.key, message, nil)
}

func replayPQSigner(csp bccsp.BCCSP, key bccsp.Key) replayCertificateSigner {
	publicKey, err := key.PublicKey()
	Expect(err).NotTo(HaveOccurred())
	publicDER, err := publicKey.Bytes()
	Expect(err).NotTo(HaveOccurred())
	parsed, err := oqs.ParsePKIXPublicKey(publicDER)
	Expect(err).NotTo(HaveOccurred())
	pqPublic, ok := parsed.(*oqs.PublicKey)
	Expect(ok).To(BeTrue())
	return replayCertificateSigner{csp: csp, key: key, publicKey: *pqPublic}
}

func replayPrivateKey(csp bccsp.BCCSP, keyStoreDir string, publicDER []byte) bccsp.Key {
	files, err := os.ReadDir(keyStoreDir)
	Expect(err).NotTo(HaveOccurred())
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), "_sk") {
			continue
		}
		ski, err := hex.DecodeString(strings.TrimSuffix(file.Name(), "_sk"))
		Expect(err).NotTo(HaveOccurred())
		key, err := csp.GetKey(ski)
		Expect(err).NotTo(HaveOccurred())
		publicKey, err := key.PublicKey()
		Expect(err).NotTo(HaveOccurred())
		keyDER, err := publicKey.Bytes()
		Expect(err).NotTo(HaveOccurred())
		if bytes.Equal(keyDER, publicDER) {
			return key
		}
	}
	Fail("no persisted private key matches the certificate public key")
	return nil
}

func generateExpiringClientMSP(source, certificatePath, caKeyStoreDir, destination string, validity time.Duration) time.Time {
	Expect(copyDirectory(source, destination)).To(Succeed())
	keyStoreDir := filepath.Join(destination, "keystore")
	Expect(os.RemoveAll(keyStoreDir)).To(Succeed())
	Expect(os.MkdirAll(keyStoreDir, 0o700)).To(Succeed())
	keyStore, err := sw.NewFileBasedKeyStore(nil, keyStoreDir, false)
	Expect(err).NotTo(HaveOccurred())
	csp, err := sw.NewDefaultSecurityLevelWithKeystore(keyStore)
	Expect(err).NotTo(HaveOccurred())
	caKeyStore, err := sw.NewFileBasedKeyStore(nil, caKeyStoreDir, true)
	Expect(err).NotTo(HaveOccurred())
	caCSP, err := sw.NewDefaultSecurityLevelWithKeystore(caKeyStore)
	Expect(err).NotTo(HaveOccurred())

	certificatePEM, err := os.ReadFile(certificatePath)
	Expect(err).NotTo(HaveOccurred())
	block, _ := pem.Decode(certificatePEM)
	Expect(block).NotTo(BeNil())
	intermediate, err := x509.ParseCertificate(block.Bytes)
	Expect(err).NotTo(HaveOccurred())
	profile, err := oqs.HybridCertificateProfileFromEnvironment()
	Expect(err).NotTo(HaveOccurred())
	hybridInfo, found, err := oqs.ParseHybridCertificate(intermediate, profile)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())

	caKey := replayPrivateKey(caCSP, caKeyStoreDir, intermediate.RawSubjectPublicKeyInfo)
	caSigner, err := signer.New(caCSP, caKey)
	Expect(err).NotTo(HaveOccurred())
	caPQDER, err := oqs.MarshalPKIXPublicKey(&hybridInfo.SubjectPublicKey)
	Expect(err).NotTo(HaveOccurred())
	caPQSigner := replayPQSigner(caCSP, replayPrivateKey(caCSP, caKeyStoreDir, caPQDER))
	key, err := csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	Expect(err).NotTo(HaveOccurred())
	publicKey, err := key.PublicKey()
	Expect(err).NotTo(HaveOccurred())
	publicDER, err := publicKey.Bytes()
	Expect(err).NotTo(HaveOccurred())
	public, err := x509.ParsePKIXPublicKey(publicDER)
	Expect(err).NotTo(HaveOccurred())
	pqKey, err := csp.KeyGen(&bccsp.OQSKeyGenOpts{Temporary: false, SignatureScheme: string(oqs.MLDSA65ProfileV1)})
	Expect(err).NotTo(HaveOccurred())

	now := time.Now()
	notAfter := now.Add(validity).Truncate(time.Second)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	Expect(err).NotTo(HaveOccurred())
	subjectKeyID := make([]byte, 20)
	_, err = rand.Read(subjectKeyID)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "expiring-client-org1", OrganizationalUnit: []string{"client"}},
		NotBefore: now.Add(-time.Minute), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true,
		SubjectKeyId: subjectKeyID, AuthorityKeyId: intermediate.SubjectKeyId,
	}
	der, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, template, intermediate, public, caSigner, replayPQSigner(csp, pqKey).PublicKey(), caPQSigner,
	)
	Expect(err).NotTo(HaveOccurred())
	signcerts := filepath.Join(destination, "signcerts")
	Expect(os.RemoveAll(signcerts)).To(Succeed())
	Expect(os.MkdirAll(signcerts, 0o700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(signcerts, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)).To(Succeed())
	return notAfter
}
