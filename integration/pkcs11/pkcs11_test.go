/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/miekg/pkcs11"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("PKCS11 enabled network", func() {
	var (
		tempDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "p11")
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), tempDir, nil, BasePort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		By("configuring PKCS11 artifacts")
		configurePKCS11(network)
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		network.Cleanup()
		os.RemoveAll(tempDir)
	})

	It("executes transactions against a basic solo network using yaml config", func() {
		setPKCS11Config(network, bccspConfig)

		By("starting fabric processes")
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
		}

		orderer := network.Orderer("orderer")
		network.CreateAndJoinChannels(orderer)

		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.PeersWithChannel("testchannel")...)
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
		runQueryInvokeQuery(network, orderer, network.Peer("Org1", "peer0"), "testchannel")
	})

	It("executes transactions against a basic solo network using environment variable config", func() {
		envCleanup := setPKCS11ConfigWithEnvVariables(network, bccspConfig)
		defer envCleanup()

		By("starting fabric processes")
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
		}

		orderer := network.Orderer("orderer")
		network.CreateAndJoinChannels(orderer)

		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.PeersWithChannel("testchannel")...)
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
		runQueryInvokeQuery(network, orderer, network.Peer("Org1", "peer0"), "testchannel")
	})
})

func configurePKCS11(network *nwo.Network) {
	configurePeerPKCS11(ctx, sess, network)
	configureOrdererPKCS11(ctx, sess, network)
}

func setPKCS11Config(network *nwo.Network, bccspConfig *fabricconfig.BCCSP) {
	By("updating bccsp peer config")
	for _, peer := range network.Peers {
		peerConfig := network.ReadPeerConfig(peer)
		peerConfig.Peer.BCCSP = bccspConfig
		network.WritePeerConfig(peer, peerConfig)
	}

	By("updating bccsp orderer config")
	orderer := network.Orderer("orderer")
	ordererConfig := network.ReadOrdererConfig(orderer)
	ordererConfig.General.BCCSP = bccspConfig
	network.WriteOrdererConfig(orderer, ordererConfig)
}

func setPKCS11ConfigWithEnvVariables(network *nwo.Network, bccspConfig *fabricconfig.BCCSP) (cleanup func()) {
	By("setting bccsp peer config via environment variables")
	os.Setenv("CORE_PEER_BCCSP_DEFAULT", bccspConfig.Default)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_SECURITY", strconv.Itoa(bccspConfig.PKCS11.Security))
	os.Setenv("CORE_PEER_BCCSP_PKCS11_HASH", bccspConfig.PKCS11.Hash)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_PIN", bccspConfig.PKCS11.Pin)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LABEL", bccspConfig.PKCS11.Label)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LIBRARY", bccspConfig.PKCS11.Library)

	By("setting bccsp orderer config via environment variables")
	os.Setenv("ORDERER_GENERAL_BCCSP_DEFAULT", bccspConfig.Default)
	os.Setenv("ORDERER_GENERAL_BCCSP_PKCS11_SECURITY", strconv.Itoa(bccspConfig.PKCS11.Security))
	os.Setenv("ORDERER_GENERAL_BCCSP_PKCS11_HASH", bccspConfig.PKCS11.Hash)
	os.Setenv("ORDERER_GENERAL_BCCSP_PKCS11_PIN", bccspConfig.PKCS11.Pin)
	os.Setenv("ORDERER_GENERAL_BCCSP_PKCS11_LABEL", bccspConfig.PKCS11.Label)
	os.Setenv("ORDERER_GENERAL_BCCSP_PKCS11_LIBRARY", bccspConfig.PKCS11.Library)

	return func() {
		os.Unsetenv("CORE_PEER_BCCSP_DEFAULT")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_SECURITY")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_HASH")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_PIN")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LABEL")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LIBRARY")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_DEFAULT")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_PKCS11_SECURITY")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_PKCS11_HASH")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_PKCS11_PIN")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_PKCS11_LABEL")
		os.Unsetenv("ORDERER_GENERAL_BCCSP_PKCS11_LIBRARY")
	}
}

func configurePeerPKCS11(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, network *nwo.Network) {
	for _, peer := range network.Peers {
		orgName := peer.Organization

		peerPubKey, peerCSR, peerSerial := createCSR(ctx, sess, orgName, "peer")
		adminPubKey, adminCSR, adminSerial := createCSR(ctx, sess, orgName, "admin")
		userPubKey, userCSR, userSerial := createCSR(ctx, sess, orgName, "client")

		domain := network.Organization(orgName).Domain

		// Retrieves org CA cert
		orgCAPath := network.PeerOrgCADir(network.Organization(orgName))
		caBytes, err := ioutil.ReadFile(filepath.Join(orgCAPath, fmt.Sprintf("ca.%s-cert.pem", domain)))
		Expect(err).NotTo(HaveOccurred())

		By("Updating the peer signcerts")
		newOrdererPemCert := buildCert(caBytes, orgCAPath, peerCSR, peerSerial, peerPubKey)
		updateMSPFolder(network.PeerLocalMSPDir(peer), fmt.Sprintf("peer.%s-cert.pem", domain), newOrdererPemCert)

		By("Updating the peer admin user signcerts")
		newAdminPemCert := buildCert(caBytes, orgCAPath, adminCSR, adminSerial, adminPubKey)
		orgAdminMSPPath := network.PeerUserMSPDir(peer, "Admin")
		updateMSPFolder(orgAdminMSPPath, fmt.Sprintf("Admin@%s-cert.pem", domain), newAdminPemCert)

		By("Updating the peer user1 signcerts")
		newUserPemCert := buildCert(caBytes, orgCAPath, userCSR, userSerial, userPubKey)
		orgUserMSPPath := network.PeerUserMSPDir(peer, "User1")
		updateMSPFolder(orgUserMSPPath, fmt.Sprintf("User1@%s-cert.pem", domain), newUserPemCert)
	}
}

func configureOrdererPKCS11(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, network *nwo.Network) {
	orderer := network.Orderer("orderer")
	orgName := orderer.Organization
	domain := network.Organization(orgName).Domain

	ordererPubKey, ordererCSR, ordererSerial := createCSR(ctx, sess, orgName, "orderer")
	adminPubKey, adminCSR, adminSerial := createCSR(ctx, sess, orgName, "admin")

	// Retrieves org CA cert
	orgCAPath := network.OrdererOrgCADir(network.Organization(orgName))
	caBytes, err := ioutil.ReadFile(filepath.Join(orgCAPath, fmt.Sprintf("ca.%s-cert.pem", domain)))
	Expect(err).NotTo(HaveOccurred())

	By("Updating the orderer signcerts")
	newOrdererPemCert := buildCert(caBytes, orgCAPath, ordererCSR, ordererSerial, ordererPubKey)
	updateMSPFolder(network.OrdererLocalMSPDir(orderer), fmt.Sprintf("orderer.%s-cert.pem", domain), newOrdererPemCert)

	By("Updating the orderer admin user signcerts")
	newAdminPemCert := buildCert(caBytes, orgCAPath, adminCSR, adminSerial, adminPubKey)
	orgAdminMSPPath := network.OrdererUserMSPDir(orderer, "Admin")
	updateMSPFolder(orgAdminMSPPath, fmt.Sprintf("Admin@%s-cert.pem", domain), newAdminPemCert)
}

// Creates CSR for provided organization and organizational unit
func createCSR(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, org, ou string) (*ecdsa.PublicKey, *x509.CertificateRequest, *big.Int) {
	pubKey, pkcs11Key := generateKeyPair(ctx, sess)

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"San Francisco"},
			Organization:       []string{fmt.Sprintf("%s.example.com", org)},
			OrganizationalUnit: []string{ou},
			CommonName:         fmt.Sprintf("peer.%s.example.com", org),
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, pkcs11Key)
	Expect(err).NotTo(HaveOccurred())

	csr, err := x509.ParseCertificateRequest(csrBytes)
	Expect(err).NotTo(HaveOccurred())
	err = csr.CheckSignature()
	Expect(err).NotTo(HaveOccurred())

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Expect(err).NotTo(HaveOccurred())

	return pubKey, csr, serialNumber
}

func buildCert(caBytes []byte, org1CAPath string, csr *x509.CertificateRequest, serialNumber *big.Int, pubKey *ecdsa.PublicKey) []byte {
	pemBlock, _ := pem.Decode(caBytes)
	Expect(pemBlock).NotTo(BeNil())

	caCert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())

	keyBytes, err := ioutil.ReadFile(filepath.Join(org1CAPath, "priv_sk"))
	Expect(err).NotTo(HaveOccurred())

	pemBlock, _ = pem.Decode(keyBytes)
	Expect(pemBlock).NotTo(BeNil())
	key, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	caKey := key.(*ecdsa.PrivateKey)

	certTemplate := &x509.Certificate{
		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,
		PublicKey:          csr.PublicKey,
		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,

		SerialNumber:          serialNumber,
		NotBefore:             time.Now().Add(-1 * time.Minute).UTC(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour).UTC(),
		BasicConstraintsValid: true,

		Subject:     csr.Subject,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{},
	}

	// Use root CA to create and sign cert
	signedCert, err := x509.CreateCertificate(rand.Reader, certTemplate, caCert, pubKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	return pem.EncodeToMemory(&pem.Block{Bytes: signedCert, Type: "CERTIFICATE"})
}

// Overwrites existing cert and removes private key from keystore folder
func updateMSPFolder(path, certName string, cert []byte) {
	// Overwrite existing certificate with new certificate
	err := ioutil.WriteFile(filepath.Join(path, "signcerts", certName), cert, 0644)
	Expect(err).NotTo(HaveOccurred())

	// delete the existing private key - this is stored in the hsm
	adminKSCert := filepath.Join(path, "keystore", "priv_sk")
	err = os.Remove(adminKSCert)
	Expect(err).NotTo(HaveOccurred())
}

// Generating key pair in HSM, convert, and return keys
func generateKeyPair(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle) (*ecdsa.PublicKey, *P11ECDSAKey) {
	publabel, privlabel := "BCPUB7", "BCPRV7"

	curve := asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7} // secp256r1 Curve

	marshaledOID, err := asn1.Marshal(curve)
	Expect(err).NotTo(HaveOccurred())

	pubAttrs := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
		pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, marshaledOID),

		pkcs11.NewAttribute(pkcs11.CKA_ID, publabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, publabel),
	}

	privAttrs := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
		pkcs11.NewAttribute(pkcs11.CKA_SIGN, true),

		pkcs11.NewAttribute(pkcs11.CKA_ID, privlabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, privlabel),

		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false),
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true),
	}

	pubK, privK, err := ctx.GenerateKeyPair(
		sess,
		[]*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_EC_KEY_PAIR_GEN, nil)},
		pubAttrs,
		privAttrs,
	)
	Expect(err).NotTo(HaveOccurred())

	ecpt := ecPoint(ctx, sess, pubK)
	Expect(ecpt).NotTo(BeEmpty(), "CKA_EC_POINT not found")

	hash := sha256.Sum256(ecpt)
	ski := hash[:]

	setskiT := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, hex.EncodeToString(ski)),
	}

	err = ctx.SetAttributeValue(sess, pubK, setskiT)
	Expect(err).NotTo(HaveOccurred())

	err = ctx.SetAttributeValue(sess, privK, setskiT)
	Expect(err).NotTo(HaveOccurred())

	// convert pub key to rsa types
	nistCurve := elliptic.P256()
	x, y := elliptic.Unmarshal(nistCurve, ecpt)
	if x == nil {
		Expect(x).NotTo(BeNil(), "Failed Unmarshaling Public Key")
	}

	pubKey := &ecdsa.PublicKey{Curve: nistCurve, X: x, Y: y}

	pkcs11Key := &P11ECDSAKey{
		ctx:              ctx,
		session:          sess,
		publicKey:        pubKey,
		privateKeyHandle: privK,
	}

	return pubKey, pkcs11Key
}

// SoftHSM reports extra two bytes before the uncompressed point
// see /bccsp/pkcs11/pkcs11.go::ecPoint() for additional details
func ecPoint(pkcs11lib *pkcs11.Ctx, session pkcs11.SessionHandle, key pkcs11.ObjectHandle) (ecpt []byte) {
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, nil),
	}

	attr, err := pkcs11lib.GetAttributeValue(session, key, template)
	if err != nil {
		Expect(err).NotTo(HaveOccurred(), "PKCS11: get(EC point)")
	}

	for _, a := range attr {
		if a.Type != pkcs11.CKA_EC_POINT {
			continue
		}

		switch {
		case ((len(a.Value) % 2) == 0) && (byte(0x04) == a.Value[0]) && (byte(0x04) == a.Value[len(a.Value)-1]):
			ecpt = a.Value[0 : len(a.Value)-1] // Trim trailing 0x04
		case byte(0x04) == a.Value[0] && byte(0x04) == a.Value[2]:
			ecpt = a.Value[2:len(a.Value)]
		default:
			ecpt = a.Value
		}
	}

	return ecpt
}
