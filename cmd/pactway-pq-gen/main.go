/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/signer"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	fabricmsp "github.com/hyperledger/fabric/msp"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"google.golang.org/protobuf/proto"
)

const (
	defaultFabricProfile = genesisconfig.SampleAppChannelEtcdRaftProfile
	defaultChannelID     = "pactway-pq2-demo"
	ordererOrgName       = "OrdererOrg"
	ordererMSPID         = "OrdererOrgMSP"
)

type options struct {
	outputDir     string
	configDir     string
	fabricProfile string
	hybridProfile string
	channelID     string
	ordererHost   string
	ordererPort   uint32
}

type hybridCertificateSigner struct {
	csp       bccsp.BCCSP
	key       bccsp.Key
	publicKey oqs.PublicKey
}

func (signer hybridCertificateSigner) PublicKey() oqs.PublicKey {
	return signer.publicKey
}

func (signer hybridCertificateSigner) SignHybrid(message []byte) ([]byte, error) {
	return signer.csp.Sign(signer.key, message, nil)
}

type hybridKeys struct {
	classicalPublic crypto.PublicKey
	classicalSigner crypto.Signer
	pqSigner        hybridCertificateSigner
}

type tlsAuthority struct {
	certificate *x509.Certificate
	privateKey  *ecdsa.PrivateKey
	certPEM     []byte
}

type organization struct {
	name              string
	mspID             string
	publicMSPDir      string
	nodeRole          string
	nodeName          string
	nodeHost          string
	nodePort          uint32
	tlsServerCertPath string
	tlsClientCertPath string
}

type demoManifest struct {
	ChannelID                string   `json:"channel_id"`
	FabricProfile            string   `json:"fabric_profile"`
	HybridProfile            string   `json:"hybrid_profile"`
	DevelopmentOnly          bool     `json:"development_only"`
	ChannelCapability        string   `json:"channel_capability"`
	Organizations            []string `json:"organizations"`
	GenesisSHA256            string   `json:"genesis_sha256"`
	TLSTransport             string   `json:"tls_transport"`
	PrivateMaterialDirectory string   `json:"private_material_directory"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("pactway-pq-gen", flag.ContinueOnError)
	opts := options{}
	flags.StringVar(&opts.outputDir, "output", "", "new output directory (must not already exist)")
	flags.StringVar(&opts.configDir, "configtx-dir", "sampleconfig", "directory containing configtx.yaml")
	flags.StringVar(&opts.fabricProfile, "fabric-profile", defaultFabricProfile, "configtx profile to use as Fabric defaults")
	flags.StringVar(&opts.hybridProfile, "hybrid-profile", "", "explicit Pactway hybrid certificate profile")
	flags.StringVar(&opts.channelID, "channel-id", defaultChannelID, "application channel ID")
	flags.StringVar(&opts.ordererHost, "orderer-host", "orderer0.ordererorg.pactway.local", "demo orderer hostname")
	ordererPort := uint(7050)
	flags.UintVar(&ordererPort, "orderer-port", ordererPort, "demo orderer port")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if ordererPort > 65535 {
		return errors.New("orderer port must not exceed 65535")
	}
	opts.ordererPort = uint32(ordererPort)
	if opts.outputDir == "" {
		return errors.New("-output is required")
	}
	if opts.hybridProfile == "" {
		return errors.New("-hybrid-profile is required; profile selection must be explicit")
	}
	if os.Getenv("PACTWAY_ISOLATED_DEVELOPMENT") != "true" {
		return errors.New("certificate generation requires PACTWAY_ISOLATED_DEVELOPMENT=true")
	}
	profile, err := oqs.HybridCertificateProfileByName(opts.hybridProfile, oqs.ProfileUseIsolatedDevelopment)
	if err != nil {
		return err
	}
	return generateDemo(opts, profile)
}

func generateDemo(opts options, profile oqs.HybridCertificateProfile) error {
	if opts.ordererPort == 0 {
		return errors.New("orderer port must be nonzero")
	}
	configtxFile := filepath.Join(opts.configDir, "configtx.yaml")
	if _, err := os.Stat(configtxFile); err != nil {
		return fmt.Errorf("read configtx file %s: %w", configtxFile, err)
	}
	if _, err := os.Stat(opts.outputDir); err == nil {
		return fmt.Errorf("output directory already exists: %s", opts.outputDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output directory %s: %w", opts.outputDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.outputDir), 0o700); err != nil {
		return fmt.Errorf("create output parent directory: %w", err)
	}
	if err := os.Mkdir(opts.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	publicDir := filepath.Join(opts.outputDir, "public")
	privateDir := filepath.Join(opts.outputDir, "private")
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		return fmt.Errorf("create public output directory: %w", err)
	}
	if err := os.MkdirAll(privateDir, 0o700); err != nil {
		return fmt.Errorf("create private output directory: %w", err)
	}

	org1, err := generateOrganization(publicDir, privateDir, profile, "Org1", "Org1MSP", "peer", "peer0.org1.pactway.local", 7051)
	if err != nil {
		return err
	}
	org2, err := generateOrganization(publicDir, privateDir, profile, "Org2", "Org2MSP", "peer", "peer0.org2.pactway.local", 8051)
	if err != nil {
		return err
	}
	ordererOrg, err := generateOrganization(publicDir, privateDir, profile, ordererOrgName, ordererMSPID, "orderer", opts.ordererHost, opts.ordererPort)
	if err != nil {
		return err
	}

	genesisProfile := genesisconfig.Load(opts.fabricProfile, opts.configDir)
	if genesisProfile.Orderer == nil || genesisProfile.Orderer.EtcdRaft == nil || genesisProfile.Application == nil {
		return errors.New("selected Fabric profile must define an application channel and etcdraft orderer")
	}
	if err := configtx.ValidateChannelID(opts.channelID); err != nil {
		return err
	}

	genesisProfile.Capabilities = ensureCapability(genesisProfile.Capabilities, capabilities.PactwayHybridCertificateV1)
	genesisProfile.Orderer.Addresses = nil
	genesisProfile.Orderer.Organizations = []*genesisconfig.Organization{ordererOrg.profileOrganization()}
	genesisProfile.Orderer.EtcdRaft.Consenters = []*etcdraft.Consenter{{
		Host:          opts.ordererHost,
		Port:          opts.ordererPort,
		ClientTlsCert: []byte(ordererOrg.tlsClientCertPath),
		ServerTlsCert: []byte(ordererOrg.tlsServerCertPath),
	}}
	genesisProfile.Application.Organizations = []*genesisconfig.Organization{
		org1.profileOrganization(),
		org2.profileOrganization(),
	}

	bootstrapper, err := encoder.NewBootstrapper(genesisProfile)
	if err != nil {
		return fmt.Errorf("create Fabric bootstrapper: %w", err)
	}
	genesisBlock := bootstrapper.GenesisBlockForChannel(opts.channelID)
	genesisDER, err := proto.Marshal(genesisBlock)
	if err != nil {
		return fmt.Errorf("marshal genesis block: %w", err)
	}
	if err := writeFile(filepath.Join(publicDir, "genesis.block"), genesisDER, 0o644); err != nil {
		return err
	}

	digest := sha256.Sum256(genesisDER)
	manifest := demoManifest{
		ChannelID:                opts.channelID,
		FabricProfile:            opts.fabricProfile,
		HybridProfile:            profile.Name(),
		DevelopmentOnly:          profile.DevelopmentOnly(),
		ChannelCapability:        capabilities.PactwayHybridCertificateV1,
		Organizations:            []string{"Org1MSP", "Org2MSP", ordererMSPID},
		GenesisSHA256:            hex.EncodeToString(digest[:]),
		TLSTransport:             "classical ECDSA; enrollment certificates use the selected hybrid profile",
		PrivateMaterialDirectory: "private/",
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal demo manifest: %w", err)
	}
	if err := writeFile(filepath.Join(publicDir, "manifest.json"), append(manifestBytes, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func generateOrganization(
	publicDir, privateDir string,
	profile oqs.HybridCertificateProfile,
	name, mspID, nodeRole, nodeHost string,
	nodePort uint32,
) (*organization, error) {
	publicMSPDir := filepath.Join(publicDir, "organizations", name, "msp")
	rootCAStore := filepath.Join(privateDir, "ca", name, "root", "keystore")
	intermediateCAStore := filepath.Join(privateDir, "ca", name, "intermediate", "keystore")

	rootKeys, err := generateHybridKeys(rootCAStore)
	if err != nil {
		return nil, fmt.Errorf("generate %s root CA keys: %w", mspID, err)
	}
	rootTemplate, err := newHybridCertificateTemplate(name+" root CA", "ca", true, 1)
	if err != nil {
		return nil, err
	}
	rootTemplate.PublicKey = rootKeys.classicalPublic
	rootTemplate.AuthorityKeyId = rootTemplate.SubjectKeyId
	rootDER, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, rootTemplate, rootTemplate,
		rootKeys.classicalPublic, rootKeys.classicalSigner, rootKeys.pqSigner.PublicKey(), rootKeys.pqSigner,
	)
	if err != nil {
		return nil, fmt.Errorf("create %s hybrid root CA: %w", mspID, err)
	}
	rootCertificate, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, fmt.Errorf("parse %s root CA: %w", mspID, err)
	}
	rootPEM := encodeCertificate(rootDER)

	intermediateKeys, err := generateHybridKeys(intermediateCAStore)
	if err != nil {
		return nil, fmt.Errorf("generate %s intermediate CA keys: %w", mspID, err)
	}
	intermediateTemplate, err := newHybridCertificateTemplate(name+" intermediate CA", "ca", true, 0)
	if err != nil {
		return nil, err
	}
	intermediateTemplate.PublicKey = intermediateKeys.classicalPublic
	intermediateTemplate.AuthorityKeyId = rootCertificate.SubjectKeyId
	intermediateDER, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, intermediateTemplate, rootCertificate,
		intermediateKeys.classicalPublic, rootKeys.classicalSigner, intermediateKeys.pqSigner.PublicKey(), rootKeys.pqSigner,
	)
	if err != nil {
		return nil, fmt.Errorf("create %s hybrid intermediate CA: %w", mspID, err)
	}
	intermediateCertificate, err := x509.ParseCertificate(intermediateDER)
	if err != nil {
		return nil, fmt.Errorf("parse %s intermediate CA: %w", mspID, err)
	}
	intermediatePEM := encodeCertificate(intermediateDER)

	tlsCA, err := generateTLSAuthority(privateDir, name)
	if err != nil {
		return nil, err
	}

	adminIdentity, err := generateIdentity(
		profile, name, "admin", "Admin@"+name, intermediateCertificate, intermediateKeys,
		filepath.Join(privateDir, "organizations", name, "users", "Admin", "msp"),
		filepath.Join(publicDir, "organizations", name, "identities", "admin-cert.pem"),
	)
	if err != nil {
		return nil, err
	}

	clientIdentity, err := generateIdentity(
		profile, name, "client", "Client@"+name, intermediateCertificate, intermediateKeys,
		filepath.Join(privateDir, "organizations", name, "users", "Client", "msp"),
		filepath.Join(publicDir, "organizations", name, "identities", "client-cert.pem"),
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client identity: %w", mspID, err)
	}

	nodeLocalMSP := filepath.Join(privateDir, "organizations", name, "nodes", nodeRole+"0", "msp")
	nodePublicCert := filepath.Join(publicDir, "organizations", name, "identities", nodeRole+"0-cert.pem")
	nodeIdentity, err := generateIdentity(
		profile, name, nodeRole, nodeRole+"0."+name, intermediateCertificate, intermediateKeys,
		nodeLocalMSP, nodePublicCert,
	)
	if err != nil {
		return nil, err
	}

	tlsServerCertPath, tlsClientCertPath, err := generateTLSNodeCertificates(
		tlsCA, privateDir, name, nodeRole, nodeHost,
	)
	if err != nil {
		return nil, err
	}

	if err := writeMSPFiles(publicMSPDir, rootPEM, intermediatePEM, tlsCA.certPEM, adminIdentity.certPEM, nil); err != nil {
		return nil, fmt.Errorf("write public %s MSP: %w", mspID, err)
	}
	if err := writeMSPFiles(
		filepath.Join(privateDir, "organizations", name, "users", "Admin", "msp"),
		rootPEM, intermediatePEM, tlsCA.certPEM, adminIdentity.certPEM, adminIdentity.certPEM,
	); err != nil {
		return nil, fmt.Errorf("write %s admin local MSP: %w", mspID, err)
	}
	if err := writeMSPFiles(
		filepath.Join(privateDir, "organizations", name, "users", "Client", "msp"),
		rootPEM, intermediatePEM, tlsCA.certPEM, adminIdentity.certPEM, clientIdentity.certPEM,
	); err != nil {
		return nil, fmt.Errorf("write %s client local MSP: %w", mspID, err)
	}
	if err := writeMSPFiles(
		nodeLocalMSP, rootPEM, intermediatePEM, tlsCA.certPEM, adminIdentity.certPEM, nodeIdentity.certPEM,
	); err != nil {
		return nil, fmt.Errorf("write %s %s local MSP: %w", mspID, nodeRole, err)
	}
	result := &organization{
		name:              name,
		mspID:             mspID,
		publicMSPDir:      publicMSPDir,
		nodeRole:          nodeRole,
		nodeName:          nodeRole + "0",
		nodeHost:          nodeHost,
		nodePort:          nodePort,
		tlsServerCertPath: tlsServerCertPath,
		tlsClientCertPath: tlsClientCertPath,
	}
	return result, nil
}

func (org *organization) profileOrganization() *genesisconfig.Organization {
	policies := map[string]*genesisconfig.Policy{
		"Readers":     {Type: "Signature", Rule: fmt.Sprintf("OR('%s.member')", org.mspID)},
		"Writers":     {Type: "Signature", Rule: fmt.Sprintf("OR('%s.member')", org.mspID)},
		"Admins":      {Type: "Signature", Rule: fmt.Sprintf("OR('%s.admin')", org.mspID)},
		"Endorsement": {Type: "Signature", Rule: fmt.Sprintf("OR('%s.peer')", org.mspID)},
	}
	if org.nodeRole == "orderer" {
		policies["Endorsement"].Rule = fmt.Sprintf("OR('%s.member')", org.mspID)
	}
	result := &genesisconfig.Organization{
		Name:     org.name,
		ID:       org.mspID,
		MSPDir:   org.publicMSPDir,
		MSPType:  fabricmsp.ProviderTypeToString(fabricmsp.FABRIC),
		Policies: policies,
	}
	if org.nodeRole == "orderer" {
		result.OrdererEndpoints = []string{fmt.Sprintf("%s:%d", org.nodeHost, org.nodePort)}
	} else {
		result.AnchorPeers = []*genesisconfig.AnchorPeer{{Host: org.nodeHost, Port: int(org.nodePort)}}
	}
	return result
}

type generatedIdentity struct {
	certPEM []byte
}

func generateIdentity(
	profile oqs.HybridCertificateProfile,
	orgName, role, commonName string,
	issuerCertificate *x509.Certificate,
	issuerKeys hybridKeys,
	localMSPDir, publicCertPath string,
) (generatedIdentity, error) {
	cryptoProvider, err := newCryptoProvider(filepath.Join(localMSPDir, "keystore"))
	if err != nil {
		return generatedIdentity{}, err
	}
	identityKeys, err := generateHybridKeysWithProvider(cryptoProvider)
	if err != nil {
		return generatedIdentity{}, fmt.Errorf("generate %s %s keys: %w", orgName, role, err)
	}
	template, err := newHybridCertificateTemplate(commonName, role, false, 0)
	if err != nil {
		return generatedIdentity{}, err
	}
	template.PublicKey = identityKeys.classicalPublic
	template.AuthorityKeyId = issuerCertificate.SubjectKeyId
	certificateDER, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, template, issuerCertificate,
		identityKeys.classicalPublic, issuerKeys.classicalSigner, identityKeys.pqSigner.PublicKey(), issuerKeys.pqSigner,
	)
	if err != nil {
		return generatedIdentity{}, fmt.Errorf("create %s %s hybrid certificate: %w", orgName, role, err)
	}
	certificatePEM := encodeCertificate(certificateDER)
	if err := writeFile(publicCertPath, certificatePEM, 0o644); err != nil {
		return generatedIdentity{}, err
	}
	if err := os.MkdirAll(filepath.Join(localMSPDir, "signcerts"), 0o700); err != nil {
		return generatedIdentity{}, err
	}
	if err := writeFile(filepath.Join(localMSPDir, "signcerts", "cert.pem"), certificatePEM, 0o600); err != nil {
		return generatedIdentity{}, err
	}
	return generatedIdentity{certPEM: certificatePEM}, nil
}

func generateHybridKeys(keyStoreDir string) (hybridKeys, error) {
	cryptoProvider, err := newCryptoProvider(keyStoreDir)
	if err != nil {
		return hybridKeys{}, err
	}
	return generateHybridKeysWithProvider(cryptoProvider)
}

func generateHybridKeysWithProvider(cryptoProvider bccsp.BCCSP) (hybridKeys, error) {
	classicalPrivate, err := cryptoProvider.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	if err != nil {
		return hybridKeys{}, err
	}
	classicalPublicKey, err := classicalPrivate.PublicKey()
	if err != nil {
		return hybridKeys{}, err
	}
	classicalPublicDER, err := classicalPublicKey.Bytes()
	if err != nil {
		return hybridKeys{}, err
	}
	classicalPublic, err := x509.ParsePKIXPublicKey(classicalPublicDER)
	if err != nil {
		return hybridKeys{}, err
	}
	classicalSigner, err := signer.New(cryptoProvider, classicalPrivate)
	if err != nil {
		return hybridKeys{}, err
	}
	pqPrivate, err := cryptoProvider.KeyGen(&bccsp.OQSKeyGenOpts{
		Temporary:       false,
		SignatureScheme: string(oqs.MLDSA65ProfileV1),
	})
	if err != nil {
		return hybridKeys{}, err
	}
	pqSigner, err := newHybridCertificateSigner(cryptoProvider, pqPrivate)
	if err != nil {
		return hybridKeys{}, err
	}
	return hybridKeys{
		classicalPublic: classicalPublic,
		classicalSigner: classicalSigner,
		pqSigner:        pqSigner,
	}, nil
}

func newHybridCertificateSigner(csp bccsp.BCCSP, key bccsp.Key) (hybridCertificateSigner, error) {
	publicKey, err := key.PublicKey()
	if err != nil {
		return hybridCertificateSigner{}, err
	}
	publicKeyDER, err := publicKey.Bytes()
	if err != nil {
		return hybridCertificateSigner{}, err
	}
	parsedKey, err := oqs.ParsePKIXPublicKey(publicKeyDER)
	if err != nil {
		return hybridCertificateSigner{}, err
	}
	hybridPublicKey, ok := parsedKey.(*oqs.PublicKey)
	if !ok {
		return hybridCertificateSigner{}, errors.New("BCCSP signing key is not ML-DSA-65")
	}
	return hybridCertificateSigner{csp: csp, key: key, publicKey: *hybridPublicKey}, nil
}

func newCryptoProvider(keyStoreDir string) (bccsp.BCCSP, error) {
	if err := os.MkdirAll(keyStoreDir, 0o700); err != nil {
		return nil, err
	}
	keyStore, err := sw.NewFileBasedKeyStore(nil, keyStoreDir, false)
	if err != nil {
		return nil, err
	}
	return sw.NewDefaultSecurityLevelWithKeystore(keyStore)
}

func newHybridCertificateTemplate(commonName, role string, isCA bool, maxPathLen int) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	subjectKeyID := make([]byte, 20)
	if _, err := rand.Read(subjectKeyID); err != nil {
		return nil, fmt.Errorf("generate subject key identifier: %w", err)
	}
	keyUsage := x509.KeyUsageDigitalSignature
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, OrganizationalUnit: []string{role}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		SubjectKeyId:          subjectKeyID,
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		template.MaxPathLen = maxPathLen
		template.MaxPathLenZero = maxPathLen == 0
		template.Subject.OrganizationalUnit = []string{"ca"}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	}
	return template, nil
}

func writeMSPFiles(dir string, rootPEM, intermediatePEM, tlsRootPEM, adminPEM, signerPEM []byte) error {
	for _, directory := range []string{"cacerts", "intermediatecerts", "admincerts", "tlscacerts"} {
		if err := os.MkdirAll(filepath.Join(dir, directory), 0o755); err != nil {
			return err
		}
	}
	for path, content := range map[string][]byte{
		filepath.Join("cacerts", "root.pem"):                   rootPEM,
		filepath.Join("intermediatecerts", "intermediate.pem"): intermediatePEM,
		filepath.Join("tlscacerts", "tls-ca.pem"):              tlsRootPEM,
		filepath.Join("admincerts", "admin.pem"):               adminPEM,
	} {
		if err := writeFile(filepath.Join(dir, path), content, 0o644); err != nil {
			return err
		}
	}
	if signerPEM != nil {
		if err := os.MkdirAll(filepath.Join(dir, "signcerts"), 0o700); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, "signcerts", "cert.pem"), signerPEM, 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, fabricmsp.PactwayHybridPolicyFile), []byte("PQ_REQUIRED\n"), 0o600); err != nil {
			return err
		}
	}
	config := []byte(`NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: intermediatecerts/intermediate.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: intermediatecerts/intermediate.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: intermediatecerts/intermediate.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: intermediatecerts/intermediate.pem
    OrganizationalUnitIdentifier: orderer
`)
	return writeFile(filepath.Join(dir, "config.yaml"), config, 0o644)
}

func generateTLSAuthority(privateDir, orgName string) (tlsAuthority, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tlsAuthority{}, err
	}
	template, err := newTLSCertificateTemplate(orgName+" TLS CA", true)
	if err != nil {
		return tlsAuthority{}, err
	}
	template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tlsAuthority{}, err
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return tlsAuthority{}, err
	}
	certPEM := encodeCertificate(der)
	keyPEM, err := encodeECPrivateKey(privateKey)
	if err != nil {
		return tlsAuthority{}, err
	}
	if err := writeFile(filepath.Join(privateDir, "ca", orgName, "tls", "ca.pem"), certPEM, 0o644); err != nil {
		return tlsAuthority{}, err
	}
	if err := writeFile(filepath.Join(privateDir, "ca", orgName, "tls", "ca-key.pem"), keyPEM, 0o600); err != nil {
		return tlsAuthority{}, err
	}
	return tlsAuthority{certificate: certificate, privateKey: privateKey, certPEM: certPEM}, nil
}

func generateTLSNodeCertificates(ca tlsAuthority, privateDir, orgName, role, host string) (string, string, error) {
	nodeDir := filepath.Join(privateDir, "organizations", orgName, "nodes", role+"0", "tls")
	serverCertPEM, err := generateTLSLeaf(ca, host, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return "", "", err
	}
	clientCertPEM, err := generateTLSLeaf(ca, host, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return "", "", err
	}
	serverCertPath := filepath.Join(nodeDir, "server.crt")
	serverKeyPath := filepath.Join(nodeDir, "server.key")
	clientCertPath := filepath.Join(nodeDir, "client.crt")
	clientKeyPath := filepath.Join(nodeDir, "client.key")
	if err := writeFile(serverCertPath, serverCertPEM.certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := writeFile(serverKeyPath, serverCertPEM.keyPEM, 0o600); err != nil {
		return "", "", err
	}
	if err := writeFile(clientCertPath, clientCertPEM.certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := writeFile(clientKeyPath, clientCertPEM.keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return serverCertPath, clientCertPath, nil
}

type tlsLeaf struct {
	certPEM []byte
	keyPEM  []byte
}

func generateTLSLeaf(ca tlsAuthority, host string, usage x509.ExtKeyUsage) (tlsLeaf, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tlsLeaf{}, err
	}
	template, err := newTLSCertificateTemplate(host, false)
	if err != nil {
		return tlsLeaf{}, err
	}
	template.DNSNames = []string{host}
	template.ExtKeyUsage = []x509.ExtKeyUsage{usage}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.certificate, &privateKey.PublicKey, ca.privateKey)
	if err != nil {
		return tlsLeaf{}, err
	}
	keyPEM, err := encodeECPrivateKey(privateKey)
	if err != nil {
		return tlsLeaf{}, err
	}
	return tlsLeaf{certPEM: encodeCertificate(der), keyPEM: keyPEM}, nil
}

func newTLSCertificateTemplate(commonName string, isCA bool) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	keyUsage := x509.KeyUsageDigitalSignature
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	return template, nil
}

func encodeECPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func encodeCertificate(certificateDER []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
}

func ensureCapability(capabilities map[string]bool, name string) map[string]bool {
	if capabilities == nil {
		capabilities = make(map[string]bool)
	}
	capabilities[name] = true
	return capabilities
}

func writeFile(path string, contents []byte, mode os.FileMode) error {
	directoryMode := os.FileMode(0o700)
	if mode&0o007 != 0 {
		directoryMode = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
