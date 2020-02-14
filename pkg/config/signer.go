/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
)

// Signer holds the information necessary to sign a configuration update.
// It implements the crypto.Signer interface for ECDSA keys.
type Signer struct {
	cert       *x509.Certificate
	mspID      string
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

type ecdsaSignature struct {
	R, S *big.Int
}

// NewSigner creates a new signer for configuration updates. The publicCert
// and privateKey should be pem encoded.
func NewSigner(publicCert []byte, privateCert []byte, mspID string) (*Signer, error) {
	if mspID == "" {
		return nil, errors.New("failed to create new signer, mspID can not be empty")
	}

	cert, err := getCertFromPem(publicCert)
	if err != nil {
		return nil, fmt.Errorf("failed to get cert from pem: %v", err)
	}

	publicKey, err := ecdsaPublicKeyImport(cert)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECDSA public key: %v", err)
	}

	pkBlock, _ := pem.Decode(privateCert)
	if pkBlock == nil {
		return nil, errors.New("failed to decode private key from pem")
	}

	privateKey, err := ecdsaPrivateKeyImport(pkBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECDSA private key: %v", err)
	}

	signer := &Signer{
		cert:       cert,
		mspID:      mspID,
		privateKey: privateKey,
		publicKey:  publicKey,
	}

	return signer, nil
}

// Serialize returns a byte array representation of this identity.
func (s *Signer) Serialize() ([]byte, error) {
	pb := &pem.Block{Bytes: s.cert.Raw, Type: "CERTIFICATE"}
	pemBytes := pem.EncodeToMemory(pb)
	if pemBytes == nil {
		return nil, errors.New("failed to encode pem block")
	}

	// serialize identities by prepending the MSPID and appending
	// the ASN.1 DER content of the cert
	sID := &msp.SerializedIdentity{Mspid: s.mspID, IdBytes: pemBytes}

	idBytes, err := proto.Marshal(sID)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal serialized identity: %v", err)
	}

	return idBytes, nil
}

// Public returns the public key of the signer.
func (s *Signer) Public() crypto.PublicKey {
	return s.publicKey
}

// Cert returns the certificate of the signer.
func (s *Signer) Cert() *x509.Certificate {
	return s.cert
}

// MSPId returns the MSP ID of the signer.
func (s *Signer) MSPId() string {
	return s.mspID
}

// Sign performs ECDSA sign with signer's private key on given digest. It
// ensures signatures are created with Low S values since Fabric normalizes
// all signatures to Low S.
// See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
// for more detail.
func (s *Signer) Sign(reader io.Reader, digest []byte) (signature []byte, err error) {
	if reader == nil {
		return nil, errors.New("failed to sign, reader can not be nil")
	}

	rr, ss, err := ecdsa.Sign(reader, s.privateKey, digest)
	if err != nil {
		return nil, err
	}

	// ensure Low S signatures
	sig := toLowS(
		s.privateKey.PublicKey,
		ecdsaSignature{
			R: rr,
			S: ss,
		},
	)

	return asn1.Marshal(sig)
}

// CreateSignatureHeader returns a new SignatureHeader. It contains a valid
// nonce and a creator, which is a serialized representation of the signer.
func (s *Signer) CreateSignatureHeader() (*common.SignatureHeader, error) {
	creator, err := s.Serialize()
	if err != nil {
		return nil, err
	}
	nonce, err := createNonce()
	if err != nil {
		return nil, err
	}

	return &common.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}, nil
}

// createNonce generates a nonce using the crypto/rand package.
func createNonce() ([]byte, error) {
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %s", err)
	}
	return nonce, nil
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get random bytes: %s", err)
	}
	return key, nil
}

// When using ECDSA, both (r,s) and (r, -s mod n) are valid signatures.
// In order to protect against signature malleability attacks, Fabric
// normalizes all signatures to a canonical form where s is at most half
// the order of the curve. In order to make signatures compliant with what
// Fabric expects, toLowS creates signatures in this canonical form.
func toLowS(key ecdsa.PublicKey, sig ecdsaSignature) ecdsaSignature {
	// calculate half order of the curve
	halfOrder := new(big.Int).Div(key.Curve.Params().N, big.NewInt(2))
	// check if s is greater than half order of curve
	if sig.S.Cmp(halfOrder) == 1 {
		// Set s to N - s so that s will be less than or equal to half order
		sig.S.Sub(key.Params().N, sig.S)
	}
	return sig
}

// getCertFromPem decodes a pem encoded cert into an x509 certificate.
func getCertFromPem(pemBytes []byte) (*x509.Certificate, error) {
	pemCert, _ := pem.Decode(pemBytes)
	if pemCert == nil {
		return nil, fmt.Errorf("failed to decode pem bytes: %v", pemBytes)
	}

	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 cert: %v", err)
	}

	return cert, nil
}

// ecdsaPublicKeyImport imports the public key from an x509 certificate.
func ecdsaPublicKeyImport(x509Cert *x509.Certificate) (*ecdsa.PublicKey, error) {
	pk := x509Cert.PublicKey

	lowLevelKey, ok := pk.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("certificate does not contain valid ECDSA public key")
	}

	return lowLevelKey, nil
}

// ecdsaPrivateKeyImport imports the private key from the private key bytes.
func ecdsaPrivateKeyImport(pkBytes []byte) (*ecdsa.PrivateKey, error) {
	lowLevelKey, err := x509.ParsePKCS8PrivateKey(pkBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid key type. The DER must contain an ecdsa.PrivateKey: %v", err)
	}

	ecdsaSK, ok := lowLevelKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("failed to cast private key bytes to ECDSA private key")
	}

	return ecdsaSK, nil
}
