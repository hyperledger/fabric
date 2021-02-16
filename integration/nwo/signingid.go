/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/utils"
)

// A SigningIdentity represents an MSP signing identity.
type SigningIdentity struct {
	CertPath string
	KeyPath  string
	MSPID    string
}

// Serialize returns the probobuf encoding of an msp.SerializedIdenity.
func (s *SigningIdentity) Serialize() ([]byte, error) {
	cert, err := ioutil.ReadFile(s.CertPath)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&msp.SerializedIdentity{
		Mspid:   s.MSPID,
		IdBytes: cert,
	})
}

// Sign computes a SHA256 message digest, signs it with the associated private
// key, and returns the signature after low-S normlization.
func (s *SigningIdentity) Sign(msg []byte) ([]byte, error) {
	digest := sha256.Sum256(msg)
	pemKey, err := ioutil.ReadFile(s.KeyPath)
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
	eckey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type: %T", key)
	}
	r, _s, err := ecdsa.Sign(rand.Reader, eckey, digest[:])
	if err != nil {
		return nil, err
	}
	sig, err := utils.MarshalECDSASignature(r, _s)
	if err != nil {
		return nil, err
	}
	return utils.SignatureToLowS(&eckey.PublicKey, sig)
}
