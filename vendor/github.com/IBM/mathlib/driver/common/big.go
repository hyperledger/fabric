/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "math/big"

var onebytes = []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
var onebig = new(big.Int).SetBytes(onebytes)

func BigToBytes(bi *big.Int) []byte {
	b := bi.Bytes()

	if bi.Sign() >= 0 {
		return append(make([]byte, 32-len(b)), b...)
	}

	twoscomp := new(big.Int).Set(onebig)
	pos := new(big.Int).Neg(bi)
	twoscomp = twoscomp.Sub(twoscomp, pos)
	twoscomp = twoscomp.Add(twoscomp, big.NewInt(1))
	b = twoscomp.Bytes()
	return append(onebytes[:32-len(b)], b...)
}
