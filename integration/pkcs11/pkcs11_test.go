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
	"syscall"
	"time"

	"github.com/hyperledger/fabric/integration/channelparticipation"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"

	bpkcs11 "github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/miekg/pkcs11"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("PKCS11 enabled network", func() {
	var (
		tempDir                     string
		network                     *nwo.Network
		chaincode                   nwo.Chaincode
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "p11")
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), tempDir, nil, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(tempDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_prebuilt_chaincode",
		}
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		network.Cleanup()
		os.RemoveAll(tempDir)
	})

	Describe("without mapping", func() {
		BeforeEach(func() {
			By("configuring PKCS11 artifacts")
			setupPKCS11(network, noMapping)

			By("starting fabric processes")
			ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
		})

		It("executes transactions against a basic etcdraft network", func() {
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.PeersWithChannel("testchannel")...)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			runQueryInvokeQuery(network, orderer, network.Peer("Org1", "peer0"), "testchannel")
		})
	})

	Describe("mapping everything", func() {
		BeforeEach(func() {
			By("configuring PKCS11 artifacts")
			setupPKCS11(network, mapAll)

			By("starting fabric processes")
			ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
		})

		It("executes transactions against a basic etcdraft network", func() {
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.PeersWithChannel("testchannel")...)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			runQueryInvokeQuery(network, orderer, network.Peer("Org1", "peer0"), "testchannel")
		})
	})
})

type model uint8

const (
	noMapping = model(iota)
	mapAll
)

func setupPKCS11(network *nwo.Network, model model) {
	lib, pin, label := bpkcs11.FindPKCS11Lib()

	By("establishing a PKCS11 session")
	ctx, sess := setupPKCS11Ctx(lib, label, pin)
	defer ctx.Destroy()
	defer ctx.CloseSession(sess)

	serialNumbers := map[string]*big.Int{}
	configurePeerPKCS11(ctx, sess, network, serialNumbers)
	configureOrdererPKCS11(ctx, sess, network, serialNumbers)

	var keyConfig []fabricconfig.KeyIDMapping
	switch model {
	case noMapping:
	case mapAll:
		updateKeyIdentifiers(ctx, sess, serialNumbers)
		for ski, serial := range serialNumbers {
			keyConfig = append(keyConfig, fabricconfig.KeyIDMapping{
				SKI: ski,
				ID:  serial.String(),
			})
		}
	}

	bccspConfig := &fabricconfig.BCCSP{
		Default: "PKCS11",
		PKCS11: &fabricconfig.PKCS11{
			Security: 256,
			Hash:     "SHA2",
			Pin:      pin,
			Label:    label,
			Library:  lib,
			KeyIDs:   keyConfig,
		},
	}

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

func configurePeerPKCS11(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, network *nwo.Network, serialNumbers map[string]*big.Int) {
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
		serialNumbers[hex.EncodeToString(skiForKey(peerPubKey))] = peerSerial

		By("Updating the peer admin user signcerts")
		newAdminPemCert := buildCert(caBytes, orgCAPath, adminCSR, adminSerial, adminPubKey)
		orgAdminMSPPath := network.PeerUserMSPDir(peer, "Admin")
		updateMSPFolder(orgAdminMSPPath, fmt.Sprintf("Admin@%s-cert.pem", domain), newAdminPemCert)
		serialNumbers[hex.EncodeToString(skiForKey(adminPubKey))] = adminSerial

		By("Updating the peer user1 signcerts")
		newUserPemCert := buildCert(caBytes, orgCAPath, userCSR, userSerial, userPubKey)
		orgUserMSPPath := network.PeerUserMSPDir(peer, "User1")
		updateMSPFolder(orgUserMSPPath, fmt.Sprintf("User1@%s-cert.pem", domain), newUserPemCert)
		serialNumbers[hex.EncodeToString(skiForKey(userPubKey))] = userSerial
	}
}

func configureOrdererPKCS11(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, network *nwo.Network, serialNumbers map[string]*big.Int) {
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
	serialNumbers[hex.EncodeToString(skiForKey(ordererPubKey))] = ordererSerial

	By("Updating the orderer admin user signcerts")
	newAdminPemCert := buildCert(caBytes, orgCAPath, adminCSR, adminSerial, adminPubKey)
	orgAdminMSPPath := network.OrdererUserMSPDir(orderer, "Admin")
	updateMSPFolder(orgAdminMSPPath, fmt.Sprintf("Admin@%s-cert.pem", domain), newAdminPemCert)
	serialNumbers[hex.EncodeToString(skiForKey(adminPubKey))] = adminSerial
}

// Creates pkcs11 context and session
func setupPKCS11Ctx(lib, label, pin string) (*pkcs11.Ctx, pkcs11.SessionHandle) {
	ctx := pkcs11.New(lib)

	if err := ctx.Initialize(); err != nil {
		Expect(err).To(Equal(pkcs11.Error(pkcs11.CKR_CRYPTOKI_ALREADY_INITIALIZED)))
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	slot := findPKCS11Slot(ctx, label)
	Expect(slot).Should(BeNumerically(">", 0), "Could not find slot with label %s", label)

	sess, err := ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
	Expect(err).NotTo(HaveOccurred())

	// Login
	err = ctx.Login(sess, pkcs11.CKU_USER, pin)
	Expect(err).NotTo(HaveOccurred())

	return ctx, sess
}

// Identifies pkcs11 slot using specified label
func findPKCS11Slot(ctx *pkcs11.Ctx, label string) uint {
	slots, err := ctx.GetSlotList(true)
	Expect(err).NotTo(HaveOccurred())

	for _, s := range slots {
		tokInfo, err := ctx.GetTokenInfo(s)
		Expect(err).NotTo(HaveOccurred())

		if tokInfo.Label == label {
			return s
		}
	}

	return 0
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
	err := ioutil.WriteFile(filepath.Join(path, "signcerts", certName), cert, 0o644)
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

	// convert pub key to ansi types
	nistCurve := elliptic.P256()
	x, y := elliptic.Unmarshal(nistCurve, ecpt)
	if x == nil {
		Expect(x).NotTo(BeNil(), "Failed Unmarshalling Public Key")
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

func skiForKey(pk *ecdsa.PublicKey) []byte {
	ski := sha256.Sum256(elliptic.Marshal(pk.Curve, pk.X, pk.Y))
	return ski[:]
}

func updateKeyIdentifiers(pctx *pkcs11.Ctx, sess pkcs11.SessionHandle, serialNumbers map[string]*big.Int) {
	for ks, serial := range serialNumbers {
		ski, err := hex.DecodeString(ks)
		Expect(err).NotTo(HaveOccurred())

		updateKeyIdentifier(pctx, sess, pkcs11.CKO_PUBLIC_KEY, ski, []byte(serial.String()))
		updateKeyIdentifier(pctx, sess, pkcs11.CKO_PRIVATE_KEY, ski, []byte(serial.String()))
	}
}

func updateKeyIdentifier(pctx *pkcs11.Ctx, sess pkcs11.SessionHandle, class uint, currentID, newID []byte) {
	pkt := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, class),
		pkcs11.NewAttribute(pkcs11.CKA_ID, currentID),
	}
	err := pctx.FindObjectsInit(sess, pkt)
	Expect(err).NotTo(HaveOccurred())

	objs, _, err := pctx.FindObjects(sess, 1)
	Expect(err).NotTo(HaveOccurred())
	Expect(objs).To(HaveLen(1))

	err = pctx.FindObjectsFinal(sess)
	Expect(err).NotTo(HaveOccurred())

	err = pctx.SetAttributeValue(sess, objs[0], []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, newID),
	})
	Expect(err).NotTo(HaveOccurred())
}
