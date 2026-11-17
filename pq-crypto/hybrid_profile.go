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
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"slices"
	"strings"
)

const (
	TemporaryHybridProfileV1 = "rfc5612-test-only-v1"
	hybridCertificateDomain  = "Pactway-Hybrid-Certificate-v1"
	hybridCertificateVersion = 1
	hybridSignatureLength    = 3309
	profileUseTest           = ProfileUse(2)
)

type ProfileUse uint8

const (
	ProfileUseProduction ProfileUse = iota
	ProfileUseIsolatedDevelopment
	ProfileUseTest
)

type HybridCertificateProfile struct {
	name            string
	version         int
	namespaceOID    asn1.ObjectIdentifier
	profileKeyOID   asn1.ObjectIdentifier
	pqSignatureOID  asn1.ObjectIdentifier
	developmentOnly bool
	use             ProfileUse
}

type hybridProfileDefinition struct {
	version         int
	namespaceOID    asn1.ObjectIdentifier
	profileKeyOID   asn1.ObjectIdentifier
	pqSignatureOID  asn1.ObjectIdentifier
	developmentOnly bool
}

var hybridProfileRegistry = map[string]hybridProfileDefinition{
	TemporaryHybridProfileV1: {
		version:         hybridCertificateVersion,
		namespaceOID:    asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 32473, 1},
		profileKeyOID:   asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 32473, 1, 1},
		pqSignatureOID:  asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 32473, 1, 2},
		developmentOnly: true,
	},
}

type hybridCertificateProfileValue struct {
	Version             int
	SubjectPublicKeyDER []byte
	SignatureAlgorithm  pkix.AlgorithmIdentifier
}

type HybridCertificateInfo struct {
	Version            int
	SubjectPublicKey   PublicKey
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
}

type HybridCertificateSigner interface {
	PublicKey() PublicKey
	SignHybrid(message []byte) ([]byte, error)
}

type secretHybridCertificateSigner struct {
	key SecretKey
}

func (signer secretHybridCertificateSigner) PublicKey() PublicKey {
	return signer.key.PublicKey
}

func (signer secretHybridCertificateSigner) SignHybrid(message []byte) ([]byte, error) {
	return Sign(signer.key, message)
}

type hybridDERValue struct {
	tag      byte
	full     []byte
	content  []byte
	headerLn int
	offset   int
}

// HybridCertificateProfileByName returns one explicitly selected profile. The
// RFC 5612 profile can only be selected for tests or isolated development.
func HybridCertificateProfileByName(name string, use ProfileUse) (HybridCertificateProfile, error) {
	definition, ok := hybridProfileRegistry[name]
	if !ok {
		return HybridCertificateProfile{}, fmt.Errorf("unsupported hybrid certificate profile %q", name)
	}
	if use != ProfileUseProduction && use != ProfileUseIsolatedDevelopment && use != profileUseTest {
		return HybridCertificateProfile{}, errors.New("invalid hybrid certificate profile use")
	}
	if definition.developmentOnly && use == ProfileUseProduction {
		return HybridCertificateProfile{}, fmt.Errorf("hybrid certificate profile %q is development-only", name)
	}

	return HybridCertificateProfile{
		name:            name,
		version:         definition.version,
		namespaceOID:    slices.Clone(definition.namespaceOID),
		profileKeyOID:   slices.Clone(definition.profileKeyOID),
		pqSignatureOID:  slices.Clone(definition.pqSignatureOID),
		developmentOnly: definition.developmentOnly,
		use:             use,
	}, nil
}

// HybridCertificateProfileFromEnvironment requires an explicit profile
// selection. Test-only profiles are available only in test binaries or when
// isolated development is explicitly enabled.
func HybridCertificateProfileFromEnvironment() (HybridCertificateProfile, error) {
	name := os.Getenv("PACTWAY_HYBRID_PROFILE")
	if name == "" {
		return HybridCertificateProfile{}, errors.New("PACTWAY_HYBRID_PROFILE must explicitly select a certificate profile")
	}
	return HybridCertificateProfileByName(name, CurrentHybridProfileUse())
}

func CurrentHybridProfileUse() ProfileUse {
	if strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe") {
		return profileUseTest
	}
	if os.Getenv("PACTWAY_ISOLATED_DEVELOPMENT") == "true" {
		return ProfileUseIsolatedDevelopment
	}
	return ProfileUseProduction
}

func HasPactwayHybridExtensions(extensions []pkix.Extension, profile HybridCertificateProfile) bool {
	if len(profile.namespaceOID) == 0 {
		return false
	}
	for _, extension := range extensions {
		if isWithinOIDNamespace(extension.Id, profile.namespaceOID) {
			return true
		}
	}
	return false
}

func HasRegisteredHybridCertificateExtensions(extensions []pkix.Extension) bool {
	for _, extension := range extensions {
		for _, definition := range hybridProfileRegistry {
			if isWithinOIDNamespace(extension.Id, definition.namespaceOID) {
				return true
			}
		}
	}
	return false
}

func isWithinOIDNamespace(candidate, namespace asn1.ObjectIdentifier) bool {
	if len(candidate) < len(namespace) {
		return false
	}
	for index := range namespace {
		if candidate[index] != namespace[index] {
			return false
		}
	}
	return true
}

func (profile HybridCertificateProfile) Name() string {
	return profile.name
}

func (profile HybridCertificateProfile) Version() int {
	return profile.version
}

func (profile HybridCertificateProfile) DevelopmentOnly() bool {
	return profile.developmentOnly
}

func (profile HybridCertificateProfile) ProfileKeyOID() asn1.ObjectIdentifier {
	return slices.Clone(profile.profileKeyOID)
}

func (profile HybridCertificateProfile) PQSignatureOID() asn1.ObjectIdentifier {
	return slices.Clone(profile.pqSignatureOID)
}

// CreateHybridCertificate constructs the final X.509 certificate with the
// PQ signature covering the complete TBSCertificate except its own extension.
func CreateHybridCertificate(
	random io.Reader,
	profile HybridCertificateProfile,
	template, parent *x509.Certificate,
	publicKey crypto.PublicKey,
	classicalSigner crypto.Signer,
	subjectPQ PublicKey,
	issuerPQ SecretKey,
) ([]byte, error) {
	return CreateHybridCertificateWithPQSigner(
		random, profile, template, parent, publicKey, classicalSigner, subjectPQ,
		secretHybridCertificateSigner{key: issuerPQ},
	)
}

func CreateHybridCertificateWithPQSigner(
	random io.Reader,
	profile HybridCertificateProfile,
	template, parent *x509.Certificate,
	publicKey crypto.PublicKey,
	classicalSigner crypto.Signer,
	subjectPQ PublicKey,
	issuerPQSigner HybridCertificateSigner,
) ([]byte, error) {
	if profile.name == "" || profile.version != hybridCertificateVersion {
		return nil, errors.New("invalid or unsupported hybrid certificate profile")
	}
	if profile.developmentOnly && profile.use == ProfileUseProduction {
		return nil, fmt.Errorf("hybrid certificate profile %q is development-only", profile.name)
	}
	if template == nil || parent == nil || publicKey == nil || classicalSigner == nil || issuerPQSigner == nil {
		return nil, errors.New("hybrid certificate template, issuer, public key, and classical and PQ signers are required")
	}
	if random == nil {
		random = rand.Reader
	}
	issuerPQ := issuerPQSigner.PublicKey()
	if err := requireProfile(subjectPQ.Sig.Algorithm); err != nil {
		return nil, err
	}
	if err := requireProfile(issuerPQ.Sig.Algorithm); err != nil {
		return nil, err
	}
	if len(parent.Raw) == 0 {
		if !bytes.Equal(subjectPQ.Pk, issuerPQ.Pk) {
			return nil, errors.New("self-signed hybrid certificate subject and issuer ML-DSA-65 keys differ")
		}
	} else {
		parentInfo, found, err := ParseHybridCertificate(parent, profile)
		if err != nil {
			return nil, fmt.Errorf("parse issuer hybrid certificate: %w", err)
		}
		if !found || !bytes.Equal(parentInfo.SubjectPublicKey.Pk, issuerPQ.Pk) {
			return nil, errors.New("issuer ML-DSA-65 private key does not match the issuer certificate")
		}
	}
	classicalIssuerPublicKeyDER, err := x509.MarshalPKIXPublicKey(classicalSigner.Public())
	if err != nil {
		return nil, fmt.Errorf("marshal classical issuer public key: %w", err)
	}
	parentPublicKeyDER := parent.RawSubjectPublicKeyInfo
	if len(parent.Raw) == 0 {
		parentPublicKeyDER, err = x509.MarshalPKIXPublicKey(parent.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("marshal self-signed hybrid certificate public key: %w", err)
		}
	}
	if !bytes.Equal(classicalIssuerPublicKeyDER, parentPublicKeyDER) {
		return nil, errors.New("classical signing key does not match issuer certificate")
	}

	subjectSPKI, err := MarshalPKIXPublicKey(&subjectPQ)
	if err != nil {
		return nil, fmt.Errorf("marshal subject ML-DSA-65 public key: %w", err)
	}
	profileDER, err := asn1.Marshal(hybridCertificateProfileValue{
		Version:             profile.version,
		SubjectPublicKeyDER: subjectSPKI,
		SignatureAlgorithm:  pkix.AlgorithmIdentifier{Algorithm: oidMLDSA65},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal hybrid certificate profile extension: %w", err)
	}

	placeholderValue, err := encodeHybridSignature(make([]byte, hybridSignatureLength))
	if err != nil {
		return nil, fmt.Errorf("encode placeholder ML-DSA-65 signature: %w", err)
	}
	placeholder := pkix.Extension{
		Id:    profile.pqSignatureOID,
		Value: placeholderValue,
	}
	if err := rejectDuplicateHybridExtensions(profile, template, placeholder); err != nil {
		return nil, err
	}

	baseTemplate := *template
	baseTemplate.ExtraExtensions = append(slices.Clone(template.ExtraExtensions),
		pkix.Extension{Id: profile.profileKeyOID, Value: profileDER},
		placeholder,
	)
	placeholderDER, err := x509.CreateCertificate(random, &baseTemplate, parent, publicKey, classicalSigner)
	if err != nil {
		return nil, fmt.Errorf("create unsigned hybrid certificate: %w", err)
	}
	placeholderCert, err := x509.ParseCertificate(placeholderDER)
	if err != nil {
		return nil, fmt.Errorf("parse unsigned hybrid certificate: %w", err)
	}
	if placeholderCert.SerialNumber == nil {
		return nil, errors.New("unsigned hybrid certificate has no serial number")
	}
	unsignedTBS, err := removeHybridSignatureExtension(placeholderCert.RawTBSCertificate, profile.pqSignatureOID)
	if err != nil {
		return nil, fmt.Errorf("reconstruct unsigned TBSCertificate: %w", err)
	}

	signedInput := hybridSignedInput(unsignedTBS)
	pqSignature, err := issuerPQSigner.SignHybrid(signedInput)
	if err != nil {
		return nil, fmt.Errorf("sign TBSCertificate with issuer ML-DSA-65 key: %w", err)
	}
	if len(pqSignature) != hybridSignatureLength {
		return nil, fmt.Errorf("unexpected ML-DSA-65 signature length %d", len(pqSignature))
	}
	valid, err := Verify(issuerPQ, pqSignature, signedInput)
	if err != nil {
		return nil, fmt.Errorf("verify issuer ML-DSA-65 signing key: %w", err)
	}
	if !valid {
		return nil, errors.New("issuer ML-DSA-65 signer does not match its public key")
	}
	signatureValue, err := encodeHybridSignature(pqSignature)
	if err != nil {
		return nil, fmt.Errorf("encode ML-DSA-65 signature extension: %w", err)
	}

	finalTemplate := *template
	finalTemplate.SerialNumber = new(big.Int).Set(placeholderCert.SerialNumber)
	finalTemplate.ExtraExtensions = append(slices.Clone(template.ExtraExtensions),
		pkix.Extension{Id: profile.profileKeyOID, Value: profileDER},
		pkix.Extension{Id: profile.pqSignatureOID, Value: signatureValue},
	)
	certificateDER, err := x509.CreateCertificate(random, &finalTemplate, parent, publicKey, classicalSigner)
	if err != nil {
		return nil, fmt.Errorf("create final hybrid certificate: %w", err)
	}
	certificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		return nil, fmt.Errorf("parse final hybrid certificate: %w", err)
	}
	reconstructedTBS, err := removeHybridSignatureExtension(certificate.RawTBSCertificate, profile.pqSignatureOID)
	if err != nil {
		return nil, fmt.Errorf("reconstruct final TBSCertificate: %w", err)
	}
	if !bytes.Equal(unsignedTBS, reconstructedTBS) {
		return nil, errors.New("final TBSCertificate differs from the PQ-signed TBSCertificate")
	}
	return certificateDER, nil
}

// ParseHybridCertificate parses and strictly validates the Pactway profile
// and signature extensions. found is false only when neither extension exists.
func ParseHybridCertificate(certificate *x509.Certificate, profile HybridCertificateProfile) (info *HybridCertificateInfo, found bool, err error) {
	if certificate == nil || profile.name == "" {
		return nil, false, errors.New("certificate and hybrid profile are required")
	}
	if profile.developmentOnly && profile.use == ProfileUseProduction {
		return nil, false, fmt.Errorf("hybrid certificate profile %q is development-only", profile.name)
	}

	var profileExtension, signatureExtension *pkix.Extension
	profileCount, signatureCount := 0, 0
	for index := range certificate.Extensions {
		extension := &certificate.Extensions[index]
		switch {
		case extension.Id.Equal(profile.profileKeyOID):
			profileExtension = extension
			profileCount++
		case extension.Id.Equal(profile.pqSignatureOID):
			signatureExtension = extension
			signatureCount++
		case isWithinOIDNamespace(extension.Id, profile.namespaceOID):
			return nil, true, fmt.Errorf("unsupported Pactway hybrid certificate extension OID %s", extension.Id.String())
		}
	}
	if profileCount == 0 && signatureCount == 0 {
		return nil, false, nil
	}
	if profileCount != 1 || signatureCount != 1 {
		return nil, true, fmt.Errorf("expected exactly one profile/key and one PQ signature extension, got %d and %d", profileCount, signatureCount)
	}
	if profileExtension.Critical || signatureExtension.Critical {
		return nil, true, errors.New("Pactway Hybrid Certificate v1 extensions must be noncritical")
	}

	var value hybridCertificateProfileValue
	rest, err := asn1.Unmarshal(profileExtension.Value, &value)
	if err != nil {
		return nil, true, fmt.Errorf("decode profile/key extension: %w", err)
	}
	if len(rest) != 0 {
		return nil, true, errors.New("trailing data in profile/key extension")
	}
	canonicalProfile, err := asn1.Marshal(value)
	if err != nil {
		return nil, true, fmt.Errorf("re-encode profile/key extension: %w", err)
	}
	if !bytes.Equal(canonicalProfile, profileExtension.Value) {
		return nil, true, errors.New("profile/key extension is not canonical DER")
	}
	if value.Version != profile.version {
		return nil, true, fmt.Errorf("unsupported hybrid certificate profile version %d", value.Version)
	}
	if !value.SignatureAlgorithm.Algorithm.Equal(oidMLDSA65) {
		return nil, true, errors.New("unsupported hybrid certificate PQ signature algorithm")
	}
	if len(value.SignatureAlgorithm.Parameters.FullBytes) != 0 {
		return nil, true, errors.New("ML-DSA-65 AlgorithmIdentifier parameters must be absent")
	}
	subjectKeyValue, err := ParsePKIXPublicKey(value.SubjectPublicKeyDER)
	if err != nil {
		return nil, true, fmt.Errorf("invalid subject ML-DSA-65 public key: %w", err)
	}
	subjectPQ, ok := subjectKeyValue.(*PublicKey)
	if !ok {
		return nil, true, errors.New("profile/key extension does not contain an ML-DSA-65 public key")
	}

	var signature asn1.BitString
	rest, err = asn1.Unmarshal(signatureExtension.Value, &signature)
	if err != nil {
		return nil, true, fmt.Errorf("decode PQ signature extension: %w", err)
	}
	if len(rest) != 0 {
		return nil, true, errors.New("trailing data in PQ signature extension")
	}
	canonicalSignature, err := asn1.Marshal(signature)
	if err != nil {
		return nil, true, fmt.Errorf("re-encode PQ signature extension: %w", err)
	}
	if !bytes.Equal(canonicalSignature, signatureExtension.Value) {
		return nil, true, errors.New("PQ signature extension is not canonical DER")
	}
	if signature.BitLength != len(signature.Bytes)*8 {
		return nil, true, errors.New("PQ signature BIT STRING is not byte-aligned")
	}
	if len(signature.Bytes) != hybridSignatureLength {
		return nil, true, fmt.Errorf("invalid ML-DSA-65 signature length %d", len(signature.Bytes))
	}

	return &HybridCertificateInfo{
		Version:            value.Version,
		SubjectPublicKey:   *subjectPQ,
		SignatureAlgorithm: value.SignatureAlgorithm,
		Signature:          slices.Clone(signature.Bytes),
	}, true, nil
}

// ValidateHybridCertificateChain validates a Go-verified chain in leaf-to-root
// order. If required is false, an entirely classical chain is permitted, but a
// partial or invalid Pactway v1 chain is never treated as classical.
func ValidateHybridCertificateChain(
	chain []*x509.Certificate,
	profile HybridCertificateProfile,
	required bool,
) error {
	if profile.name == "" {
		return errors.New("hybrid certificate profile is required")
	}
	if profile.developmentOnly && profile.use == ProfileUseProduction {
		return fmt.Errorf("hybrid certificate profile %q is development-only", profile.name)
	}
	if len(chain) == 0 {
		return errors.New("hybrid certificate validation chain is empty")
	}
	infos := make([]*HybridCertificateInfo, len(chain))
	foundCount := 0
	for index, certificate := range chain {
		if HasLegacyHybridExtensions(certificate.Extensions) {
			return fmt.Errorf("certificate at chain index %d contains legacy alternate-key extensions", index)
		}
		info, found, err := ParseHybridCertificate(certificate, profile)
		if err != nil {
			return fmt.Errorf("certificate at chain index %d has an invalid hybrid profile: %w", index, err)
		}
		if found {
			infos[index] = info
			foundCount++
		}
	}
	if foundCount == 0 {
		if required {
			return errors.New("PQ-required MSP received a classical-only certificate chain")
		}
		return nil
	}
	if foundCount != len(chain) {
		return errors.New("hybrid certificate chain is incomplete; every certificate must use the same profile")
	}

	for index, certificate := range chain {
		isTrustAnchor := index == len(chain)-1
		if isTrustAnchor && !bytes.Equal(certificate.RawSubject, certificate.RawIssuer) {
			return errors.New("hybrid MSP trust anchor must be self-issued and self-signed")
		}
		issuerIndex := index + 1
		if isTrustAnchor {
			issuerIndex = index
		}
		issuer := chain[issuerIndex]
		if err := certificate.CheckSignatureFrom(issuer); err != nil {
			return fmt.Errorf("certificate at chain index %d has an invalid classical issuer signature: %w", index, err)
		}
		if !bytes.Equal(certificate.RawIssuer, issuer.RawSubject) {
			return fmt.Errorf("certificate at chain index %d does not name its configured issuer", index)
		}
		unsignedTBS, err := removeHybridSignatureExtension(certificate.RawTBSCertificate, profile.pqSignatureOID)
		if err != nil {
			return fmt.Errorf("certificate at chain index %d has invalid TBSCertificate DER: %w", index, err)
		}
		valid, err := Verify(infos[issuerIndex].SubjectPublicKey, infos[index].Signature, hybridSignedInput(unsignedTBS))
		if err != nil || !valid {
			if err == nil {
				err = errors.New("signature verification returned false")
			}
			return fmt.Errorf("certificate at chain index %d has an invalid issuer ML-DSA-65 signature: %w", index, err)
		}
	}
	return nil
}

func hybridSignedInput(unsignedTBS []byte) []byte {
	message := append([]byte(hybridCertificateDomain), 0)
	return append(message, unsignedTBS...)
}

func encodeHybridSignature(signature []byte) ([]byte, error) {
	return asn1.Marshal(asn1.BitString{Bytes: signature, BitLength: len(signature) * 8})
}

func rejectDuplicateHybridExtensions(
	profile HybridCertificateProfile,
	template *x509.Certificate,
	signatureExtension pkix.Extension,
) error {
	all := append(slices.Clone(template.Extensions), template.ExtraExtensions...)
	all = append(all, signatureExtension)
	profileCount, signatureCount := 0, 0
	for _, extension := range all {
		if extension.Id.Equal(profile.profileKeyOID) {
			profileCount++
		}
		if extension.Id.Equal(profile.pqSignatureOID) {
			signatureCount++
		}
	}
	if profileCount != 0 || signatureCount != 1 {
		return errors.New("certificate template contains duplicate or conflicting Pactway profile extensions")
	}
	return nil
}

func removeHybridSignatureExtension(tbs []byte, signatureOID asn1.ObjectIdentifier) ([]byte, error) {
	fields, err := hybridDERChildren(tbs, 0x30)
	if err != nil {
		return nil, err
	}
	extensionsIndex := -1
	var extensions []hybridDERValue
	for index, field := range fields {
		if field.tag != 0xa3 {
			continue
		}
		if extensionsIndex != -1 {
			return nil, errors.New("duplicate TBSCertificate extensions field")
		}
		extensionsIndex = index
		extensions, err = hybridDERChildren(field.content, 0x30)
		if err != nil {
			return nil, err
		}
	}
	if extensionsIndex == -1 {
		return nil, errors.New("TBSCertificate has no extensions")
	}

	filtered := make([][]byte, 0, len(extensions))
	signatureCount := 0
	for _, extensionDER := range extensions {
		var extension pkix.Extension
		rest, err := asn1.Unmarshal(extensionDER.full, &extension)
		if err != nil {
			return nil, fmt.Errorf("decode TBSCertificate extension: %w", err)
		}
		if len(rest) != 0 {
			return nil, errors.New("trailing data in TBSCertificate extension")
		}
		canonicalExtension, err := asn1.Marshal(extension)
		if err != nil {
			return nil, fmt.Errorf("re-encode TBSCertificate extension: %w", err)
		}
		if !bytes.Equal(canonicalExtension, extensionDER.full) {
			return nil, errors.New("TBSCertificate extension is not canonical DER")
		}
		if extension.Id.Equal(signatureOID) {
			signatureCount++
			continue
		}
		filtered = append(filtered, extensionDER.full)
	}
	if signatureCount != 1 {
		return nil, fmt.Errorf("expected one PQ signature extension, got %d", signatureCount)
	}

	fields[extensionsIndex].full = hybridDERWrap(0xa3, hybridDERWrap(0x30, bytes.Join(filtered, nil)))
	var content []byte
	for _, field := range fields {
		content = append(content, field.full...)
	}
	return hybridDERWrap(0x30, content), nil
}

func hybridDERChildren(der []byte, expectedTag byte) ([]hybridDERValue, error) {
	value, rest, err := hybridDERRead(der)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 || value.tag != expectedTag {
		return nil, errors.New("unexpected or trailing DER value")
	}
	var children []hybridDERValue
	contentOffset := value.headerLn
	for content := value.content; len(content) > 0; {
		child, remaining, err := hybridDERRead(content)
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

func hybridDERRead(der []byte) (hybridDERValue, []byte, error) {
	if len(der) < 2 {
		return hybridDERValue{}, nil, errors.New("truncated DER value")
	}
	tag := der[0]
	if tag&0x1f == 0x1f {
		return hybridDERValue{}, nil, errors.New("high-tag-number DER is unsupported")
	}
	lengthByte := der[1]
	headerLength, contentLength := 2, 0
	switch {
	case lengthByte < 0x80:
		contentLength = int(lengthByte)
	case lengthByte == 0x80:
		return hybridDERValue{}, nil, errors.New("indefinite length is not DER")
	default:
		lengthOctets := int(lengthByte & 0x7f)
		if lengthOctets > 4 || len(der) < 2+lengthOctets {
			return hybridDERValue{}, nil, errors.New("invalid DER length")
		}
		if der[2] == 0 {
			return hybridDERValue{}, nil, errors.New("non-minimal DER length")
		}
		headerLength += lengthOctets
		for _, octet := range der[2:headerLength] {
			contentLength = contentLength<<8 | int(octet)
		}
		if contentLength < 0x80 {
			return hybridDERValue{}, nil, errors.New("non-minimal DER length")
		}
		if lengthOctets > 1 && contentLength < 1<<(8*(lengthOctets-1)) {
			return hybridDERValue{}, nil, errors.New("non-minimal DER length")
		}
	}
	if contentLength < 0 || len(der) < headerLength+contentLength {
		return hybridDERValue{}, nil, errors.New("truncated DER content")
	}
	end := headerLength + contentLength
	return hybridDERValue{
		tag:      tag,
		full:     der[:end],
		content:  der[headerLength:end],
		headerLn: headerLength,
	}, der[end:], nil
}

func hybridDERWrap(tag byte, content []byte) []byte {
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
