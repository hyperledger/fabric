/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/hyperledger/fabric/idemix"
)

// Big encapsulate an amcl big integer
type Big struct {
	E *FP256BN.BIG
}

func (b *Big) Bytes() ([]byte, error) {
	return idemix.BigToBytes(b.E), nil
}

// Ecp encapsulate an amcl elliptic curve point
type Ecp struct {
	E *FP256BN.ECP
}

func (o *Ecp) Bytes() ([]byte, error) {
	// To store a non-compressed elliptic curve point, we need to allocate
	// enough space for the x and y coordinate, therefore two elements in the
	// base field
	res := make([]byte, 2*idemix.FieldBytes+1)
	o.E.ToBytes(res, false)

	return res, nil
}
