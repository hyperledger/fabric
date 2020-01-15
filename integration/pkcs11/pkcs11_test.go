// +build pkcs11

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

	bpkcs11 "github.com/hyperledger/fabric/bccsp/pkcs11"

	"github.com/miekg/pkcs11"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configure pkcs11 for peer", func() {
	var (
		tempDir string
		network *nwo.Network
		client  *docker.Client
		process ifrit.Process
		peer0   *nwo.Peer
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "p11")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), tempDir, client, StartPort(), components)
		network.GenerateConfigTree()

		peer0 = network.Peers[0]

		By("configuring PKCS11")
		lib, pin, label := bpkcs11.FindPKCS11Lib()
		myBCCSP := &fabricconfig.BCCSP{
			Default: "PKCS11",
			PKCS11: &fabricconfig.PKCS11{
				Security: 256,
				Hash:     "SHA2",
				Pin:      pin,
				Label:    label,
				Library:  lib,
			},
		}

		peerConfig0 := network.ReadPeerConfig(peer0)
		peerConfig0.Peer.BCCSP = myBCCSP
		network.WritePeerConfig(peer0, peerConfig0)

		ctx, sess, err := setupPKCS11Ctx(lib, label, pin)
		Expect(err).NotTo(HaveOccurred())
		configurePKCS11(ctx, sess, network)

		By("Starting fabric processes")
		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), time.Second*5).Should(BeClosed())

	})

	AfterEach(func() {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(tempDir)
	})

	It("invokes a chaincode on peer", func() {

		// channel := "testchannel"

		// orderer := network.Orderer("orderer")

		// network.CreateAndJoinChannels(orderer)
	})

})

// Create pkcs11 context and session
func setupPKCS11Ctx(lib, label, pin string) (*pkcs11.Ctx, pkcs11.SessionHandle, error) {

	ctx := pkcs11.New(lib)

	ctx.Initialize()

	slot, err := findPKCS11Slot(ctx, label)
	Expect(err).ToNot(HaveOccurred())

	sess, err := ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
	Expect(err).ToNot(HaveOccurred())

	// Login
	err = ctx.Login(sess, pkcs11.CKU_USER, pin)
	Expect(err).NotTo(HaveOccurred())

	return ctx, sess, err

}

// Identify pkcs11 slot using specified label
func findPKCS11Slot(ctx *pkcs11.Ctx, label string) (uint, error) {

	var (
		slot      uint
		foundSlot bool
	)

	slots, err := ctx.GetSlotList(true)
	Expect(err).ToNot(HaveOccurred())

	for _, s := range slots {
		tokInfo, err := ctx.GetTokenInfo(s)
		Expect(err).ToNot(HaveOccurred())

		if tokInfo.Label == label {
			foundSlot = true
			slot = s
			break
		}
	}

	if !foundSlot {
		Fail("Could not find slot with label")
		return 0, err
	}

	return slot, nil

}

func configurePKCS11(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle, network *nwo.Network) {

	peer0 := network.Peers[0]

	pubKey, pkcs11Key := generateKeyPair(ctx, sess)

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"San Francisco"},
			Organization:       []string{"org1.example.com"},
			OrganizationalUnit: []string{"peer"},
			CommonName:         "peer0.org1.example.com",
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

	network.Bootstrap()

	org1CAPath := network.PeerOrgCADir(network.Organization("Org1"))
	caBytes, err := ioutil.ReadFile(filepath.Join(org1CAPath, "ca.org1.example.com-cert.pem"))
	Expect(err).NotTo(HaveOccurred())

	pemBlock, _ := pem.Decode(caBytes)
	Expect(pemBlock).ToNot(BeNil())

	caCert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())

	keyBytes, err := ioutil.ReadFile(filepath.Join(org1CAPath, "priv_sk"))
	Expect(err).NotTo(HaveOccurred())

	pemBlock, _ = pem.Decode(keyBytes)
	Expect(pemBlock).ToNot(BeNil())
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
	Expect(err).ToNot(HaveOccurred())

	newPemCert := pem.EncodeToMemory(&pem.Block{Bytes: signedCert, Type: "CERTIFICATE"})

	// Overwrite existing certificate with new certificate
	peerSignCertsPath := filepath.Join(network.PeerLocalMSPDir(peer0), "signcerts")
	err = ioutil.WriteFile(fmt.Sprintf("%s/peer0.org1.example.com-cert.pem", peerSignCertsPath), newPemCert, 0644)
	Expect(err).NotTo(HaveOccurred())

	// delete the existing private key - this is stored in the hsm
	peerKSCert := filepath.Join(network.PeerLocalMSPDir(peer0), "keystore", "priv_sk")
	err = os.Remove(peerKSCert)
	Expect(err).NotTo(HaveOccurred())

	ctx.CloseSession(sess)

}

// Generating key pair in HSM, convert, and return keys
func generateKeyPair(ctx *pkcs11.Ctx, sess pkcs11.SessionHandle) (*ecdsa.PublicKey, *P11ECDSAKey) {
	publabel, privlabel := "BCPUB7", "BCPRV7"

	curve := asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7} // secp256r1 Curve

	marshaledOID, err := asn1.Marshal(curve)
	Expect(err).ToNot(HaveOccurred())

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
	Expect(err).ToNot(HaveOccurred())

	ecpt := ecPoint(ctx, sess, pubK)
	Expect(err).NotTo(HaveOccurred())

	hash := sha256.Sum256(ecpt)
	ski := hash[:]

	setskiT := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, hex.EncodeToString(ski)),
	}

	err = ctx.SetAttributeValue(sess, pubK, setskiT)
	Expect(err).ToNot(HaveOccurred())

	err = ctx.SetAttributeValue(sess, privK, setskiT)
	Expect(err).ToNot(HaveOccurred())

	// convert pub key to rsa types
	nistCurve := elliptic.P256()
	x, y := elliptic.Unmarshal(nistCurve, ecpt)
	if x == nil {
		Fail("Failed Unmarshaling Public Key")
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
		Fail("PKCS11: get(EC point)")
	}

	for _, a := range attr {
		if a.Type == pkcs11.CKA_EC_POINT {
			if ((len(a.Value) % 2) == 0) && (byte(0x04) == a.Value[0]) && (byte(0x04) == a.Value[len(a.Value)-1]) {
				ecpt = a.Value[0 : len(a.Value)-1] // Trim trailing 0x04
			} else if byte(0x04) == a.Value[0] && byte(0x04) == a.Value[2] {
				ecpt = a.Value[2:len(a.Value)]
			} else {
				ecpt = a.Value
			}
		}
	}
	if ecpt == nil {
		Fail("CKA_EC_POINT not found")
	}

	return ecpt
}
