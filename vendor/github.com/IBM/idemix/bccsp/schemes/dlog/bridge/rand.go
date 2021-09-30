/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"io"

	math "github.com/IBM/mathlib"
)

func newRandOrPanic(curve *math.Curve) io.Reader {
	rng, err := curve.Rand()
	if err != nil {
		panic(err)
	}
	return rng
}
