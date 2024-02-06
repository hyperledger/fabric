/*
Copyright IBM Corp. All Rights Reserved.
Copyright (c) 2012-2020 MIRACL UK Ltd.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	_ "unsafe"

	"github.com/hyperledger/fabric-amcl/core"
	"github.com/hyperledger/fabric-amcl/core/FP256BN"
)

const HASH_TYPE int = 32

//go:linkname hash_to_field github.com/hyperledger/fabric-amcl/core/FP256BN.hash_to_field
func hash_to_field(hash int, hlen int, DST []byte, M []byte, ctr int) []*FP256BN.FP

func bls_hash_to_point_miracl(M, DST []byte) *FP256BN.ECP {
	u := hash_to_field(core.MC_SHA2, HASH_TYPE, DST, M, 2)

	P := FP256BN.ECP_map2point(u[0])
	P1 := FP256BN.ECP_map2point(u[1])
	P.Add(P1)
	P.Cfp()
	P.Affine()
	return P
}
