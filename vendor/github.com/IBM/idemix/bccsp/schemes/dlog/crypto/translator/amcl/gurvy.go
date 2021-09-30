/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	math "github.com/IBM/mathlib"
)

type Gurvy struct {
	C *math.Curve
}

func (a *Gurvy) G1ToProto(g1 *math.G1) *ECP {
	bytes := g1.Bytes()

	l := len(bytes) / 2
	return &ECP{
		X: bytes[:l],
		Y: bytes[l:],
	}
}

func (a *Gurvy) G1FromRawBytes(raw []byte) (*math.G1, error) {
	l := len(raw) / 2

	return a.G1FromProto(&ECP{
		X: raw[:l],
		Y: raw[l:],
	})
}

func (a *Gurvy) G1FromProto(e *ECP) (*math.G1, error) {
	bytes := make([]byte, len(e.X)*2)
	l := len(e.X)
	copy(bytes, e.X)
	copy(bytes[l:], e.Y)
	return a.C.NewG1FromBytes(bytes)
}

func (a *Gurvy) G2ToProto(g2 *math.G2) *ECP2 {
	bytes := g2.Bytes()
	l := len(bytes) / 4
	return &ECP2{
		Xa: bytes[0:l],
		Xb: bytes[l : 2*l],
		Ya: bytes[2*l : 3*l],
		Yb: bytes[3*l:],
	}

}

func (a *Gurvy) G2FromProto(e *ECP2) (*math.G2, error) {
	bytes := make([]byte, len(e.Xa)*4)
	l := len(e.Xa)
	copy(bytes[0:l], e.Xa)
	copy(bytes[l:2*l], e.Xb)
	copy(bytes[2*l:3*l], e.Ya)
	copy(bytes[3*l:], e.Yb)
	return a.C.NewG2FromBytes(bytes)
}
