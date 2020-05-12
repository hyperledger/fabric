/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"
	cryptolib "github.com/hyperledger/fabric/idemix"
	"github.com/pkg/errors"
)

// User encapsulates the idemix algorithms to generate user secret keys and pseudonym.
type User struct {
	NewRand func() *amcl.RAND
}

// NewKey generates an idemix user secret key
func (u *User) NewKey() (res handlers.Big, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	res = &Big{E: cryptolib.RandModOrder(u.NewRand())}

	return
}

func (*User) NewKeyFromBytes(raw []byte) (res handlers.Big, err error) {
	if len(raw) != int(FP256BN.MODBYTES) {
		return nil, errors.Errorf("invalid length, expected [%d], got [%d]", FP256BN.MODBYTES, len(raw))
	}

	res = &Big{E: FP256BN.FromBytes(raw)}

	return
}

// MakeNym generates a new pseudonym key-pair derived from the passed user secret key (sk) and issuer public key (ipk)
func (u *User) MakeNym(sk handlers.Big, ipk handlers.IssuerPublicKey) (r1 handlers.Ecp, r2 handlers.Big, err error) {
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

func (*User) NewPublicNymFromBytes(raw []byte) (r handlers.Ecp, err error) {
	defer func() {
		if r := recover(); r != nil {
			r = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	// raw is the concatenation of two big integers
	lHalve := len(raw) / 2

	r = &Ecp{E: FP256BN.NewECPbigs(FP256BN.FromBytes(raw[:lHalve]), FP256BN.FromBytes(raw[lHalve:]))}

	return
}
