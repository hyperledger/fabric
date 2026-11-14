/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/bccsp/signer"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type bccspHybridCertificateSigner struct {
	csp       bccsp.BCCSP
	key       bccsp.Key
	publicKey oqs.PublicKey
}

func (signer bccspHybridCertificateSigner) PublicKey() oqs.PublicKey {
	return signer.publicKey
}

func (signer bccspHybridCertificateSigner) SignHybrid(message []byte) ([]byte, error) {
	return signer.csp.Sign(signer.key, message, nil)
}

func TestHybridRequiredMSPSigningAndVerificationWithBCCSP(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	mspConfig, cryptoProvider := newHybridMSPTestConfig(t, true)

	instance, err := newBccspMsp(MSPv1_0, cryptoProvider)
	require.NoError(t, err)
	require.NoError(t, instance.Setup(mspConfig))
	require.ErrorContains(t, instance.(*bccspmsp).SetHybridRequired(false), "cannot be weakened")

	signingIdentity, err := instance.GetDefaultSigningIdentity()
	require.NoError(t, err)
	signature, err := signingIdentity.Sign([]byte("PQ-required MSP integration"))
	require.NoError(t, err)
	require.NoError(t, signingIdentity.Verify([]byte("PQ-required MSP integration"), signature))
	require.NoError(t, signingIdentity.GetPublicVersion().Verify([]byte("PQ-required MSP integration"), signature))
	require.Error(t, signingIdentity.Verify([]byte("different message"), signature))
}

func TestHybridRequiredMSPRejectsClassicalOnlyAndMissingPQPrivateKey(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)

	classicalOnlyMSP, err := newBccspMsp(MSPv1_0, factory.GetDefault())
	require.NoError(t, err)
	require.NoError(t, classicalOnlyMSP.(*bccspmsp).SetHybridRequired(true))
	require.ErrorContains(t, classicalOnlyMSP.Setup(conf), "Pactway Hybrid Certificate v1")

	mspConfig, cryptoProvider := newHybridMSPTestConfig(t, false)
	missingPQPrivateKeyMSP, err := newBccspMsp(MSPv1_0, cryptoProvider)
	require.NoError(t, err)
	err = missingPQPrivateKeyMSP.Setup(mspConfig)
	require.ErrorContains(t, err, "matching ML-DSA-65 private key")
}

func TestHybridRequiredMSPCompliantRootRotation(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)

	oldConfig, oldCryptoProvider := newHybridMSPTestConfig(t, true)
	oldMSP, err := newBccspMsp(MSPv1_0, oldCryptoProvider)
	require.NoError(t, err)
	require.NoError(t, oldMSP.Setup(oldConfig))
	oldIdentity, err := oldMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	require.NoError(t, oldMSP.Validate(oldIdentity.GetPublicVersion()))

	newConfig, newCryptoProvider := newHybridMSPTestConfig(t, true)
	newMSP, err := newBccspMsp(MSPv1_0, newCryptoProvider)
	require.NoError(t, err)
	require.NoError(t, newMSP.Setup(newConfig))
	newIdentity, err := newMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	require.NoError(t, newMSP.Validate(newIdentity.GetPublicVersion()))
	oldSerializedIdentity, err := oldIdentity.GetPublicVersion().Serialize()
	require.NoError(t, err)
	_, err = newMSP.DeserializeIdentity(oldSerializedIdentity)
	require.ErrorContains(t, err, "certificate signed by unknown authority")
}

func newHybridMSPTestConfig(t *testing.T, persistLeafPQKey bool) (*mspprotos.MSPConfig, bccsp.BCCSP) {
	t.Helper()

	keyStore, err := sw.NewFileBasedKeyStore(nil, filepath.Join(t.TempDir(), "keystore"), false)
	require.NoError(t, err)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(keyStore)
	require.NoError(t, err)

	profile, err := oqs.HybridCertificateProfileFromEnvironment()
	require.NoError(t, err)

	rootClassicalKey, err := cryptoProvider.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.NoError(t, err)
	rootClassicalPublic, err := cryptoPublicKey(rootClassicalKey)
	require.NoError(t, err)
	rootClassicalSigner, err := signer.New(cryptoProvider, rootClassicalKey)
	require.NoError(t, err)
	rootPQKey, err := cryptoProvider.KeyGen(&bccsp.OQSKeyGenOpts{
		Temporary:       false,
		SignatureScheme: string(oqs.MLDSA65ProfileV1),
	})
	require.NoError(t, err)
	rootPQSigner, err := newBCCSPHybridCertificateSigner(cryptoProvider, rootPQKey)
	require.NoError(t, err)

	rootTemplate := hybridMSPCertificateTemplate(1, "PQ2 MSP test root", true)
	rootTemplate.PublicKey = rootClassicalPublic
	rootDER, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, rootTemplate, rootTemplate,
		rootClassicalPublic, rootClassicalSigner, rootPQSigner.PublicKey(), rootPQSigner,
	)
	require.NoError(t, err)
	rootCertificate, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	leafClassicalKey, err := cryptoProvider.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.NoError(t, err)
	leafClassicalPublic, err := cryptoPublicKey(leafClassicalKey)
	require.NoError(t, err)
	leafPQKey, err := cryptoProvider.KeyGen(&bccsp.OQSKeyGenOpts{
		Temporary:       !persistLeafPQKey,
		SignatureScheme: string(oqs.MLDSA65ProfileV1),
	})
	require.NoError(t, err)
	leafPQSigner, err := newBCCSPHybridCertificateSigner(cryptoProvider, leafPQKey)
	require.NoError(t, err)

	leafTemplate := hybridMSPCertificateTemplate(2, "PQ2 MSP test client", false)
	leafTemplate.AuthorityKeyId = rootCertificate.SubjectKeyId
	leafDER, err := oqs.CreateHybridCertificateWithPQSigner(
		rand.Reader, profile, leafTemplate, rootCertificate,
		leafClassicalPublic, rootClassicalSigner, leafPQSigner.PublicKey(), rootPQSigner,
	)
	require.NoError(t, err)

	config := &mspprotos.FabricMSPConfig{
		Name:      "PQ2TestMSP",
		RootCerts: [][]byte{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})},
		SigningIdentity: &mspprotos.SigningIdentityInfo{
			PublicSigner: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		},
		CryptoConfig: &mspprotos.FabricCryptoConfig{
			SignatureHashFamily:            bccsp.SHA2,
			IdentityIdentifierHashFunction: bccsp.SHA256,
		},
	}
	require.NoError(t, markHybridRequired(config))
	serializedConfig, err := proto.Marshal(config)
	require.NoError(t, err)

	return &mspprotos.MSPConfig{Type: int32(FABRIC), Config: serializedConfig}, cryptoProvider
}

func newBCCSPHybridCertificateSigner(csp bccsp.BCCSP, key bccsp.Key) (bccspHybridCertificateSigner, error) {
	publicKey, err := key.PublicKey()
	if err != nil {
		return bccspHybridCertificateSigner{}, err
	}
	publicKeyDER, err := publicKey.Bytes()
	if err != nil {
		return bccspHybridCertificateSigner{}, err
	}
	parsedKey, err := oqs.ParsePKIXPublicKey(publicKeyDER)
	if err != nil {
		return bccspHybridCertificateSigner{}, err
	}
	hybridPublicKey, ok := parsedKey.(*oqs.PublicKey)
	if !ok {
		return bccspHybridCertificateSigner{}, errors.New("BCCSP key is not an ML-DSA-65 key")
	}
	return bccspHybridCertificateSigner{
		csp:       csp,
		key:       key,
		publicKey: *hybridPublicKey,
	}, nil
}

func cryptoPublicKey(key bccsp.Key) (crypto.PublicKey, error) {
	publicKey, err := key.PublicKey()
	if err != nil {
		return nil, err
	}
	publicKeyDER, err := publicKey.Bytes()
	if err != nil {
		return nil, err
	}
	return x509.ParsePKIXPublicKey(publicKeyDER)
}

func hybridMSPCertificateTemplate(serial int64, commonName string, isCA bool) *x509.Certificate {
	keyUsage := x509.KeyUsageDigitalSignature
	if isCA {
		keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		SubjectKeyId:          []byte{byte(serial), 0x42},
	}
	if isCA {
		template.MaxPathLen = 1
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	}
	return template
}
