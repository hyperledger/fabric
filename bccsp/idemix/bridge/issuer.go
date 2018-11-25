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

// IssuerPublicKey encapsulate an idemix issuer public key.
type IssuerPublicKey struct {
	PK *cryptolib.IssuerPublicKey
}

func (o *IssuerPublicKey) Bytes() ([]byte, error) {
	return proto.Marshal(o.PK)
}

// IssuerPublicKey encapsulate an idemix issuer secret key.
type IssuerSecretKey struct {
	SK *cryptolib.IssuerKey
}

func (o *IssuerSecretKey) Bytes() ([]byte, error) {
	return proto.Marshal(o.SK)
}

func (o *IssuerSecretKey) Public() handlers.IssuerPublicKey {
	return &IssuerPublicKey{o.SK.Ipk}
}

// Issuer encapsulates the idemix algorithms to generate issuer key-pairs
type Issuer struct {
	NewRand func() *amcl.RAND
}

// NewKey generates a new issuer key-pair
func (i *Issuer) NewKey(attributeNames []string) (res handlers.IssuerSecretKey, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	sk, err := cryptolib.NewIssuerKey(attributeNames, i.NewRand())
	if err != nil {
		return
	}

	res = &IssuerSecretKey{SK: sk}

	return
}
