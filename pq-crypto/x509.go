/*
SPDX-License-Identifier: Apache-2.0
*/

package oqs

import (
	"bytes"
	"crypto"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"

	"github.com/pkg/errors"
)

type Algorithm = SigType

func MarshalPKIXPublicKey(pub any) ([]byte, error) {
	pk, ok := pub.(*PublicKey)
	if !ok {
		return nil, errors.New("key is not a known OQS key type")
	}
	if err := requireProfile(pk.Sig.Algorithm); err != nil {
		return nil, err
	}
	key, err := mldsa.NewPublicKey(mldsa.MLDSA65(), pk.Pk)
	if err != nil {
		return nil, errors.Wrap(err, "invalid ML-DSA-65 public key")
	}
	return x509.MarshalPKIXPublicKey(key)
}

func ParsePKIXPublicKey(derBytes []byte) (any, error) {
	var info struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	rest, err := asn1.Unmarshal(derBytes, &info)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("x509: trailing data after ASN.1 of public key")
	}
	if !info.Algorithm.Algorithm.Equal(oidMLDSA65) {
		return nil, errors.New("unknown OQS public key algorithm; only ML-DSA-65 is supported")
	}
	if len(info.Algorithm.Parameters.FullBytes) != 0 {
		return nil, errors.New("ML-DSA-65 AlgorithmIdentifier parameters must be absent")
	}
	if info.PublicKey.BitLength != 8*len(info.PublicKey.Bytes) {
		return nil, errors.New("ML-DSA-65 public key BIT STRING must be byte-aligned")
	}
	mlDSAKey, err := mldsa.NewPublicKey(mldsa.MLDSA65(), info.PublicKey.RightAlign())
	if err != nil {
		return nil, errors.Wrap(err, "invalid ML-DSA-65 public key")
	}
	return &PublicKey{
		Pk: mlDSAKey.Bytes(),
		Sig: OQSSigInfo{
			Algorithm: MLDSA65ProfileV1,
		},
	}, nil
}

func MarshalPKIXPrivateKey(pub any) ([]byte, error) {
	sk, ok := pub.(*SecretKey)
	if !ok {
		return nil, errors.New("key is not a known OQS key type")
	}
	if err := requireProfile(sk.Sig.Algorithm); err != nil {
		return nil, err
	}
	privateKey, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), sk.Sk)
	if err != nil {
		return nil, errors.Wrap(err, "invalid ML-DSA-65 private key")
	}
	if !bytes.Equal(privateKey.PublicKey().Bytes(), sk.Pk) {
		return nil, errors.New("ML-DSA-65 private and public keys do not match")
	}
	return x509.MarshalPKCS8PrivateKey(privateKey)
}

func ParsePKIXPrivateKey(derBytes []byte) (any, error) {
	key, err := x509.ParsePKCS8PrivateKey(derBytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := key.(*mldsa.PrivateKey)
	if !ok {
		return nil, errors.New("unknown ML-DSA private key algorithm")
	}
	if parameters := privateKey.PublicKey().Parameters(); parameters != mldsa.MLDSA65() {
		return nil, errors.Errorf("unsupported ML-DSA parameter set %s; only ML-DSA-65 is supported", parameters.String())
	}
	return &SecretKey{
		privateKey.Bytes(),
		PublicKey{
			Pk: privateKey.PublicKey().Bytes(),
			Sig: OQSSigInfo{
				Algorithm: MLDSA65ProfileV1,
			},
		},
	}, nil
}

// Encode quantum public keys as X509 Extensions, per
// https://tools.ietf.org/id/draft-truskovsky-lamps-pq-hybrid-x509-00.html
var (
	oidMLDSA65                 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 18}
	oidSubjectAltPublicKeyInfo = asn1.ObjectIdentifier{2, 5, 29, 71}
	oidAltSignatureAlgorithm   = asn1.ObjectIdentifier{2, 5, 29, 72}
	oidAltSignatureValue       = asn1.ObjectIdentifier{2, 5, 29, 73}
)

type KeyMaterial struct {
	ClassicalBytes asn1.BitString
	QuantumBytes   asn1.BitString
}

func buildSubjectAltPublicKeyInfoExt(qk *PublicKey) (pkix.Extension, error) {
	value, err := MarshalPKIXPublicKey(qk)
	if err != nil {
		return pkix.Extension{}, errors.Wrap(err, "unable to build subjectAltPublicKeyInfo extension")
	}

	return pkix.Extension{
		Id:       oidSubjectAltPublicKeyInfo,
		Critical: false,
		Value:    value,
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
			Bytes:     ckb,
			BitLength: 8 * len(ckb),
		},
		QuantumBytes: asn1.BitString{
			Bytes:     qkb,
			BitLength: 8 * len(qkb),
		},
	}
	kmBytes, _ := asn1.Marshal(km)
	return kmBytes
}

func buildAltSignatureValueExt(qkb []byte, ckb []byte, qSigner crypto.Signer) (pkix.Extension, error) {
	kmBytes := buildKmMessage(qkb, ckb)

	signature, err := qSigner.Sign(rand.Reader, kmBytes, nil)
	if err != nil {
		return pkix.Extension{}, errors.Wrap(err, "failed to sign classical/quantum key material")
	}
	return pkix.Extension{
		Id:       oidAltSignatureValue,
		Critical: false,
		Value:    signature,
	}, err
}

// BuildAltPublicKeyExtensions constructs a slice of X.509 extensions that encode both quantum-safe and classical public keys,
// as well as a signature value generated by the provided quantum-safe signer.
//
// Parameters:
//   - quantumKey: a value expected to be a pointer to a quantum-safe public key (*PublicKey).
//   - classicalKey: a classical public key, suitable for x509.MarshalPKIXPublicKey.
//   - qSigner: a crypto.Signer capable of producing quantum-safe signatures.
//
// Returns:
//   - []pkix.Extension: a slice of X.509 extensions containing the quantum public key, and an extension with a signature value
//     over both the quantum and classical public keys.
//   - error: non-nil if any key is of an unexpected type or if extension construction fails.
//
// The function first encodes the quantum public key as a custom extension, then marshals the classical public key,
// and finally creates an extension containing a signature value over both keys using the quantum-safe signer.
func BuildAltPublicKeyExtensions(quantumKey any, classicalKey any, qSigner crypto.Signer) ([]pkix.Extension, error) {
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
	var publicKey *PublicKey
	for _, ext := range extensions {
		if ext.Id.Equal(oidSubjectAltPublicKeyInfo) {
			if publicKey != nil {
				return nil, errors.New("Multiple alternate public keys found")
			}
			key, err := ParsePKIXPublicKey(ext.Value)
			if err != nil {
				return nil, errors.Wrap(err, "invalid alternate ML-DSA public key")
			}
			publicKey = key.(*PublicKey)
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

func HasLegacyHybridExtensions(extensions []pkix.Extension) bool {
	for _, extension := range extensions {
		if extension.Id.Equal(oidSubjectAltPublicKeyInfo) ||
			extension.Id.Equal(oidAltSignatureAlgorithm) ||
			extension.Id.Equal(oidAltSignatureValue) {
			return true
		}
	}
	return false
}

func Validate(validationChain []*x509.Certificate) error {
	if len(validationChain) == 0 {
		return errors.New("post-quantum validation chain is empty")
	}

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
	for i := len(validationChain) - 1; i >= 0; i-- {
		cert := validationChain[i]
		issuerIndex := i + 1
		if issuerIndex == len(validationChain) {
			issuerIndex = i
		}
		issuerPk, err := ParseSubjectAltPublicKeyInfoExtension(validationChain[issuerIndex].Extensions)
		if err != nil {
			return err
		}
		if issuerPk == nil {
			return errors.Errorf("Found no post-quantum key in issuer cert at index %d on a quantum-safe validation chain", issuerIndex)
		}

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
		isValid, err := Verify(*issuerPk, sig, km)
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
