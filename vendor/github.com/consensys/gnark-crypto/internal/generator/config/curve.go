package config

import (
	"math/big"

	"github.com/consensys/gnark-crypto/field"
)

// Curve describes parameters of the curve useful for the template
type Curve struct {
	Name         string
	CurvePackage string
	Package      string // current package being generated
	EnumID       string
	FpModulus    string
	FrModulus    string

	Fp           *field.Field
	Fr           *field.Field
	FpUnusedBits int

	FpInfo, FrInfo Field
	G1             Point
	G2             Point
}

type Field struct {
	Bits    int
	Bytes   int
	Modulus func() *big.Int
}

func (c Curve) Equal(other Curve) bool {
	return c.Name == other.Name
}

type Point struct {
	CoordType        string
	PointName        string
	GLV              bool  // scalar mulitplication using GLV
	CofactorCleaning bool  // flag telling if the Cofactor cleaning is available
	CRange           []int // multiexp bucket method: generate inner methods (with const arrays) for each c
	Projective       bool  // generate projective coordinates
}

var Curves []Curve

func defaultCRange() []int {
	// default range for C values in the multiExp
	return []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 20, 21, 22}
}

func addCurve(c *Curve) {
	// init FpInfo and FrInfo
	c.FpInfo = newFieldInfo(c.FpModulus)
	c.FrInfo = newFieldInfo(c.FrModulus)
	Curves = append(Curves, *c)
}

func newFieldInfo(modulus string) Field {
	var F Field
	var bModulus big.Int
	if _, ok := bModulus.SetString(modulus, 10); !ok {
		panic("invalid modulus " + modulus)
	}

	F.Bits = bModulus.BitLen()
	F.Bytes = len(bModulus.Bits()) * 8
	F.Modulus = func() *big.Int { return new(big.Int).Set(&bModulus) }
	return F
}
