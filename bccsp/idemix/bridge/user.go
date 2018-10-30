/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric/bccsp/idemix"
	cryptolib "github.com/hyperledger/fabric/idemix"
	"github.com/pkg/errors"
)

// User encapsulates the idemix algorithms to generate user secret keys and pseudonym.
type User struct {
	NewRand func() *amcl.RAND
}

// NewKey generates an idemix user secret key
func (u *User) NewKey() (res idemix.Big, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	res = &Big{E: cryptolib.RandModOrder(u.NewRand())}

	return
}

// MakeNym generates a new pseudonym key-pair derived from the passed user secret key (sk) and issuer public key (ipk)
func (u *User) MakeNym(sk idemix.Big, ipk idemix.IssuerPublicKey) (r1 idemix.Ecp, r2 idemix.Big, err error) {
	defer func() {
		if r := recover(); r != nil {
			r1 = nil
			r2 = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	isk, ok := sk.(*Big)
	if !ok {
		return nil, nil, errors.Errorf("invalid user secret key, expected *Big, got [%T]", sk)
	}
	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	ecp, big := cryptolib.MakeNym(isk.E, iipk.PK, u.NewRand())

	r1 = &Ecp{E: ecp}
	r2 = &Big{E: big}

	return
}
