/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"
	cryptolib "github.com/hyperledger/fabric/idemix"
	"github.com/pkg/errors"
)

// CredRequest encapsulates the idemix algorithms to produce (sign) a credential request
// and verify it. Recall that a credential request is produced by a user,
// and it is verified by the issuer at credential creation time.
type CredRequest struct {
	NewRand func() *amcl.RAND
}

// Sign produces an idemix credential request. It takes in input a user secret key and
// an issuer public key.
func (cr *CredRequest) Sign(sk handlers.Big, ipk handlers.IssuerPublicKey) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	isk, ok := sk.(*Big)
	if !ok {
		return nil, errors.Errorf("invalid user secret key, expected *Big, got [%T]", sk)
	}
	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	rng := cr.NewRand()

	credRequest := cryptolib.NewCredRequest(
		isk.E,
		cryptolib.RandModOrder(rng),
		iipk.PK,
		rng)

	return proto.Marshal(credRequest)
}

// Verify checks that the passed credential request is valid with the respect to the passed
// issuer public key.
func (*CredRequest) Verify(credentialRequest []byte, ipk handlers.IssuerPublicKey) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	credRequest := &cryptolib.CredRequest{}
	err = proto.Unmarshal(credentialRequest, credRequest)
	if err != nil {
		return err
	}

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	return credRequest.Check(iipk.PK)
}
