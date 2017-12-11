/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemixca

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/idemix"
	"github.com/hyperledger/fabric/msp"
	m "github.com/hyperledger/fabric/protos/msp"
	"github.com/milagro-crypto/amcl/version3/go/amcl/FP256BN"
	"github.com/pkg/errors"
)

// GenerateIssuerKey invokes Idemix library to generate an issuer (CA) signing key pair
// currently two attributes are supported by the issuer:
// AttributeNameOU is the organization unit name
// AttributeNameRole is the role (member or admin) name
// Generated keys are serialized to bytes
func GenerateIssuerKey() ([]byte, []byte, error) {
	rng, err := idemix.GetRand()
	if err != nil {
		return nil, nil, err
	}
	AttributeNames := []string{msp.AttributeNameOU, msp.AttributeNameRole}
	key, err := idemix.NewIssuerKey(AttributeNames, rng)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate CA key")
	}
	ipkSerialized, err := proto.Marshal(key.IPk)

	return key.ISk, ipkSerialized, err
}

// GenerateMSPConfig creates a new MSP config
// If the new MSP config contains a signer then
// it generates a fresh user secret and issues a credential
// with two attributes (described above)
// using the CA's key pair from the file
// If the new MSP config does not contain a signer
// (meaning it is used only for verification)
// then only a public key of the CA (issuer) is added to the MSP config (besides the name)
func GenerateSignerConfig(isAdmin bool, ouString string, key *idemix.IssuerKey) ([]byte, error) {
	attrs := make([]*FP256BN.BIG, 2)

	if ouString == "" {
		return nil, errors.Errorf("the OU attribute value is empty")
	}

	role := m.MSPRole_MEMBER

	if isAdmin {
		role = m.MSPRole_ADMIN
	}

	attrs[0] = idemix.HashModOrder([]byte(ouString))
	attrs[1] = FP256BN.NewBIGint(int(role))

	rng, err := idemix.GetRand()
	if err != nil {
		return nil, errors.WithMessage(err, "Error getting PRNG")
	}
	sk := idemix.RandModOrder(rng)
	randCred := idemix.RandModOrder(rng)
	ni := idemix.RandModOrder(rng)
	msg := idemix.NewCredRequest(sk, randCred, ni, key.IPk, rng)
	cred, err := idemix.NewCredential(key, msg, attrs, rng)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate a credential")
	}
	cred.Complete(randCred)

	credBytes, err := proto.Marshal(cred)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal credential")
	}

	signer := &m.IdemixMSPSignerConfig{
		credBytes,
		idemix.BigToBytes(sk),
		ouString,
		isAdmin,
	}
	return proto.Marshal(signer)
}
