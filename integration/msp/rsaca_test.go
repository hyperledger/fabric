/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	fabricmsp "github.com/hyperledger/fabric/msp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"gopkg.in/yaml.v2"
)

var _ = Describe("MSPs with RSA Certificate Authorities", func() {
	var (
		client  *docker.Client
		testDir string
		network *nwo.Network

		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "msp")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()

		By("manually bootstrapping MSPs with RSA CAs")
		generateRSACACrypto(network)
		network.CreateDockerNetwork()

		for _, c := range network.Channels {
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   c.Name,
				Profile:     c.Profile,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath(c.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		}
		network.ConcatenateTLSCACertificates()

		By("starting all processes for fabric")
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
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

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("executes transactions endorsed with ECDSA signing certs", func() {
		org1Peer0 := network.Peer("Org1", "peer0")
		orderer := network.Orderer("orderer")

		chaincode := nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_prebuilt_chaincode",
		}

		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		nwo.EnableCapabilities(
			network,
			"testchannel",
			"Application", "V2_0",
			orderer,
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		)
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
		RunQueryInvokeQuery(network, orderer, org1Peer0, 100)
	})
})

// What follows is a bunch of code to build a hand-crafted set of MSPs where
// everything except signing certificates use RSA keys. It's not pretty but it
// gets the job done for testing.

func generateRSACACrypto(n *nwo.Network) {
	cryptoDir := n.CryptoPath()
	for _, o := range n.OrdererOrgs() {
		orgDir := filepath.Join(cryptoDir, "ordererOrganizations", o.Domain)
		signCA, tlsCA, adminCert := createMSP(orgDir, o.Domain, o.EnableNodeOUs)
		for i := 1; i <= o.Users; i++ {
			name := fmt.Sprintf("User%d@%s", i, o.Domain)
			dir := filepath.Join(orgDir, "users", name)
			var ous []string
			if o.EnableNodeOUs {
				ous = append(ous, "client")
			}
			writeLocalMSP(dir, name, ous, nil, signCA, tlsCA, adminCert, o.EnableNodeOUs, true)
		}
		for _, orderer := range n.OrderersInOrg(o.Name) {
			name := orderer.Name + "." + o.Domain
			dir := filepath.Join(orgDir, "orderers", name)
			sans := []string{"127.0.0.1", "::1", "localhost"}
			var ous []string
			if o.EnableNodeOUs {
				ous = append(ous, "orderer")
			}
			writeLocalMSP(dir, name, ous, sans, signCA, tlsCA, adminCert, o.EnableNodeOUs, false)
		}
	}

	for _, o := range n.PeerOrgs() {
		orgDir := filepath.Join(cryptoDir, "peerOrganizations", o.Domain)
		signCA, tlsCA, adminCert := createMSP(orgDir, o.Domain, o.EnableNodeOUs)
		for i := 1; i <= o.Users; i++ {
			name := fmt.Sprintf("User%d@%s", i, o.Domain)
			dir := filepath.Join(orgDir, "users", name)
			var ous []string
			if o.EnableNodeOUs {
				ous = append(ous, "client")
			}
			writeLocalMSP(dir, name, ous, nil, signCA, tlsCA, adminCert, o.EnableNodeOUs, true)
		}
		for _, peer := range n.PeersInOrg(o.Name) {
			name := peer.Name + "." + o.Domain
			dir := filepath.Join(orgDir, "peers", name)
			sans := []string{"127.0.0.1", "::1", "localhost"}
			var ous []string
			if o.EnableNodeOUs {
				ous = append(ous, "peer")
			}
			writeLocalMSP(dir, name, ous, sans, signCA, tlsCA, adminCert, o.EnableNodeOUs, false)
		}
	}
}

func createMSP(baseDir, domain string, nodeOUs bool) (signCA *CA, tlsCA *CA, adminPemCert []byte) {
	caDir := filepath.Join(baseDir, "ca")
	signCA = newCA(domain, "ca")
	writeCA(signCA, caDir)

	tlsCADir := filepath.Join(baseDir, "tlsca")
	tlsCA = newCA(domain, "tlsca")
	writeCA(tlsCA, tlsCADir)

	mspDir := filepath.Join(baseDir, "msp")
	writeVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)

	adminUsername := "Admin@" + domain
	adminDir := filepath.Join(baseDir, "users", adminUsername)
	err := os.MkdirAll(adminDir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	var ous []string
	if nodeOUs {
		ous = append(ous, "admin")
	}

	writeLocalMSP(adminDir, adminUsername, ous, nil, signCA, tlsCA, nil, nodeOUs, true)
	adminPemCert, err = ioutil.ReadFile(filepath.Join(adminDir, "msp", "signcerts", certFilename(adminUsername)))
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(adminDir, "msp", "admincerts", certFilename(adminUsername)), adminPemCert, 0o644)
	Expect(err).NotTo(HaveOccurred())

	if !nodeOUs {
		err := ioutil.WriteFile(filepath.Join(mspDir, "admincerts", certFilename(adminUsername)), adminPemCert, 0o644)
		Expect(err).NotTo(HaveOccurred())
	}

	return signCA, tlsCA, adminPemCert
}

func writeCA(ca *CA, dir string) {
	err := os.MkdirAll(dir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	certFilename := filepath.Join(dir, ca.certFilename())
	writeCertificate(certFilename, ca.certBytes)

	keyFilename := filepath.Join(dir, fmt.Sprintf("%x_sk", ca.cert.SubjectKeyId))
	writeKey(keyFilename, ca.signer)
}

func writeCertificate(filename string, der []byte) {
	certFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0o644)
	Expect(err).NotTo(HaveOccurred())
	defer certFile.Close()
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	Expect(err).NotTo(HaveOccurred())
}

func writeKey(filename string, signer crypto.Signer) {
	keyFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0o600)
	Expect(err).NotTo(HaveOccurred())
	defer keyFile.Close()
	derKey, err := x509.MarshalPKCS8PrivateKey(signer)
	Expect(err).NotTo(HaveOccurred())
	err = pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: derKey})
	Expect(err).NotTo(HaveOccurred())
}

func writeVerifyingMSP(mspDir string, signCA, tlsCA *CA, nodeOUs bool) {
	for _, dir := range []string{"admincerts", "cacerts", "tlscacerts"} {
		err := os.MkdirAll(filepath.Join(mspDir, dir), 0o755)
		Expect(err).NotTo(HaveOccurred())
	}
	if nodeOUs {
		configFilename := filepath.Join(mspDir, "config.yaml")
		writeConfigYaml(configFilename, filepath.Join("cacerts", signCA.certFilename()), nodeOUs)
	}
	writeCertificate(filepath.Join(mspDir, "cacerts", signCA.certFilename()), signCA.certBytes)
	writeCertificate(filepath.Join(mspDir, "tlscacerts", tlsCA.certFilename()), tlsCA.certBytes)
}

func writeLocalMSP(baseDir, name string, signOUs, sans []string, signCA, tlsCA *CA, adminCertPem []byte, nodeOUs, client bool) {
	mspDir := filepath.Join(baseDir, "msp")
	err := os.MkdirAll(mspDir, 0o755)
	Expect(err).NotTo(HaveOccurred())
	writeVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)

	for _, dir := range []string{"admincerts", "keystore", "signcerts"} {
		err := os.MkdirAll(filepath.Join(mspDir, dir), 0o755)
		Expect(err).NotTo(HaveOccurred())
	}

	if !nodeOUs && len(adminCertPem) != 0 {
		block, _ := pem.Decode(adminCertPem)
		adminCert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(mspDir, "admincerts", certFilename(adminCert.Subject.CommonName)), adminCertPem, 0o644)
		Expect(err).NotTo(HaveOccurred())
	}

	// create signcert
	priv := generateECKey()
	signcertBytes, signcert := signCA.issueSignCertificate(name, signOUs, priv.Public())
	signcertFilename := filepath.Join(mspDir, "signcerts", certFilename(name))
	writeCertificate(signcertFilename, signcertBytes)
	signcertKeyFilename := filepath.Join(mspDir, "keystore", fmt.Sprintf("%x_sk", signcert.SubjectKeyId))
	writeKey(signcertKeyFilename, priv)

	// populate tls
	tlsDir := filepath.Join(baseDir, "tls")
	err = os.MkdirAll(tlsDir, 0o755)
	Expect(err).NotTo(HaveOccurred())
	writeCertificate(filepath.Join(tlsDir, "ca.crt"), tlsCA.certBytes)

	tlsKey := generateRSAKey()
	tlsCertBytes, _ := tlsCA.issueTLSCertificate(name, sans, tlsKey.Public())
	if client {
		writeCertificate(filepath.Join(tlsDir, "client.crt"), tlsCertBytes)
		writeKey(filepath.Join(tlsDir, "client.key"), tlsKey)
	} else {
		writeCertificate(filepath.Join(tlsDir, "server.crt"), tlsCertBytes)
		writeKey(filepath.Join(tlsDir, "server.key"), tlsKey)
	}
}

func writeConfigYaml(configFilename, caFile string, enable bool) {
	config := &fabricmsp.Configuration{
		NodeOUs: &fabricmsp.NodeOUs{
			Enable: enable,
			ClientOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: "client",
			},
			PeerOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: "peer",
			},
			AdminOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: "admin",
			},
			OrdererOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: "orderer",
			},
		},
	}

	configFile, err := os.Create(configFilename)
	Expect(err).NotTo(HaveOccurred())
	defer configFile.Close()

	err = yaml.NewEncoder(configFile).Encode(config)
	Expect(err).NotTo(HaveOccurred())
}

type CA struct {
	signer    crypto.Signer
	cert      *x509.Certificate
	certBytes []byte
}

func newCA(orgName, caName string) *CA {
	signer := generateRSAKey()

	template := x509Template()
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageDigitalSignature
	template.KeyUsage |= x509.KeyUsageKeyEncipherment
	template.KeyUsage |= x509.KeyUsageCertSign
	template.KeyUsage |= x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{
		x509.ExtKeyUsageClientAuth,
		x509.ExtKeyUsageServerAuth,
	}
	template.Subject = pkix.Name{
		CommonName:   caName + "." + orgName,
		Organization: []string{orgName},
	}
	template.SubjectKeyId = computeSKI(signer.Public())

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, signer.Public(), signer)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(certBytes)
	Expect(err).NotTo(HaveOccurred())

	return &CA{
		signer:    signer,
		cert:      cert,
		certBytes: certBytes,
	}
}

func (ca *CA) issueSignCertificate(name string, ous []string, pub crypto.PublicKey) ([]byte, *x509.Certificate) {
	template := x509Template()
	template.KeyUsage = x509.KeyUsageDigitalSignature
	template.ExtKeyUsage = nil
	template.Subject = pkix.Name{
		CommonName:         name,
		Organization:       ca.cert.Subject.Organization,
		OrganizationalUnit: ous,
	}
	template.SubjectKeyId = computeSKI(pub)

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, ca.cert, pub, ca.signer)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(certBytes)
	Expect(err).NotTo(HaveOccurred())
	return certBytes, cert
}

func (ca *CA) issueTLSCertificate(name string, sans []string, pub crypto.PublicKey) ([]byte, *x509.Certificate) {
	template := x509Template()
	template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	template.ExtKeyUsage = []x509.ExtKeyUsage{
		x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth,
	}
	template.Subject = pkix.Name{
		CommonName:   name,
		Organization: ca.cert.Subject.Organization,
	}
	template.SubjectKeyId = computeSKI(pub)

	for _, san := range sans {
		if ip := net.ParseIP(san); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, san)
		}
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, ca.cert, pub, ca.signer)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(certBytes)
	Expect(err).NotTo(HaveOccurred())
	return certBytes, cert
}

func (ca *CA) certFilename() string {
	return certFilename(ca.cert.Subject.CommonName)
}

func certFilename(stem string) string {
	return stem + "-cert.pem"
}

func generateRSAKey() crypto.Signer {
	signer, err := rsa.GenerateKey(rand.Reader, 4096)
	Expect(err).NotTo(HaveOccurred())
	return signer
}

func generateECKey() crypto.Signer {
	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	return signer
}

func x509Template() x509.Certificate {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	notBefore := time.Now().Round(time.Minute).Add(-5 * time.Minute).UTC()

	return x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(3650 * 24 * time.Hour).UTC(),
		BasicConstraintsValid: true,
	}
}

func computeSKI(key crypto.PublicKey) []byte {
	var raw []byte
	switch key := key.(type) {
	case *rsa.PublicKey:
		raw = x509.MarshalPKCS1PublicKey(key)
	case *ecdsa.PublicKey:
		raw = elliptic.Marshal(key.Curve, key.X, key.Y)
	default:
		panic(fmt.Sprintf("unexpected type: %T", key))
	}
	hash := sha256.Sum256(raw)
	return hash[:]
}
