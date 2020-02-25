/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
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

// SigningIdentity is an MSP Identity that can be used to sign configuration updates and transaction.
type SigningIdentity struct {
	pemCert    []byte // pem encoded publicCert
	mspID      string
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

type ecdsaSignature struct {
	R, S *big.Int
}

// NewSigningIdentity creates a new signing identity for configuration updates.
// The publicCert and privateKey should be pem encoded.
func NewSigningIdentity(publicCert []byte, privateKey []byte, mspID string) (*SigningIdentity, error) {
	if mspID == "" {
		return nil, errors.New("failed to create new signingIdentity, mspID can not be empty")
	}

	cert, err := getCertFromPem(publicCert)
	if err != nil {
		return nil, fmt.Errorf("failed to get cert from pem: %v", err)
	}

	pubKey, err := ecdsaPublicKeyImport(cert)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECDSA public key: %v", err)
	}

	pkBlock, _ := pem.Decode(privateKey)
	if pkBlock == nil || pkBlock.Type != "PRIVATE KEY" {
		return nil, errors.New("failed to decode private key from pem")
	}

	privKey, err := ecdsaPrivateKeyImport(pkBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECDSA private key: %v", err)
	}

	signer := &SigningIdentity{
		pemCert:    publicCert,
		mspID:      mspID,
		privateKey: privKey,
		publicKey:  pubKey,
	}

	return signer, nil
}

// Serialize returns a byte array representation of this identity.
func (s *SigningIdentity) Serialize() ([]byte, error) {
	// serialize identities by prepending the MSPID and appending
	// the ASN.1 DER content of the cert
	sID := &msp.SerializedIdentity{Mspid: s.mspID, IdBytes: s.pemCert}
	idBytes, err := proto.Marshal(sID)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal serialized indentity: %v", err)
	}

	return idBytes, nil
}

// Cert returns the certificate of the signer.
func (s *SigningIdentity) Cert() (*x509.Certificate, error) {
	cert, err := getCertFromPem(s.pemCert)
	if err != nil {
		return nil, fmt.Errorf("failed to get cert from pem: %v", err)
	}

	return cert, nil
}

// MSPId returns the MSP ID of the signer.
func (s *SigningIdentity) MSPId() string {
	return s.mspID
}

// Sign performs ECDSA sign with SigningIdentity's private key on given digest.
// It ensures signatures are created with Low S values since Fabric normalizes
// all signatures to Low S.
// See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
// for more detail.
func (s *SigningIdentity) Sign(reader io.Reader, digest []byte) (signature []byte, err error) {
	if reader == nil {
		return nil, errors.New("reader can not be nil")
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

// CreateSignatureHeader returns a new SignatureHeader. It contains a valid nonce
// and a creator, which is a serialized representation of the signing identity.
func (s *SigningIdentity) CreateSignatureHeader() (*common.SignatureHeader, error) {
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
	nonce := make([]byte, 24)

	_, err := rand.Read(nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to get random bytes: %v", err)
	}

	return nonce, nil
}

// toLows normalizes all signatures to a canonical form where s is at most
// half the order of the curve. By doing so, it compliant with what Fabric
// expected as well as protect against signature malleability attacks.
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
	if pemCert == nil || pemCert.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("decoding pem bytes: %v", pemBytes)
	}

	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing x509 cert: %v", err)
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
		return nil, errors.New("casting private key bytes to ECDSA private key")
	}

	return ecdsaSK, nil
}
