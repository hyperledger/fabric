/*
SPDX-License-Identifier: Apache-2.0
*/

package oqs

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHybridCertificateV1GenerationAndIssuerChain(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)

	rootClassical := feasibilityECDSAKey(7)
	intermediateClassical := feasibilityECDSAKey(8)
	leafClassical := feasibilityECDSAKey(9)
	rootPQ := feasibilityMLDSAKey(t, 0x61)
	intermediatePQ := feasibilityMLDSAKey(t, 0x72)
	leafPQ := feasibilityMLDSAKey(t, 0x83)

	rootTemplate := feasibilityCertificateTemplate(11, "PQ2 v1 root", rootClassical, true)
	rootTemplate.SerialNumber = nil
	rootDER, err := CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical,
		rootPQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)
	require.NotNil(t, root.SerialNumber)
	require.Positive(t, root.SerialNumber.Sign())

	intermediateTemplate := feasibilityCertificateTemplate(12, "PQ2 v1 intermediate", intermediateClassical, true)
	intermediateDER, err := CreateHybridCertificate(
		rand.Reader, profile, intermediateTemplate, root, &intermediateClassical.PublicKey, rootClassical,
		intermediatePQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	intermediate, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	leafTemplate := feasibilityCertificateTemplate(13, "PQ2 v1 leaf", leafClassical, false)
	leafDER, err := CreateHybridCertificate(
		rand.Reader, profile, leafTemplate, intermediate, &leafClassical.PublicKey, intermediateClassical,
		leafPQ.PublicKey, intermediatePQ,
	)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(intermediate)
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, chains, 1)
	require.Len(t, chains[0], 3)
	require.NoError(t, ValidateHybridCertificateChain(chains[0], profile, true))
	require.NoError(t, ValidateHybridCertificateChain(chains[0], profile, false))

	info, found, err := ParseHybridCertificate(leaf, profile)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, leafPQ.Pk, info.SubjectPublicKey.Pk)
	require.Equal(t, 1, info.Version)
	require.True(t, info.SignatureAlgorithm.Algorithm.Equal(oidMLDSA65))
	require.Empty(t, info.SignatureAlgorithm.Parameters.FullBytes)
}

func TestHybridCertificateV1RequiresSelfIssuedTrustAnchorWhenRequired(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)

	externalIssuerClassical := feasibilityECDSAKey(97)
	externalIssuerPQ := feasibilityMLDSAKey(t, 0x11)
	externalRootTemplate := feasibilityCertificateTemplate(101, "external issuer", externalIssuerClassical, true)
	externalRootDER, err := CreateHybridCertificate(
		rand.Reader, profile, externalRootTemplate, externalRootTemplate,
		&externalIssuerClassical.PublicKey, externalIssuerClassical,
		externalIssuerPQ.PublicKey, externalIssuerPQ,
	)
	require.NoError(t, err)
	externalRoot, err := x509.ParseCertificate(externalRootDER)
	require.NoError(t, err)

	anchorClassical := feasibilityECDSAKey(98)
	anchorPQ := feasibilityMLDSAKey(t, 0x22)
	anchorTemplate := feasibilityCertificateTemplate(102, "configured trust anchor", anchorClassical, true)
	anchorDER, err := CreateHybridCertificate(
		rand.Reader, profile, anchorTemplate, externalRoot,
		&anchorClassical.PublicKey, externalIssuerClassical,
		anchorPQ.PublicKey, externalIssuerPQ,
	)
	require.NoError(t, err)
	anchor, err := x509.ParseCertificate(anchorDER)
	require.NoError(t, err)
	require.NotEqual(t, anchor.RawSubject, anchor.RawIssuer)

	leafClassical := feasibilityECDSAKey(99)
	leafPQ := feasibilityMLDSAKey(t, 0x33)
	leafTemplate := feasibilityCertificateTemplate(103, "leaf", leafClassical, false)
	leafDER, err := CreateHybridCertificate(
		rand.Reader, profile, leafTemplate, anchor,
		&leafClassical.PublicKey, anchorClassical,
		leafPQ.PublicKey, anchorPQ,
	)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(anchor)
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:       roots,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		CurrentTime: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, chains, 1)
	require.ErrorContains(t, ValidateHybridCertificateChain(chains[0], profile, true), "trust anchor must be self-issued")
	require.NoError(t, ValidateHybridCertificateChain(chains[0], profile, false))
}

func TestHybridCertificateV1RejectsWrongIssuerAndPartialChains(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)

	rootClassical := feasibilityECDSAKey(17)
	intermediateClassical := feasibilityECDSAKey(18)
	leafClassical := feasibilityECDSAKey(19)
	rootPQ := feasibilityMLDSAKey(t, 0x91)
	intermediatePQ := feasibilityMLDSAKey(t, 0xa2)
	leafPQ := feasibilityMLDSAKey(t, 0xb3)

	rootTemplate := feasibilityCertificateTemplate(21, "PQ2 wrong-issuer root", rootClassical, true)
	rootDER, err := CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical,
		rootPQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	intermediateTemplate := feasibilityCertificateTemplate(22, "PQ2 wrong-issuer intermediate", intermediateClassical, true)
	intermediateDER, err := CreateHybridCertificate(
		rand.Reader, profile, intermediateTemplate, root, &intermediateClassical.PublicKey, rootClassical,
		intermediatePQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	intermediate, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	leafTemplate := feasibilityCertificateTemplate(23, "PQ2 wrong-issuer leaf", leafClassical, false)
	leafDER, err := CreateHybridCertificate(
		rand.Reader, profile, leafTemplate, intermediate, &leafClassical.PublicKey, intermediateClassical,
		leafPQ.PublicKey, intermediatePQ,
	)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	unsignedTBS, err := removeHybridSignatureExtension(leaf.RawTBSCertificate, profile.pqSignatureOID)
	require.NoError(t, err)
	wrongSignature, err := Sign(rootPQ, hybridSignedInput(unsignedTBS))
	require.NoError(t, err)
	wrongSignatureValue, err := encodeHybridSignature(wrongSignature)
	require.NoError(t, err)
	wronglySignedLeaf := *leaf
	wronglySignedLeaf.Extensions = append([]pkix.Extension(nil), leaf.Extensions...)
	for index := range wronglySignedLeaf.Extensions {
		if wronglySignedLeaf.Extensions[index].Id.Equal(profile.pqSignatureOID) {
			wronglySignedLeaf.Extensions[index].Value = wrongSignatureValue
		}
	}
	require.ErrorContains(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{&wronglySignedLeaf, intermediate, root}, profile, true,
	), "issuer ML-DSA-65")

	classicalRootKey := feasibilityECDSAKey(27)
	classicalLeafKey := feasibilityECDSAKey(28)
	classicalRootTemplate := feasibilityCertificateTemplate(31, "classical root", classicalRootKey, true)
	classicalRootDER, err := x509.CreateCertificate(rand.Reader, classicalRootTemplate, classicalRootTemplate, &classicalRootKey.PublicKey, classicalRootKey)
	require.NoError(t, err)
	classicalRoot, err := x509.ParseCertificate(classicalRootDER)
	require.NoError(t, err)
	classicalLeafTemplate := feasibilityCertificateTemplate(32, "classical leaf", classicalLeafKey, false)
	classicalLeafDER, err := x509.CreateCertificate(rand.Reader, classicalLeafTemplate, classicalRoot, &classicalLeafKey.PublicKey, classicalRootKey)
	require.NoError(t, err)
	classicalLeaf, err := x509.ParseCertificate(classicalLeafDER)
	require.NoError(t, err)

	require.NoError(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{classicalLeaf, classicalRoot}, profile, false,
	))
	require.ErrorContains(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{classicalLeaf, classicalRoot}, profile, true,
	), "classical-only")

	require.ErrorContains(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{leaf, classicalRoot}, profile, false,
	), "incomplete")
}

func TestHybridCertificateV1RejectsMalformedAndDuplicateExtensions(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)
	rootClassical := feasibilityECDSAKey(37)
	rootPQ := feasibilityMLDSAKey(t, 0xc4)
	rootTemplate := feasibilityCertificateTemplate(41, "PQ2 malformed root", rootClassical, true)
	rootDER, err := CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical,
		rootPQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	_, found, err := ParseHybridCertificate(root, profile)
	require.NoError(t, err)
	require.True(t, found)

	duplicate := *root
	duplicate.Extensions = append(append([]pkix.Extension(nil), root.Extensions...), pkix.Extension{
		Id:    profile.profileKeyOID,
		Value: []byte{0x30, 0x00},
	})
	_, _, err = ParseHybridCertificate(&duplicate, profile)
	require.ErrorContains(t, err, "exactly one")

	malformed := *root
	malformed.Extensions = append([]pkix.Extension(nil), root.Extensions...)
	for index := range malformed.Extensions {
		if malformed.Extensions[index].Id.Equal(profile.profileKeyOID) {
			malformed.Extensions[index].Value = []byte{0x30, 0x80, 0x00, 0x00}
		}
	}
	_, _, err = ParseHybridCertificate(&malformed, profile)
	require.Error(t, err)

	unsupportedVersion := *root
	unsupportedVersion.Extensions = append([]pkix.Extension(nil), root.Extensions...)
	for index := range unsupportedVersion.Extensions {
		if unsupportedVersion.Extensions[index].Id.Equal(profile.profileKeyOID) {
			var value hybridCertificateProfileValue
			rest, unmarshalErr := asn1.Unmarshal(unsupportedVersion.Extensions[index].Value, &value)
			require.NoError(t, unmarshalErr)
			require.Empty(t, rest)
			value.Version = 2
			unsupportedVersion.Extensions[index].Value, err = asn1.Marshal(value)
			require.NoError(t, err)
		}
	}
	_, _, err = ParseHybridCertificate(&unsupportedVersion, profile)
	require.ErrorContains(t, err, "unsupported hybrid certificate profile version")

	tbsDER := root.RawTBSCertificate
	require.Greater(t, len(tbsDER), 4)
	require.Equal(t, byte(0x82), tbsDER[1])
	nonMinimalTBS := append([]byte{0x30, 0x83, 0x00}, tbsDER[2:]...)
	_, err = removeHybridSignatureExtension(nonMinimalTBS, profile.pqSignatureOID)
	require.ErrorContains(t, err, "non-minimal DER length")
}

func TestHybridCertificateV1RejectsMissingSignatureUnsupportedAlgorithmAlteredKeyAndWrongAnchor(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)

	rootClassical := feasibilityECDSAKey(67)
	intermediateClassical := feasibilityECDSAKey(68)
	leafClassical := feasibilityECDSAKey(69)
	rootPQ := feasibilityMLDSAKey(t, 0x19)
	intermediatePQ := feasibilityMLDSAKey(t, 0x2a)
	leafPQ := feasibilityMLDSAKey(t, 0x3b)

	rootTemplate := feasibilityCertificateTemplate(71, "PQ2 mutation root", rootClassical, true)
	rootDER, err := CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical,
		rootPQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	intermediateTemplate := feasibilityCertificateTemplate(72, "PQ2 mutation intermediate", intermediateClassical, true)
	intermediateDER, err := CreateHybridCertificate(
		rand.Reader, profile, intermediateTemplate, root, &intermediateClassical.PublicKey, rootClassical,
		intermediatePQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	intermediate, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	leafTemplate := feasibilityCertificateTemplate(73, "PQ2 mutation leaf", leafClassical, false)
	leafDER, err := CreateHybridCertificate(
		rand.Reader, profile, leafTemplate, intermediate, &leafClassical.PublicKey, intermediateClassical,
		leafPQ.PublicKey, intermediatePQ,
	)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	missingSignature := *leaf
	missingSignature.Extensions = append([]pkix.Extension(nil), leaf.Extensions...)
	missingSignature.Extensions = slices.DeleteFunc(missingSignature.Extensions, func(extension pkix.Extension) bool {
		return extension.Id.Equal(profile.pqSignatureOID)
	})
	_, _, err = ParseHybridCertificate(&missingSignature, profile)
	require.ErrorContains(t, err, "expected exactly one")

	unsupportedAlgorithm := *leaf
	unsupportedAlgorithm.Extensions = append([]pkix.Extension(nil), leaf.Extensions...)
	for index := range unsupportedAlgorithm.Extensions {
		if !unsupportedAlgorithm.Extensions[index].Id.Equal(profile.profileKeyOID) {
			continue
		}
		var value hybridCertificateProfileValue
		rest, unmarshalErr := asn1.Unmarshal(unsupportedAlgorithm.Extensions[index].Value, &value)
		require.NoError(t, unmarshalErr)
		require.Empty(t, rest)
		value.SignatureAlgorithm.Algorithm = asn1.ObjectIdentifier{1, 2, 3, 4}
		unsupportedAlgorithm.Extensions[index].Value, err = asn1.Marshal(value)
		require.NoError(t, err)
	}
	_, _, err = ParseHybridCertificate(&unsupportedAlgorithm, profile)
	require.ErrorContains(t, err, "unsupported hybrid certificate PQ signature algorithm")

	alteredIntermediatePQ := feasibilityMLDSAKey(t, 0x4c)
	alteredIntermediateTemplate := feasibilityCertificateTemplate(74, "PQ2 mutation intermediate", intermediateClassical, true)
	alteredIntermediateDER, err := CreateHybridCertificate(
		rand.Reader, profile, alteredIntermediateTemplate, root, &intermediateClassical.PublicKey, rootClassical,
		alteredIntermediatePQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	alteredIntermediate, err := x509.ParseCertificate(alteredIntermediateDER)
	require.NoError(t, err)
	require.ErrorContains(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{leaf, alteredIntermediate, root}, profile, true,
	), "issuer ML-DSA-65")

	wrongAnchorClassical := feasibilityECDSAKey(75)
	wrongAnchorPQ := feasibilityMLDSAKey(t, 0x5d)
	wrongAnchorTemplate := feasibilityCertificateTemplate(76, "PQ2 mutation root", wrongAnchorClassical, true)
	wrongAnchorDER, err := CreateHybridCertificate(
		rand.Reader, profile, wrongAnchorTemplate, wrongAnchorTemplate,
		&wrongAnchorClassical.PublicKey, wrongAnchorClassical, wrongAnchorPQ.PublicKey, wrongAnchorPQ,
	)
	require.NoError(t, err)
	wrongAnchor, err := x509.ParseCertificate(wrongAnchorDER)
	require.NoError(t, err)
	require.ErrorContains(t, ValidateHybridCertificateChain(
		[]*x509.Certificate{leaf, intermediate, wrongAnchor}, profile, true,
	), "classical issuer signature")
}

func TestTemporaryHybridProfileRequiresExplicitDevelopmentUse(t *testing.T) {
	_, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, ProfileUseProduction)
	require.ErrorContains(t, err, "development-only")

	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, ProfileUseIsolatedDevelopment)
	require.NoError(t, err)
	require.True(t, profile.DevelopmentOnly())
}

func TestHybridCertificateV1TBSReconstructionRejectsTrailingData(t *testing.T) {
	profile, err := HybridCertificateProfileByName(TemporaryHybridProfileV1, profileUseTest)
	require.NoError(t, err)
	rootClassical := feasibilityECDSAKey(47)
	rootPQ := feasibilityMLDSAKey(t, 0xd5)
	rootTemplate := feasibilityCertificateTemplate(51, "PQ2 TBS root", rootClassical, true)
	rootDER, err := CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical,
		rootPQ.PublicKey, rootPQ,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)
	tbsWithTrailingByte := append(bytes.Clone(root.RawTBSCertificate), 0x00)
	_, err = removeHybridSignatureExtension(tbsWithTrailingByte, profile.pqSignatureOID)
	require.ErrorContains(t, err, "trailing")
}

func TestLegacyValidatorUsesImmediateIssuerPQKey(t *testing.T) {
	rootClassical := feasibilityECDSAKey(57)
	intermediateClassical := feasibilityECDSAKey(58)
	leafClassical := feasibilityECDSAKey(59)
	rootPQ := feasibilityMLDSAKey(t, 0xe6)
	intermediatePQ := feasibilityMLDSAKey(t, 0xf7)
	leafPQ := feasibilityMLDSAKey(t, 0x18)

	rootTemplate := feasibilityCertificateTemplate(61, "legacy root", rootClassical, true)
	rootTemplate.ExtraExtensions = legacyKeyExtensions(t, rootPQ.PublicKey, &rootClassical.PublicKey, rootPQ)
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootClassical.PublicKey, rootClassical)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	intermediateTemplate := feasibilityCertificateTemplate(62, "legacy intermediate", intermediateClassical, true)
	intermediateTemplate.ExtraExtensions = legacyKeyExtensions(t, intermediatePQ.PublicKey, &intermediateClassical.PublicKey, rootPQ)
	intermediateDER, err := x509.CreateCertificate(
		rand.Reader, intermediateTemplate, root, &intermediateClassical.PublicKey, rootClassical,
	)
	require.NoError(t, err)
	intermediate, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	validLeafTemplate := feasibilityCertificateTemplate(63, "legacy valid leaf", leafClassical, false)
	validLeafTemplate.ExtraExtensions = legacyKeyExtensions(t, leafPQ.PublicKey, &leafClassical.PublicKey, intermediatePQ)
	validLeafDER, err := x509.CreateCertificate(
		rand.Reader, validLeafTemplate, intermediate, &leafClassical.PublicKey, intermediateClassical,
	)
	require.NoError(t, err)
	validLeaf, err := x509.ParseCertificate(validLeafDER)
	require.NoError(t, err)
	require.NoError(t, Validate([]*x509.Certificate{validLeaf, intermediate, root}))

	wronglySignedLeafTemplate := feasibilityCertificateTemplate(64, "legacy wrong issuer leaf", leafClassical, false)
	wronglySignedLeafTemplate.ExtraExtensions = legacyKeyExtensions(
		t, leafPQ.PublicKey, &leafClassical.PublicKey, rootPQ,
	)
	wronglySignedLeafDER, err := x509.CreateCertificate(
		rand.Reader, wronglySignedLeafTemplate, intermediate, &leafClassical.PublicKey, intermediateClassical,
	)
	require.NoError(t, err)
	wronglySignedLeaf, err := x509.ParseCertificate(wronglySignedLeafDER)
	require.NoError(t, err)
	require.Error(t, Validate([]*x509.Certificate{wronglySignedLeaf, intermediate, root}))
}

func legacyKeyExtensions(t *testing.T, subjectPQ PublicKey, classical crypto.PublicKey, issuerPQ SecretKey) []pkix.Extension {
	t.Helper()
	classicalDER, err := x509.MarshalPKIXPublicKey(classical)
	require.NoError(t, err)
	keyExtension, err := buildSubjectAltPublicKeyInfoExt(&subjectPQ)
	require.NoError(t, err)
	signatureExtension, err := buildAltSignatureValueExt(
		subjectPQ.Pk, classicalDER, &mockSigner{sk: issuerPQ},
	)
	require.NoError(t, err)
	return []pkix.Extension{keyExtension, signatureExtension}
}
