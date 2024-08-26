/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/hyperledger/fabric-lib-go/bccsp/utils"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"
)

// A SigningIdentity represents an MSP signing identity.
type SigningIdentity struct {
	CertPath string
	KeyPath  string
	MSPID    string
}

// Serialize returns the probobuf encoding of an msp.SerializedIdenity.
func (s *SigningIdentity) Serialize() ([]byte, error) {
	cert, err := os.ReadFile(s.CertPath)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&msp.SerializedIdentity{
		Mspid:   s.MSPID,
		IdBytes: cert,
	})
}

// Sign computes a SHA256 message digest if key is ECDSA,
// signs it with the associated private key, and returns the
// signature. Low-S normlization is applied for ECDSA signatures.
func (s *SigningIdentity) Sign(msg []byte) ([]byte, error) {
	digest := sha256.Sum256(msg)
	pemKey, err := os.ReadFile(s.KeyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemKey)
	if block.Type != "EC PRIVATE KEY" && block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("file %s does not contain a private key", s.KeyPath)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		r, _s, err := ecdsa.Sign(rand.Reader, k, digest[:])
		if err != nil {
			return nil, err
		}
		sig, err := utils.MarshalECDSASignature(r, _s)
		if err != nil {
			return nil, err
		}
		return utils.SignatureToLowS(&k.PublicKey, sig)
	case ed25519.PrivateKey:
		return ed25519.Sign(k, msg), nil
	default:
		return nil, fmt.Errorf("unexpected key type: %T", key)
	}
}
