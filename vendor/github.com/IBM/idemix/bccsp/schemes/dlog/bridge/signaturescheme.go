/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"crypto/ecdsa"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/handlers"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// SignatureScheme encapsulates the idemix algorithms to sign and verify using an idemix credential.
type SignatureScheme struct {
	Translator idemix.Translator
	Idemix     *idemix.Idemix
}

// Sign produces an idemix-signature with the respect to the passed serialised credential (cred),
// user secret key (sk), pseudonym public key (Nym) and secret key (RNym), issuer public key (ipk),
// and attributes to be disclosed.
func (s *SignatureScheme) Sign(cred []byte, sk *math.Zr, Nym *math.G1, RNym *math.Zr, ipk handlers.IssuerPublicKey, attributes []bccsp.IdemixAttribute,
	msg []byte, rhIndex, eidIndex int, criRaw []byte, sigType bccsp.SignatureType) (res []byte, meta *bccsp.IdemixSignerMetadata, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	credential := &idemix.Credential{}
	err = proto.Unmarshal(cred, credential)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed unmarshalling credential")
	}

	cri := &idemix.CredentialRevocationInformation{}
	err = proto.Unmarshal(criRaw, cri)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed unmarshalling credential revocation information")
	}

	disclosure := make([]byte, len(attributes))
	for i := 0; i < len(attributes); i++ {
		if attributes[i].Type == bccsp.IdemixHiddenAttribute {
			disclosure[i] = 0
		} else {
			disclosure[i] = 1
		}
	}

	sig, meta, err := s.Idemix.NewSignature(
		credential,
		sk,
		Nym,
		RNym,
		iipk.PK,
		disclosure,
		msg,
		rhIndex, eidIndex,
		cri,
		newRandOrPanic(s.Idemix.Curve),
		s.Translator,
		sigType)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed creating new signature")
	}

	sigBytes, err := proto.Marshal(sig)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "marshalling error")
	}

	return sigBytes, meta, nil
}

func (s *SignatureScheme) AuditNymEid(
	ipk handlers.IssuerPublicKey,
	eidIndex int,
	signature []byte,
	enrollmentID string,
	RNymEid *math.Zr,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig := &idemix.Signature{}
	err = proto.Unmarshal(signature, sig)
	if err != nil {
		return err
	}

	eidAttr := s.Idemix.Curve.HashToZr([]byte(enrollmentID))

	return sig.AuditNymEid(
		iipk.PK,
		eidAttr,
		eidIndex,
		RNymEid,
		s.Idemix.Curve,
		s.Translator,
	)
}

// Verify checks that an idemix signature is valid with the respect to the passed issuer public key, digest, attributes,
// revocation index (rhIndex), revocation public key, and epoch.
func (s *SignatureScheme) Verify(
	ipk handlers.IssuerPublicKey,
	signature, digest []byte,
	attributes []bccsp.IdemixAttribute,
	rhIndex, eidIndex int,
	revocationPublicKey *ecdsa.PublicKey,
	epoch int,
	verType bccsp.VerificationType,
	meta *bccsp.IdemixSignerMetadata,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig := &idemix.Signature{}
	err = proto.Unmarshal(signature, sig)
	if err != nil {
		return err
	}

	disclosure := make([]byte, len(attributes))
	attrValues := make([]*math.Zr, len(attributes))
	for i := 0; i < len(attributes); i++ {
		switch attributes[i].Type {
		case bccsp.IdemixHiddenAttribute:
			disclosure[i] = 0
			attrValues[i] = nil
		case bccsp.IdemixBytesAttribute:
			disclosure[i] = 1
			attrValues[i] = s.Idemix.Curve.HashToZr(attributes[i].Value.([]byte))
		case bccsp.IdemixIntAttribute:
			var value int64
			if v, ok := attributes[i].Value.(int); ok {
				value = int64(v)
			} else if v, ok := attributes[i].Value.(int64); ok {
				value = v
			} else {
				return errors.Errorf("invalid int type for IdemixIntAttribute attribute")
			}

			disclosure[i] = 1
			attrValues[i] = s.Idemix.Curve.NewZrFromInt(value)
		default:
			err = errors.Errorf("attribute type not allowed or supported [%v] at position [%d]", attributes[i].Type, i)
		}
	}
	if err != nil {
		return
	}

	return sig.Ver(
		disclosure,
		iipk.PK,
		digest,
		attrValues,
		rhIndex,
		eidIndex,
		revocationPublicKey,
		epoch,
		s.Idemix.Curve,
		s.Translator,
		verType,
		meta,
	)
}
