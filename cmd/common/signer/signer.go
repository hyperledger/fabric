/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signer

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"os"
	"strings"

	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// Config holds the configuration for
// creation of a Signer
type Config struct {
	MSPID        string
	IdentityPath string
	KeyPath      string
}

// Signer signs messages.
// TODO: Ideally we'd use an MSP to be agnostic, but since it's impossible to
// initialize an MSP without a CA cert that signs the signing identity,
// this will do for now.
type Signer struct {
	key     *ecdsa.PrivateKey
	Creator []byte
}

func (si *Signer) Serialize() ([]byte, error) {
	return si.Creator, nil
}

// NewSigner creates a new Signer out of the given configuration
func NewSigner(conf Config) (*Signer, error) {
	sId, err := serializeIdentity(conf.IdentityPath, conf.MSPID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key, err := loadPrivateKey(conf.KeyPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Signer{
		Creator: sId,
		key:     key,
	}, nil
}

func serializeIdentity(clientCert string, mspID string) ([]byte, error) {
	b, err := os.ReadFile(clientCert)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := validateEnrollmentCertificate(b); err != nil {
		return nil, err
	}
	sId := &msp.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: b,
	}
	return protoutil.MarshalOrPanic(sId), nil
}

func validateEnrollmentCertificate(b []byte) error {
	bl, _ := pem.Decode(b)
	if bl == nil {
		return errors.Errorf("enrollment certificate isn't a valid PEM block")
	}

	if bl.Type != "CERTIFICATE" {
		return errors.Errorf("enrollment certificate should be a certificate, got a %s instead", strings.ToLower(bl.Type))
	}

	if _, err := x509.ParseCertificate(bl.Bytes); err != nil {
		return errors.Errorf("enrollment certificate is not a valid x509 certificate: %v", err)
	}
	return nil
}

func (si *Signer) Sign(msg []byte) ([]byte, error) {
	digest := util.ComputeSHA256(msg)
	return signECDSA(si.key, digest)
}

func loadPrivateKey(file string) (*ecdsa.PrivateKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bl, _ := pem.Decode(b)
	if bl == nil {
		return nil, errors.Errorf("failed to decode PEM block from %s", file)
	}
	key, err := parsePrivateKey(bl.Bytes)
	if err != nil {
		return nil, err
	}
	return key.(*ecdsa.PrivateKey), nil
}

// Based on crypto/tls/tls.go but modified for Fabric:
func parsePrivateKey(der []byte) (crypto.PrivateKey, error) {
	// OpenSSL 1.0.0 generates PKCS#8 keys.
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		// Fabric only supports ECDSA at the moment.
		case *ecdsa.PrivateKey:
			return key, nil
		default:
			return nil, errors.Errorf("found unknown private key type (%T) in PKCS#8 wrapping", key)
		}
	}

	// OpenSSL ecparam generates SEC1 EC private keys for ECDSA.
	key, err := x509.ParseECPrivateKey(der)
	if err != nil {
		return nil, errors.Errorf("failed to parse private key: %v", err)
	}

	return key, nil
}

func signECDSA(k *ecdsa.PrivateKey, digest []byte) (signature []byte, err error) {
	r, s, err := ecdsa.Sign(rand.Reader, k, digest)
	if err != nil {
		return nil, err
	}

	s, err = utils.ToLowS(&k.PublicKey, s)
	if err != nil {
		return nil, err
	}

	return marshalECDSASignature(r, s)
}

func marshalECDSASignature(r, s *big.Int) ([]byte, error) {
	return asn1.Marshal(ECDSASignature{r, s})
}

type ECDSASignature struct {
	R, S *big.Int
}
