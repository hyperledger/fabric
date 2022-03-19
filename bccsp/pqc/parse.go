package pqc

import "C"
import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"

	"github.com/pkg/errors"
)

// MVP implementation for marshalling and unmarshalling OQS keys.
// It is assumed that, once specific quantum-safe algorithms become standardized,
// there will be forthcoming RFCs that precisely describe their expected asn1 format, etc.
// Eventually, these should be supported by the Go SDK crypto packages directly.

type Algorithm = SigType

// pkixPublicKey reflects a PKIX public key structure. See SubjectPublicKeyInfo
// in RFC 3280.
type pkixPublicKey struct {
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

// asn1.Unmarshal will unmarshal into a data structure like pkixPublicKey, but with RawContent
type pkixPublicKeyUnpack struct {
	Raw       asn1.RawContent
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

func MarshalPKIXPublicKey(pub interface{}) ([]byte, error) {
	l, err := GetLib()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to marshal OQS key")
	}
	pk, ok := pub.(*PublicKey)
	if !ok {
		return nil, errors.New("key is not a known OQS key type")
	}

	publicKeyAlgorithm, err := l.GetAlgorithmIdentifier(pk.Sig.Algorithm)
	if err != nil {
		return nil, err
	}
	pkix := pkixPublicKey{
		Algorithm: publicKeyAlgorithm,
		PublicKey: asn1.BitString{
			Bytes:     pk.Pk,
			BitLength: 8 * len(pk.Pk),
		},
	}
	ret, _ := asn1.Marshal(pkix)
	return ret, nil
}

func ParsePKIXPublicKey(derBytes []byte) (interface{}, error) {
	l, err := GetLib()

	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse PQC key")
	}
	// hex.DecodeString("123")
	var pku pkixPublicKeyUnpack
	//
	if rest, err := asn1.Unmarshal(derBytes, &pku); err != nil {
		return nil, err
	} else if len(rest) != 0 {
		return nil, errors.New("x509: trailing data after ASN.1 of public-key")
	}
	alg := l.GetAlgorithmFromOID(pku.Algorithm.Algorithm)
	if alg == UnknownKeyAlgorithm {
		return nil, errors.New(fmt.Sprintf(
			"unknown PQC public key algorithm with id %s", pku.Algorithm.Algorithm.String()))
	}
	asn1Data := pku.PublicKey.RightAlign()
	s := OQSSigInfo{
		Algorithm: alg,
	}
	publicKey := &PublicKey{Pk: asn1Data, Sig: s}
	return publicKey, nil
}

type pkixPrivateKey struct {
	Algorithm  pkix.AlgorithmIdentifier
	PrivateKey asn1.BitString
	PublicKey  asn1.BitString
}

type pkixPrivateKeyUnpack struct {
	Raw        asn1.RawContent
	Algorithm  pkix.AlgorithmIdentifier
	PrivateKey asn1.BitString
	PublicKey  asn1.BitString
}

func MarshalPKIXPrivateKey(pub interface{}) ([]byte, error) {
	l, err := GetLib()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to marshal OQS key")
	}
	sk, ok := pub.(*SecretKey)
	if !ok {
		return nil, errors.New("key is not a known OQS key type")
	}
	privateKeyAlgorithm, err := l.GetAlgorithmIdentifier(sk.Sig.Algorithm)
	if err != nil {
		return nil, err
	}
	pkix := pkixPrivateKey{
		Algorithm: privateKeyAlgorithm,
		PrivateKey: asn1.BitString{
			Bytes:     sk.Sk,
			BitLength: 8 * len(sk.Sk),
		},
		PublicKey: asn1.BitString{
			Bytes:     sk.Pk,
			BitLength: 8 * len(sk.Pk),
		},
	}
	ret, _ := asn1.Marshal(pkix)
	return ret, nil

}

func ParsePKIXPrivateKey(derBytes []byte) (interface{}, error) {
	l, err := GetLib()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse OQS key")
	}
	var pku pkixPrivateKeyUnpack
	if rest, err := asn1.Unmarshal(derBytes, &pku); err != nil {
		return nil, err
	} else if len(rest) != 0 {
		return nil, errors.New("x509: trailing data after ASN.1 of private-key")
	}
	alg := l.GetAlgorithmFromOID(pku.Algorithm.Algorithm)
	if alg == UnknownKeyAlgorithm {
		return nil, errors.New("unknown OQS public key algorithm")
	}
	asn1PrivData := pku.PrivateKey.RightAlign()
	asn1PubData := pku.PublicKey.RightAlign()
	s := OQSSigInfo{
		Algorithm: alg,
	}
	pk := PublicKey{
		Pk:  asn1PubData,
		Sig: s,
	}
	privKey := &SecretKey{asn1PrivData, pk}
	return privKey, nil
}

// Encode quantum public keys as X509 Extensions, per
// https://tools.ietf.org/id/draft-truskovsky-lamps-pq-hybrid-x509-00.html
var (
	oidSubjectAltPublicKeyInfo = asn1.ObjectIdentifier{2, 5, 29, 71}
	oidAltSignatureAlgorithm   = asn1.ObjectIdentifier{2, 5, 29, 72}
	oidAltSignatureValue       = asn1.ObjectIdentifier{2, 5, 29, 73}
)

type KeyMaterial struct {
	ClassicalBytes asn1.BitString
	QuantumBytes   asn1.BitString
}

func buildSubjectAltPublicKeyInfoExt(qk *PublicKey) (pkix.Extension, error) {
	l, err := GetLib()
	if err != nil {
		return pkix.Extension{}, errors.Wrap(err, "Unable to build subjectaltpublickeyinfo extension")
	}
	oid, err := l.GetAlgorithmIdentifier(qk.Sig.Algorithm)
	if err != nil {
		return pkix.Extension{}, err
	}
	pki := pkixPublicKey{
		Algorithm: oid,
		PublicKey: asn1.BitString{
			qk.Pk,
			8 * len(qk.Pk),
		},
	}
	val, _ := asn1.Marshal(pki)

	return pkix.Extension{
		Id:       oidSubjectAltPublicKeyInfo,
		Critical: false,
		Value:    val,
	}, nil
}

//func buildAltSignatureAlgorithmExt(qSigner crypto.Signer) (pkix.Extension, error) {
//
//	pK, ok := qSigner.Public().(*PublicKey)
//	if !ok {
//		return pkix.Extension{}, errors.New("Unable to obtain pq algorithm from signer")
//	}
//	sa, err := getAlgorithmIdentifier(pK.Sig.Algorithm)
//	if err != nil {
//		return pkix.Extension{}, err
//	}
//	val, _ := asn1.Marshal(sa)
//	return pkix.Extension{
//		Id:       oidAltSignatureAlgorithm,
//		Critical: false,
//		Value:    val,
//	}, nil
//
//}

func buildKmMessage(qkb []byte, ckb []byte) []byte {
	km := KeyMaterial{
		ClassicalBytes: asn1.BitString{
			ckb,
			8 * len(ckb),
		},
		QuantumBytes: asn1.BitString{
			qkb,
			8 * len(qkb),
		},
	}
	kmBytes, _ := asn1.Marshal(km)
	return kmBytes
}

func buildAltSignatureValueExt(qkb []byte, ckb []byte, qSigner crypto.Signer) (pkix.Extension, error) {

	kmBytes := buildKmMessage(qkb, ckb)

	signature, err := qSigner.Sign(rand.Reader, kmBytes, nil)
	if err != nil {
		return pkix.Extension{}, errors.New("Failed to sign classical/quantum key material")
	}
	return pkix.Extension{
		Id:       oidAltSignatureValue,
		Critical: false,
		Value:    signature,
	}, err
}

func BuildAltPublicKeyExtensions(quantumKey interface{}, classicalKey interface{}, qSigner crypto.Signer) ([]pkix.Extension, error) {

	var extensions []pkix.Extension
	var qKBytes []byte
	qK, ok := quantumKey.(*PublicKey)
	if !ok {
		return nil, errors.New("provided quantum key is not a known OQS key type")
	}
	if qK != nil {
		ext, err := buildSubjectAltPublicKeyInfoExt(qK)
		if err != nil {
			return nil, err
		}
		extensions = append(extensions, ext)

		qKBytes = qK.Pk
	}

	cKBytes, err := x509.MarshalPKIXPublicKey(classicalKey)
	if err != nil {
		return nil, err
	}
	ext, err := buildAltSignatureValueExt(qKBytes, cKBytes, qSigner)
	if err != nil {
		return nil, err
	}
	extensions = append(extensions, ext)

	//ext, err = buildAltSignatureAlgorithmExt(qSigner)
	//if err != nil {
	//	return nil, err
	//}
	//extensions = append(extensions, ext)
	return extensions, nil
}

func ParseSubjectAltPublicKeyInfoExtension(extensions []pkix.Extension) (*PublicKey, error) {
	l, err := GetLib()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse subjectaltpublickeyinfo extension")
	}
	var publicKey *PublicKey = nil
	for _, ext := range extensions {
		if ext.Id.Equal(oidSubjectAltPublicKeyInfo) {
			if publicKey != nil {
				return nil, errors.New("Multiple alternate public keys found")
			}
			var pku pkixPublicKeyUnpack
			if rest, err := asn1.Unmarshal(ext.Value, &pku); err != nil {
				return nil, err
			} else if len(rest) != 0 {
				return nil, errors.New("x509: trailing data after ASN.1 of SubjectAltPublicKey")
			}
			alg := l.GetAlgorithmFromOID(pku.Algorithm.Algorithm)
			if alg == UnknownKeyAlgorithm {
				return nil, errors.New("unknown OQS public key algorithm")
			}
			asn1Data := pku.PublicKey.RightAlign()
			s := OQSSigInfo{
				Algorithm: alg,
			}
			publicKey = &PublicKey{Pk: asn1Data, Sig: s}
		}
	}
	// If no alt public key extensions are found, this is not an error.
	// Caller is responsible for checking that returned key is not nil, if required.
	return publicKey, nil
}

func parseAltSignatureValueExtension(extensions []pkix.Extension) ([]byte, error) {
	var sig []byte = nil
	for _, ext := range extensions {
		if ext.Id.Equal(oidAltSignatureValue) {
			if sig != nil {
				return nil, errors.New("Multiple alternate signature values found")
			}
			sig = ext.Value
		}
	}
	// If no alt signature value extensions are found, this is not an error.
	// Caller is responsible for checking that returned signature is not nil, if required.
	return sig, nil
}

func Validate(validationChain []*x509.Certificate) error {
	// Check the validation chain for post-quantum validation.
	// If the rootCA has no post-quantum key material, return immediately: no additional validation needed.
	rootCert := validationChain[len(validationChain)-1]
	rootPk, err := ParseSubjectAltPublicKeyInfoExtension(rootCert.Extensions)
	if err != nil {
		return err
	}
	if rootPk == nil {
		return nil
	}

	// Otherwise, the chain of certs must all carry the AltSignatureValue extension
	// with a valid post-quantum signature from the parent cert matching the cert's key material
	// The leaf cert IS permitted to have classical-only key material, but all other certs must have
	// a SubjectAltPublicKeyInfo extension.
	parentPk := rootPk
	for i := len(validationChain) - 1; i >= 0; i-- {
		cert := validationChain[i]
		pk, err := ParseSubjectAltPublicKeyInfoExtension(cert.Extensions)
		if err != nil {
			return err
		}
		var pkBytes []byte
		if pk == nil && i != 0 {
			// This is not a leaf certificate, but we do not have a post-quantum public key.
			return errors.New("Found no post-quantum key in a non-leaf cert on a quantum-safe validation chain")
		}
		if pk != nil {
			pkBytes = pk.Pk
		}

		ckBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
		if err != nil {
			return err
		}

		km := buildKmMessage(pkBytes, ckBytes)
		sig, err := parseAltSignatureValueExtension(cert.Extensions)
		if err != nil {
			return err
		}
		isValid, err := Verify(*parentPk, sig, km)
		if err != nil {
			return err
		}
		if !isValid {
			return errors.New("Invalid postquantum signature for certificate key material")
		}

		// TODO(ameliaholcomb): Check the signature algorithm extension
	}
	// If the entire chain was validated successfully, we are done
	return nil
}
