/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemixca

import (
	"crypto/ecdsa"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/hyperledger/fabric/idemix"
	"github.com/hyperledger/fabric/msp"
	m "github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

// GenerateIssuerKey invokes Idemix library to generate an issuer (CA) signing key pair.
// Currently four attributes are supported by the issuer:
// AttributeNameOU is the organization unit name
// AttributeNameRole is the role (member or admin) name
// AttributeNameEnrollmentId is the enrollment id
// AttributeNameRevocationHandle contains the revocation handle, which can be used to revoke this user
// Generated keys are serialized to bytes.
func GenerateIssuerKey() ([]byte, []byte, error) {
	rng, err := idemix.GetRand()
	if err != nil {
		return nil, nil, err
	}
	AttributeNames := []string{msp.AttributeNameOU, msp.AttributeNameRole, msp.AttributeNameEnrollmentId, msp.AttributeNameRevocationHandle}
	key, err := idemix.NewIssuerKey(AttributeNames, rng)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate CA key")
	}
	ipkSerialized, err := proto.Marshal(key.Ipk)

	return key.Isk, ipkSerialized, err
}

// GenerateSignerConfig creates a new signer config.
// It generates a fresh user secret and issues a credential
// with four attributes (described above) using the CA's key pair.
func GenerateSignerConfig(roleMask int, ouString string, enrollmentId string, revocationHandle int, key *idemix.IssuerKey, revKey *ecdsa.PrivateKey) ([]byte, error) {
	attrs := make([]*FP256BN.BIG, 4)

	if ouString == "" {
		return nil, errors.Errorf("the OU attribute value is empty")
	}

	if enrollmentId == "" {
		return nil, errors.Errorf("the enrollment id value is empty")
	}

	attrs[msp.AttributeIndexOU] = idemix.HashModOrder([]byte(ouString))
	attrs[msp.AttributeIndexRole] = FP256BN.NewBIGint(roleMask)
	attrs[msp.AttributeIndexEnrollmentId] = idemix.HashModOrder([]byte(enrollmentId))
	attrs[msp.AttributeIndexRevocationHandle] = FP256BN.NewBIGint(revocationHandle)

	rng, err := idemix.GetRand()
	if err != nil {
		return nil, errors.WithMessage(err, "Error getting PRNG")
	}
	sk := idemix.RandModOrder(rng)
	ni := idemix.RandModOrder(rng)
	msg := idemix.NewCredRequest(sk, ni, key.Ipk, rng)
	cred, err := idemix.NewCredential(key, msg, attrs, rng)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate a credential")
	}

	credBytes, err := proto.Marshal(cred)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal credential")
	}

	// NOTE currently, idemixca creates CRI's with "ALG_NO_REVOCATION"
	cri, err := idemix.CreateCRI(revKey, []*FP256BN.BIG{FP256BN.NewBIGint(revocationHandle)}, 0, idemix.ALG_NO_REVOCATION, rng)
	if err != nil {
		return nil, err
	}
	criBytes, err := proto.Marshal(cri)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal CRI")
	}

	signer := &m.IdemixMSPSignerConfig{
		Cred: credBytes,
		Sk:   idemix.BigToBytes(sk),
		OrganizationalUnitIdentifier: ouString,
		Role:                            int32(roleMask),
		EnrollmentId:                    enrollmentId,
		CredentialRevocationInformation: criBytes,
	}

	return proto.Marshal(signer)
}
