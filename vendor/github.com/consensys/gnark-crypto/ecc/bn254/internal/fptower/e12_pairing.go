package fptower

func (z *E12) nSquare(n int) {
	for range n {
		z.CyclotomicSquare(z)
	}
}

func (z *E12) nSquareCompressed(n int) {
	for range n {
		z.CyclotomicSquareCompressed(z)
	}
}

// Expt set z to xᵗ (mod q¹²) and return z (t is the generator of the curve)
// t = 4965661367192848881 = 0x44e992b44a6909f1
func (z *E12) Expt(x *E12) *E12 {
	// Expt computation is derived from the addition-subtraction chain
	// (subtraction is a multiplication by the conjugate in the cyclotomic
	// subgroup):
	//
	//	_10      = 2*1
	//	_11      = 1 + _10
	//	_101     = _10 + _11
	//	_111     = _10 + _101
	//	_1000    = 1 + _111
	//	_1000000 = _1000 << 3
	//	_1000101 = _101 + _1000000
	//	i25      = ((_1000101 << 5 - _11) << 4 + _11) << 5
	//	i36      = ((_101 + i25) << 4 - _101) << 4 - _11
	//	i52      = ((i36 << 4 + 1) << 5 + _101) << 5
	//	i66      = ((_111 + i52) << 4 - _111) << 7 + _101
	//	return     (i66 << 5 - 1) << 4 + 1
	//
	// Operations: 60 squares, 17 multiplies, 5 conjugates.

	var result, t0, x3, x5, x7 E12

	// Dictionary: x^3, x^5, x^7, then x^69 in result.
	t0.CyclotomicSquare(x)
	x3.Mul(x, &t0)
	x5.Mul(&t0, &x3)
	x7.Mul(&t0, &x5)
	result.Mul(x, &x7) // x^8
	result.nSquare(3)  // x^64
	result.Mul(&x5, &result)

	// i25 = ((x^69 << 5 - x^3) << 4 + x^3) << 5
	result.nSquare(5)
	t0.Conjugate(&x3)
	result.Mul(&result, &t0)
	result.nSquare(4)
	result.Mul(&result, &x3)
	result.nSquare(5)

	// i36 = ((x^5 * i25) << 4 - x^5) << 4 - x^3
	result.Mul(&result, &x5)
	result.nSquare(4)
	t0.Conjugate(&x5)
	result.Mul(&result, &t0)
	result.nSquare(4)
	t0.Conjugate(&x3)
	result.Mul(&result, &t0)

	// i52 = ((i36 << 4 * x) << 5 * x^5) << 5
	result.nSquare(4)
	result.Mul(&result, x)
	result.nSquare(5)
	result.Mul(&result, &x5)
	result.nSquare(5)

	// i66 = ((x^7 * i52) << 4 - x^7) << 7 * x^5
	result.Mul(&result, &x7)
	result.nSquare(4)
	t0.Conjugate(&x7)
	result.Mul(&result, &t0)
	result.nSquare(7)
	result.Mul(&result, &x5)

	// return (i66 << 5 - x) << 4 * x
	result.nSquare(5)
	t0.Conjugate(x)
	result.Mul(&result, &t0)
	result.nSquare(4)
	z.Mul(&result, x)

	return z
}

// MulBy034 multiplication by sparse element (c0,0,0,c3,c4,0)
func (z *E12) MulBy034(c0, c3, c4 *E2) *E12 {

	var a, b, d E6

	a.MulByE2(&z.C0, c0)

	b.Set(&z.C1)
	b.MulBy01(c3, c4)

	var d0 E2
	d0.Add(c0, c3)
	d.Add(&z.C0, &z.C1)
	d.MulBy01(&d0, c4)

	z.C1.Add(&a, &b).Neg(&z.C1).Add(&z.C1, &d)
	z.C0.MulByNonResidue(&b).Add(&z.C0, &a)

	return z
}

// MulBy34 multiplication by sparse element (1,0,0,c3,c4,0)
func (z *E12) MulBy34(c3, c4 *E2) *E12 {

	var a, b, d E6

	a.Set(&z.C0)

	b.Set(&z.C1)
	b.MulBy01(c3, c4)

	var d0 E2
	d0.SetOne().Add(&d0, c3)
	d.Add(&z.C0, &z.C1)
	d.MulBy01(&d0, c4)

	z.C1.Add(&a, &b).Neg(&z.C1).Add(&z.C1, &d)
	z.C0.MulByNonResidue(&b).Add(&z.C0, &a)

	return z
}

// Mul034By034 multiplication of sparse element (c0,0,0,c3,c4,0) by sparse element (d0,0,0,d3,d4,0)
func Mul034By034(d0, d3, d4, c0, c3, c4 *E2) [5]E2 {
	var z00, tmp, x0, x3, x4, x04, x03, x34 E2
	x0.Mul(c0, d0)
	x3.Mul(c3, d3)
	x4.Mul(c4, d4)
	tmp.Add(c0, c4)
	x04.Add(d0, d4).
		Mul(&x04, &tmp).
		Sub(&x04, &x0).
		Sub(&x04, &x4)
	tmp.Add(c0, c3)
	x03.Add(d0, d3).
		Mul(&x03, &tmp).
		Sub(&x03, &x0).
		Sub(&x03, &x3)
	tmp.Add(c3, c4)
	x34.Add(d3, d4).
		Mul(&x34, &tmp).
		Sub(&x34, &x3).
		Sub(&x34, &x4)

	z00.MulByNonResidue(&x4).
		Add(&z00, &x0)

	return [5]E2{z00, x3, x34, x03, x04}
}

// Mul34By34 multiplication of sparse element (1,0,0,c3,c4,0) by sparse element (1,0,0,d3,d4,0)
func Mul34By34(d3, d4, c3, c4 *E2) [5]E2 {
	var z00, tmp, x0, x3, x4, x04, x03, x34 E2
	x3.Mul(c3, d3)
	x4.Mul(c4, d4)
	x04.Add(c4, d4)
	x03.Add(c3, d3)
	tmp.Add(c3, c4)
	x34.Add(d3, d4).
		Mul(&x34, &tmp).
		Sub(&x34, &x3).
		Sub(&x34, &x4)

	x0.SetOne()
	z00.MulByNonResidue(&x4).
		Add(&z00, &x0)

	return [5]E2{z00, x3, x34, x03, x04}
}

// MulBy01234 multiplies z by an E12 sparse element of the form (x0, x1, x2, x3, x4, 0)
func (z *E12) MulBy01234(x *[5]E2) *E12 {
	var c1, a, b, c, z0, z1 E6
	c0 := &E6{B0: x[0], B1: x[1], B2: x[2]}
	c1.B0 = x[3]
	c1.B1 = x[4]
	a.Add(&z.C0, &z.C1)
	b.Add(c0, &c1)
	a.Mul(&a, &b)
	b.Mul(&z.C0, c0)
	c.Set(&z.C1).MulBy01(&x[3], &x[4])
	z1.Sub(&a, &b)
	z1.Sub(&z1, &c)
	z0.MulByNonResidue(&c)
	z0.Add(&z0, &b)

	z.C0 = z0
	z.C1 = z1

	return z
}
