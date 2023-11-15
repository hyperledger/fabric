/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package aries

import (
	"fmt"

	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/ale-linux/aries-framework-go/component/kmscrypto/crypto/primitive/bbs12381g2pub"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type Cred struct {
	Bls   *bbs12381g2pub.BBSG2Pub
	Curve *math.Curve
}

// Sign issues a new credential, which is the last step of the interactive issuance protocol
// All attribute values are added by the issuer at this step and then signed together with a commitment to
// the user's secret key from a credential request
func (c *Cred) Sign(key types.IssuerSecretKey, credentialRequest []byte, attributes []types.IdemixAttribute) ([]byte, error) {
	isk, ok := key.(*IssuerSecretKey)
	if !ok {
		return nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", key)
	}

	blindedMsg, err := ParseBlindedMessages(credentialRequest, c.Curve)
	if err != nil {
		return nil, fmt.Errorf("ParseBlindedMessages failed [%w]", err)
	}

	msgsZr := attributesToSignatureMessage(nil, attributes, c.Curve)

	sig, err := BlindSign(msgsZr, len(attributes)+1, blindedMsg.C, isk.SK.FR.Bytes())
	if err != nil {
		return nil, fmt.Errorf("ParseBlindedMessages failed [%w]", err)
	}

	attrs := make([][]byte, len(attributes))
	for i, msg := range msgsZr {
		attrs[i] = msg.FR.Bytes()
	}

	cred := &Credential{
		Cred:  sig,
		Attrs: attrs,
	}

	credBytes, err := proto.Marshal(cred)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal failed [%w]", err)
	}

	return credBytes, nil
}

// Verify cryptographically verifies the credential by verifying the signature
// on the attribute values and user's secret key
func (c *Cred) Verify(sk *math.Zr, key types.IssuerPublicKey, credBytes []byte, attributes []types.IdemixAttribute) error {
	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	credential := &Credential{}
	err := proto.Unmarshal(credBytes, credential)
	if err != nil {
		return fmt.Errorf("proto.Unmarshal failed [%w]", err)
	}

	sigma, err := bbs12381g2pub.ParseSignature(credential.Cred)
	if err != nil {
		return fmt.Errorf("ParseSignature failed [%w]", err)
	}

	sm := make([]*bbs12381g2pub.SignatureMessage, len(credential.Attrs)+1)
	sm[0] = &bbs12381g2pub.SignatureMessage{
		FR:  sk,
		Idx: 0,
	}
	for i, v := range credential.Attrs {
		sm[i+1] = &bbs12381g2pub.SignatureMessage{
			FR:  c.Curve.NewZrFromBytes(v),
			Idx: i + 1,
		}

		switch attributes[i].Type {
		case types.IdemixHiddenAttribute:
			continue
		case types.IdemixBytesAttribute:
			fr := bbs12381g2pub.FrFromOKM(attributes[i].Value.([]byte))
			if !fr.Equals(sm[i+1].FR) {
				return errors.Errorf("credential does not contain the correct attribute value at position [%d]", i)
			}
		case types.IdemixIntAttribute:
			fr := c.Curve.NewZrFromInt(int64(attributes[i].Value.(int)))
			if !fr.Equals(sm[i+1].FR) {
				return errors.Errorf("credential does not contain the correct attribute value at position [%d]", i)
			}
		}
	}

	return sigma.Verify(sm, ipk.PKwG)
}
