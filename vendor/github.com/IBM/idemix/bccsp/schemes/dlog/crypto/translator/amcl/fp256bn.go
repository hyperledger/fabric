/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	fmt "fmt"

	math "github.com/IBM/mathlib"
)

type Fp256bn struct {
	C *math.Curve
}

func (a *Fp256bn) G1ToProto(g1 *math.G1) *ECP {
	if g1 == nil {
		panic("nil argument")
	}

	bytes := g1.Bytes()[1:]
	l := len(bytes) / 2
	return &ECP{
		X: bytes[:l],
		Y: bytes[l:],
	}
}

func (a *Fp256bn) G1FromRawBytes(raw []byte) (*math.G1, error) {
	l := len(raw) / 2

	return a.G1FromProto(&ECP{
		X: raw[:l],
		Y: raw[l:],
	})
}

func (a *Fp256bn) G1FromProto(e *ECP) (*math.G1, error) {
	if e == nil {
		return nil, fmt.Errorf("nil argument")
	}

	if len(e.X) != a.C.FieldBytes || len(e.Y) != a.C.FieldBytes {
		return nil, fmt.Errorf("invalid marshalled length")
	}

	bytes := make([]byte, len(e.X)*2+1)
	l := len(e.X)
	bytes[0] = 0x04
	copy(bytes[1:], e.X)
	copy(bytes[l+1:], e.Y)
	return a.C.NewG1FromBytes(bytes)
}

func (a *Fp256bn) G2ToProto(g2 *math.G2) *ECP2 {
	if g2 == nil {
		panic("nil argument")
	}

	bytes := g2.Bytes()
	l := len(bytes) / 4
	return &ECP2{
		Xa: bytes[0:l],
		Xb: bytes[l : 2*l],
		Ya: bytes[2*l : 3*l],
		Yb: bytes[3*l:],
	}

}

func (a *Fp256bn) G2FromProto(e *ECP2) (*math.G2, error) {
	if e == nil {
		return nil, fmt.Errorf("nil argument")
	}

	if len(e.Xa) != a.C.FieldBytes || len(e.Xb) != a.C.FieldBytes || len(e.Ya) != a.C.FieldBytes || len(e.Yb) != a.C.FieldBytes {
		return nil, fmt.Errorf("invalid marshalled length")
	}

	bytes := make([]byte, len(e.Xa)*4)
	l := len(e.Xa)
	copy(bytes[0:l], e.Xa)
	copy(bytes[l:2*l], e.Xb)
	copy(bytes[2*l:3*l], e.Ya)
	copy(bytes[3*l:], e.Yb)
	return a.C.NewG2FromBytes(bytes)
}

type Fp256bnMiracl struct {
	C *math.Curve
}

func (a *Fp256bnMiracl) G1ToProto(g1 *math.G1) *ECP {
	if g1 == nil {
		panic("nil argument")
	}

	bytes := g1.Bytes()[1:]
	l := len(bytes) / 2
	return &ECP{
		X: bytes[:l],
		Y: bytes[l:],
	}
}

func (a *Fp256bnMiracl) G1FromRawBytes(raw []byte) (*math.G1, error) {
	l := len(raw) / 2

	return a.G1FromProto(&ECP{
		X: raw[:l],
		Y: raw[l:],
	})
}

func (a *Fp256bnMiracl) G1FromProto(e *ECP) (*math.G1, error) {
	if e == nil {
		return nil, fmt.Errorf("nil argument")
	}

	if len(e.X) != a.C.FieldBytes || len(e.Y) != a.C.FieldBytes {
		return nil, fmt.Errorf("invalid marshalled length")
	}

	bytes := make([]byte, len(e.X)*2+1)
	l := len(e.X)
	bytes[0] = 0x04
	copy(bytes[1:], e.X)
	copy(bytes[l+1:], e.Y)
	return a.C.NewG1FromBytes(bytes)
}

func (a *Fp256bnMiracl) G2ToProto(g2 *math.G2) *ECP2 {
	if g2 == nil {
		panic("nil argument")
	}

	bytes := g2.Bytes()[1:]
	l := len(bytes) / 4
	return &ECP2{
		Xa: bytes[0:l],
		Xb: bytes[l : 2*l],
		Ya: bytes[2*l : 3*l],
		Yb: bytes[3*l:],
	}

}

func (a *Fp256bnMiracl) G2FromProto(e *ECP2) (*math.G2, error) {
	if e == nil {
		return nil, fmt.Errorf("nil argument")
	}

	if len(e.Xa) != a.C.FieldBytes || len(e.Xb) != a.C.FieldBytes || len(e.Ya) != a.C.FieldBytes || len(e.Yb) != a.C.FieldBytes {
		return nil, fmt.Errorf("invalid marshalled length")
	}

	bytes := make([]byte, 1+len(e.Xa)*4)
	bytes[0] = 0x04
	l := len(e.Xa)
	copy(bytes[1:], e.Xa)
	copy(bytes[1+l:], e.Xb)
	copy(bytes[1+2*l:], e.Ya)
	copy(bytes[1+3*l:], e.Yb)
	return a.C.NewG2FromBytes(bytes)
}
