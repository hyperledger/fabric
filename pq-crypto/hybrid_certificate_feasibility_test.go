/*
SPDX-License-Identifier: Apache-2.0
*/

package oqs

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type feasibilityProfileRegistryEntry struct {
	version        int
	profileKeyOID  asn1.ObjectIdentifier
	pqSignatureOID asn1.ObjectIdentifier
}

type feasibilityProfile struct {
	Version             int
	SubjectPublicKeyDER []byte
	SignatureAlgorithm  pkix.AlgorithmIdentifier
}

type feasibilityDERValue struct {
	tag      byte
	full     []byte
	content  []byte
	headerLn int
	offset   int
}

func TestFullTBSCertificatePQFeasibility(t *testing.T) {
	registeredProfile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)
	profile := feasibilityProfileRegistryEntry{
		version:        registeredProfile.version,
		profileKeyOID:  registeredProfile.profileKeyOID,
		pqSignatureOID: registeredProfile.pqSignatureOID,
	}
	rootClassical := feasibilityECDSAKey(1)
	leafClassical := feasibilityECDSAKey(2)
	rootPQ := feasibilityMLDSAKey(t, 0x2a)
	leafPQ := feasibilityMLDSAKey(t, 0x3b)

	require.NotEqual(t, rootPQ.Pk, leafPQ.Pk)
	require.NotEqual(t, rootClassical.X, leafClassical.X)

	rootTemplate := feasibilityCertificateTemplate(1, "PQ2 feasibility root", rootClassical, true)
	root, rootTBS := issueFeasibilityCertificate(t, profile, rootTemplate, rootTemplate, rootClassical, rootPQ.PublicKey, rootPQ)

	leafTemplate := feasibilityCertificateTemplate(2, "PQ2 feasibility leaf", leafClassical, false)
	leaf, leafTBS := issueFeasibilityCertificate(t, profile, leafTemplate, root, rootClassical, leafPQ.PublicKey, rootPQ)

	require.NoError(t, root.CheckSignatureFrom(root))
	require.NoError(t, leaf.CheckSignatureFrom(root))

	rootPool := x509.NewCertPool()
	rootPool.AddCert(root)
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:       rootPool,
		CurrentTime: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, chains, 1)
	require.Len(t, chains[0], 2)

	requirePQCertificateSignature(t, profile, root, root, rootPQ.PublicKey, rootTBS)
	requirePQCertificateSignature(t, profile, leaf, root, rootPQ.PublicKey, leafTBS)
	requireCoveredTBSMutationsFail(t, profile, leaf, rootPQ.PublicKey, leafTBS)

	fixtureHex, err := os.ReadFile("testdata/pactway-hybrid-v1-leaf-tbs.der.hex")
	require.NoError(t, err)
	fixture, err := hex.DecodeString(strings.TrimSpace(string(fixtureHex)))
	require.NoError(t, err)
	require.Equal(t, fixture, leafTBS)
	t.Logf("leaf tbs sha256 fixture candidate: %x", sha256.Sum256(leafTBS))
}

func TestFeasibilityDERLengthBoundaries(t *testing.T) {
	for _, length := range []int{0, 1, 127, 128, 255, 256, 65535, 65536} {
		content := bytes.Repeat([]byte{0xa5}, length)
		encoded := feasibilityDERWrap(0x04, content)
		value, rest, err := feasibilityDERRead(encoded)
		require.NoError(t, err)
		require.Empty(t, rest)
		require.Equal(t, byte(0x04), value.tag)
		require.Equal(t, content, value.content)
	}
}

func feasibilityCertificateTemplate(serial int64, commonName string, key *ecdsa.PrivateKey, isCA bool) *x509.Certificate {
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"Pactway PQ2 Feasibility"}},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		BasicConstraintsValid: true,
		PublicKey:             &key.PublicKey,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if isCA {
		template.IsCA = true
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	return template
}

func issueFeasibilityCertificate(
	t *testing.T,
	profile feasibilityProfileRegistryEntry,
	template, parent *x509.Certificate,
	classicalIssuer *ecdsa.PrivateKey,
	subjectPQ PublicKey,
	issuerPQ SecretKey,
) (*x509.Certificate, []byte) {
	t.Helper()
	subjectSPKI, err := MarshalPKIXPublicKey(&subjectPQ)
	require.NoError(t, err)
	profileDER, err := asn1.Marshal(feasibilityProfile{
		Version:             profile.version,
		SubjectPublicKeyDER: subjectSPKI,
		SignatureAlgorithm:  pkix.AlgorithmIdentifier{Algorithm: oidMLDSA65},
	})
	require.NoError(t, err)

	placeholder, err := Sign(issuerPQ, []byte("placeholder signature length"))
	require.NoError(t, err)
	placeholderDER, err := asn1.Marshal(asn1.BitString{Bytes: placeholder, BitLength: len(placeholder) * 8})
	require.NoError(t, err)

	template.ExtraExtensions = []pkix.Extension{
		{Id: profile.profileKeyOID, Value: profileDER},
		{Id: profile.pqSignatureOID, Value: placeholderDER},
	}
	placeholderCertificateDER, err := x509.CreateCertificate(rand.Reader, template, parent, template.PublicKey, classicalIssuer)
	require.NoError(t, err)
	placeholderCertificate, err := x509.ParseCertificate(placeholderCertificateDER)
	require.NoError(t, err)
	unsignedTBS, err := removeFeasibilitySignatureExtension(placeholderCertificate.RawTBSCertificate, profile.pqSignatureOID)
	require.NoError(t, err)
	pqMessage := feasibilitySignedInput(unsignedTBS)
	signature, err := Sign(issuerPQ, pqMessage)
	require.NoError(t, err)
	signatureDER, err := asn1.Marshal(asn1.BitString{Bytes: signature, BitLength: len(signature) * 8})
	require.NoError(t, err)
	template.ExtraExtensions[1].Value = signatureDER

	certificateDER, err := x509.CreateCertificate(rand.Reader, template, parent, template.PublicKey, classicalIssuer)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(certificateDER)
	require.NoError(t, err)
	reconstructedTBS, err := removeFeasibilitySignatureExtension(certificate.RawTBSCertificate, profile.pqSignatureOID)
	require.NoError(t, err)
	require.Equal(t, unsignedTBS, reconstructedTBS)
	require.Equal(t, pqMessage, feasibilitySignedInput(reconstructedTBS))
	return certificate, reconstructedTBS
}

func requirePQCertificateSignature(
	t *testing.T,
	profile feasibilityProfileRegistryEntry,
	certificate, issuer *x509.Certificate,
	issuerPQ PublicKey,
	unsignedTBS []byte,
) {
	t.Helper()
	profileExtension, signatureExtension := feasibilityExtensions(t, profile, certificate.Extensions)
	var profileValue feasibilityProfile
	rest, err := asn1.Unmarshal(profileExtension.Value, &profileValue)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, profile.version, profileValue.Version)
	require.Empty(t, profileValue.SignatureAlgorithm.Parameters.FullBytes)

	subjectKeyValue, err := ParsePKIXPublicKey(profileValue.SubjectPublicKeyDER)
	require.NoError(t, err)
	subjectPQ, ok := subjectKeyValue.(*PublicKey)
	require.True(t, ok)
	require.Len(t, subjectPQ.Pk, 1952)

	var bitString asn1.BitString
	rest, err = asn1.Unmarshal(signatureExtension.Value, &bitString)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, len(bitString.Bytes)*8, bitString.BitLength)
	require.True(t, profileValue.SignatureAlgorithm.Algorithm.Equal(oidMLDSA65))
	valid, err := Verify(issuerPQ, bitString.Bytes, feasibilitySignedInput(unsignedTBS))
	require.NoError(t, err)
	require.True(t, valid)
	require.NoError(t, certificate.CheckSignatureFrom(issuer))
}

func feasibilityExtensions(
	t *testing.T,
	profile feasibilityProfileRegistryEntry,
	extensions []pkix.Extension,
) (pkix.Extension, pkix.Extension) {
	t.Helper()
	var profileExtension, signatureExtension pkix.Extension
	profileCount, signatureCount := 0, 0
	for _, extension := range extensions {
		switch {
		case extension.Id.Equal(profile.profileKeyOID):
			profileExtension = extension
			profileCount++
		case extension.Id.Equal(profile.pqSignatureOID):
			signatureExtension = extension
			signatureCount++
		}
	}
	require.Equal(t, 1, profileCount)
	require.Equal(t, 1, signatureCount)
	return profileExtension, signatureExtension
}

func requireCoveredTBSMutationsFail(
	t *testing.T,
	profile feasibilityProfileRegistryEntry,
	certificate *x509.Certificate,
	issuerPQ PublicKey,
	unsignedTBS []byte,
) {
	t.Helper()
	fields, err := feasibilityDERChildren(unsignedTBS, 0x30)
	require.NoError(t, err)
	require.NotEmpty(t, fields)
	for index, field := range fields {
		require.NotEmpty(t, field.content)
		mutated := append([]byte(nil), unsignedTBS...)
		mutated[field.offset+field.headerLn] ^= 1
		_, signatureExtension := feasibilityExtensions(t, profile, certificate.Extensions)
		var signature asn1.BitString
		rest, err := asn1.Unmarshal(signatureExtension.Value, &signature)
		require.NoError(t, err)
		require.Empty(t, rest)
		valid, err := Verify(issuerPQ, signature.Bytes, feasibilitySignedInput(mutated))
		require.Errorf(t, err, "modified TBSCertificate field %d verified", index)
		require.False(t, valid)
	}
}

func feasibilitySignedInput(unsignedTBS []byte) []byte {
	message := append([]byte(hybridCertificateDomain), 0)
	return append(message, unsignedTBS...)
}

func removeFeasibilitySignatureExtension(tbs []byte, signatureOID asn1.ObjectIdentifier) ([]byte, error) {
	fields, err := feasibilityDERChildren(tbs, 0x30)
	if err != nil {
		return nil, err
	}
	extensionIndex := -1
	var extensionValues []feasibilityDERValue
	for index, field := range fields {
		if field.tag != 0xa3 {
			continue
		}
		if extensionIndex != -1 {
			return nil, stderrors.New("duplicate TBSCertificate extensions field")
		}
		extensionIndex = index
		extensionValues, err = feasibilityDERChildren(field.content, 0x30)
		if err != nil {
			return nil, err
		}
	}
	if extensionIndex == -1 {
		return nil, stderrors.New("TBSCertificate has no extensions")
	}

	filtered := make([][]byte, 0, len(extensionValues))
	signatureCount := 0
	for _, extensionDER := range extensionValues {
		var extension pkix.Extension
		rest, err := asn1.Unmarshal(extensionDER.full, &extension)
		if err != nil {
			return nil, fmt.Errorf("decode extension: %w", err)
		}
		if len(rest) != 0 {
			return nil, stderrors.New("trailing data in extension")
		}
		if extension.Id.Equal(signatureOID) {
			signatureCount++
			continue
		}
		filtered = append(filtered, extensionDER.full)
	}
	if signatureCount != 1 {
		return nil, fmt.Errorf("expected exactly one PQ signature extension, got %d", signatureCount)
	}

	encodedExtensions := feasibilityDERWrap(0x30, bytes.Join(filtered, nil))
	fields[extensionIndex].full = feasibilityDERWrap(0xa3, encodedExtensions)
	content := make([]byte, 0, len(tbs))
	for _, field := range fields {
		content = append(content, field.full...)
	}
	return feasibilityDERWrap(0x30, content), nil
}

func feasibilityDERChildren(der []byte, expectedTag byte) ([]feasibilityDERValue, error) {
	value, rest, err := feasibilityDERRead(der)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 || value.tag != expectedTag {
		return nil, stderrors.New("unexpected or trailing DER value")
	}
	var children []feasibilityDERValue
	contentOffset := value.headerLn
	for content := value.content; len(content) > 0; {
		child, remaining, err := feasibilityDERRead(content)
		if err != nil {
			return nil, err
		}
		child.offset += contentOffset
		children = append(children, child)
		content = remaining
		contentOffset += len(child.full)
	}
	return children, nil
}

func feasibilityDERRead(der []byte) (feasibilityDERValue, []byte, error) {
	if len(der) < 2 {
		return feasibilityDERValue{}, nil, stderrors.New("truncated DER value")
	}
	tag := der[0]
	if tag&0x1f == 0x1f {
		return feasibilityDERValue{}, nil, stderrors.New("high-tag-number DER is unsupported")
	}
	lengthByte := der[1]
	headerLength, contentLength := 2, 0
	switch {
	case lengthByte < 0x80:
		contentLength = int(lengthByte)
	case lengthByte == 0x80:
		return feasibilityDERValue{}, nil, stderrors.New("indefinite length is not DER")
	default:
		lengthOctets := int(lengthByte & 0x7f)
		if lengthOctets == 0 || lengthOctets > 4 || len(der) < 2+lengthOctets || der[2] == 0 {
			return feasibilityDERValue{}, nil, stderrors.New("invalid DER length")
		}
		headerLength += lengthOctets
		for _, octet := range der[2:headerLength] {
			contentLength = contentLength<<8 | int(octet)
		}
		if contentLength < 0x80 {
			return feasibilityDERValue{}, nil, stderrors.New("non-minimal DER length")
		}
	}
	if contentLength < 0 || len(der) < headerLength+contentLength {
		return feasibilityDERValue{}, nil, stderrors.New("truncated DER content")
	}
	end := headerLength + contentLength
	return feasibilityDERValue{
		tag:      tag,
		full:     der[:end],
		content:  der[headerLength:end],
		headerLn: headerLength,
	}, der[end:], nil
}

func feasibilityDERWrap(tag byte, content []byte) []byte {
	encoded := []byte{tag}
	switch length := len(content); {
	case length < 0x80:
		encoded = append(encoded, byte(length))
	case length <= 0xff:
		encoded = append(encoded, 0x81, byte(length))
	case length <= 0xffff:
		encoded = append(encoded, 0x82, byte(length>>8), byte(length))
	case length <= 0xffffff:
		encoded = append(encoded, 0x83, byte(length>>16), byte(length>>8), byte(length))
	default:
		encoded = append(encoded, 0x84, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}
	return append(encoded, content...)
}

func feasibilityECDSAKey(d int64) *ecdsa.PrivateKey {
	scalar := big.NewInt(d)
	x, y := elliptic.P256().ScalarBaseMult(scalar.Bytes())
	publicKey := ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	return &ecdsa.PrivateKey{
		PublicKey: publicKey,
		D:         scalar,
	}
}

func feasibilityMLDSAKey(t *testing.T, seedByte byte) SecretKey {
	t.Helper()
	seed := bytes.Repeat([]byte{seedByte}, 32)
	privateKey, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
	require.NoError(t, err)
	publicKey := PublicKey{
		Pk:  privateKey.PublicKey().Bytes(),
		Sig: OQSSigInfo{Algorithm: MLDSA65ProfileV1},
	}
	return SecretKey{
		Sk:        privateKey.Bytes(),
		PublicKey: publicKey,
	}
}
