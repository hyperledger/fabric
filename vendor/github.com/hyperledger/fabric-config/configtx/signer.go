/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// SigningIdentity is an MSP Identity that can be used to sign configuration
// updates.
type SigningIdentity struct {
	Certificate *x509.Certificate
	PrivateKey  crypto.PrivateKey
	MSPID       string
}

type ecdsaSignature struct {
	R, S *big.Int
}

// Public returns the public key associated with this signing
// identity's certificate.
func (s *SigningIdentity) Public() crypto.PublicKey {
	return s.Certificate.PublicKey
}

// Sign performs ECDSA sign with this signing identity's private key on the
// given message hashed using SHA-256. It ensures signatures are created with
// Low S values since Fabric normalizes all signatures to Low S.
// See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
// for more detail.
func (s *SigningIdentity) Sign(reader io.Reader, msg []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	switch pk := s.PrivateKey.(type) {
	case *ecdsa.PrivateKey:
		hasher := sha256.New()
		hasher.Write(msg)
		digest := hasher.Sum(nil)

		rr, ss, err := ecdsa.Sign(reader, pk, digest)
		if err != nil {
			return nil, err
		}

		// ensure Low S signatures
		sig := toLowS(
			pk.PublicKey,
			ecdsaSignature{
				R: rr,
				S: ss,
			},
		)

		return asn1.Marshal(sig)
	default:
		return nil, fmt.Errorf("signing with private key of type %T not supported", pk)
	}
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

// CreateConfigSignature creates a config signature for the the given configuration
// update using the specified signing identity.
func (s *SigningIdentity) CreateConfigSignature(marshaledUpdate []byte) (*cb.ConfigSignature, error) {
	signatureHeader, err := s.signatureHeader()
	if err != nil {
		return nil, fmt.Errorf("creating signature header: %v", err)
	}

	header, err := proto.Marshal(signatureHeader)
	if err != nil {
		return nil, fmt.Errorf("marshaling signature header: %v", err)
	}

	configSignature := &cb.ConfigSignature{
		SignatureHeader: header,
	}

	configSignature.Signature, err = s.Sign(
		rand.Reader,
		concatenateBytes(configSignature.SignatureHeader, marshaledUpdate),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("signing config update: %v", err)
	}

	return configSignature, nil
}

// SignEnvelope signs an envelope using the SigningIdentity.
func (s *SigningIdentity) SignEnvelope(e *cb.Envelope) error {
	signatureHeader, err := s.signatureHeader()
	if err != nil {
		return fmt.Errorf("creating signature header: %v", err)
	}

	sHeader, err := proto.Marshal(signatureHeader)
	if err != nil {
		return fmt.Errorf("marshaling signature header: %v", err)
	}

	payload := &cb.Payload{}
	err = proto.Unmarshal(e.Payload, payload)
	if err != nil {
		return fmt.Errorf("unmarshaling envelope payload: %v", err)
	}
	payload.Header.SignatureHeader = sHeader

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %v", err)
	}

	sig, err := s.Sign(rand.Reader, payloadBytes, nil)
	if err != nil {
		return fmt.Errorf("signing envelope payload: %v", err)
	}

	e.Payload = payloadBytes
	e.Signature = sig

	return nil
}

func (s *SigningIdentity) signatureHeader() (*cb.SignatureHeader, error) {
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.Certificate.Raw,
	})

	idBytes, err := proto.Marshal(&mb.SerializedIdentity{
		Mspid:   s.MSPID,
		IdBytes: pemBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling serialized identity: %v", err)
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}

	return &cb.SignatureHeader{
		Creator: idBytes,
		Nonce:   nonce,
	}, nil
}

// newNonce generates a 24-byte nonce using the crypto/rand package.
func newNonce() ([]byte, error) {
	nonce := make([]byte, 24)

	_, err := rand.Read(nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to get random bytes: %v", err)
	}

	return nonce, nil
}
