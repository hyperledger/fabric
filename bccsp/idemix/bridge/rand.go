/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	amcl "github.com/hyperledger/fabric-amcl/core"
	cryptolib "github.com/hyperledger/fabric/idemix"
)

// NewRandOrPanic return a new amcl PRG or panic
func NewRandOrPanic() *amcl.RAND {
	rng, err := cryptolib.GetRand()
	if err != nil {
		panic(err)
	}
	return rng
}
