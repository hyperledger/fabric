// Matrix operations for elements in GF(2^128).
package msp

type Row []FieldElem

// NewRow returns a row of length s with all zero entries.
func NewRow(s int) Row {
	out := Row(make([]FieldElem, s))

	for i := 0; i < s; i++ {
		out[i] = NewFieldElem()
	}

	return out
}

// AddM adds two vectors.
func (e Row) AddM(f Row) {
	le, lf := e.Size(), f.Size()
	if le != lf {
		panic("Can't add rows that are different sizes!")
	}

	for i, f_i := range f {
		e[i].AddM(f_i)
	}

	return
}

// MulM multiplies the row by a scalar.
func (e Row) MulM(f FieldElem) {
	for i, _ := range e {
		e[i] = e[i].Mul(f)
	}
}

func (e Row) Mul(f FieldElem) Row {
	out := NewRow(e.Size())

	for i := 0; i < e.Size(); i++ {
		out[i] = e[i].Mul(f)
	}

	return out
}

// DotProduct computes the dot product of two vectors.
func (e Row) DotProduct(f Row) FieldElem {
	if e.Size() != f.Size() {
		panic("Can't get dot product of rows of different length!")
	}

	out := NewFieldElem()

	for i := 0; i < e.Size(); i++ {
		out.AddM(e[i].Mul(f[i]))
	}

	return out
}

func (e Row) Size() int {
	return len(e)
}

type Matrix []Row

// Mul right-multiplies a matrix by a row.
func (e Matrix) Mul(f Row) Row {
	out, in := e.Size()
	if in != f.Size() {
		panic("Can't multiply by row that is wrong size!")
	}

	res := NewRow(out)

	for i := 0; i < out; i++ {
		res[i] = e[i].DotProduct(f)
	}

	return res
}

// Recovery returns the row vector that takes this matrix to the target vector [1 0 0 ... 0].
func (e Matrix) Recovery() (Row, bool) {
	a, b := e.Size()

	// aug is the target vector.
	aug := NewRow(a)
	aug[0] = One.Dup()

	// Duplicate e away so we don't mutate it; transpose it at the same time.
	f := make([]Row, b)
	for i, _ := range f {
		f[i] = NewRow(a)
	}

	for i := 0; i < a; i++ {
		for j := 0; j < b; j++ {
			f[j][i] = e[i][j].Dup()
		}
	}

	for row, _ := range f {
		if row >= b { // The matrix is tall and thin--we've finished before exhausting all the rows.
			break
		}

		// Find a row with a non-zero entry in the (row)th position
		candId := -1
		for j, f_j := range f[row:] {
			if !f_j[row].IsZero() {
				candId = j + row
				break
			}
		}

		if candId == -1 { // If we can't find one, fail and return our partial work.
			return aug, false
		}

		// Move it to the top
		f[row], f[candId] = f[candId], f[row]
		aug[row], aug[candId] = aug[candId], aug[row]

		// Make the pivot 1.
		fInv := f[row][row].Invert()

		f[row].MulM(fInv)
		aug[row] = aug[row].Mul(fInv)

		// Cancel out the (row)th position for every row above and below it.
		for i, _ := range f {
			if i != row && !f[i][row].IsZero() {
				c := f[i][row].Dup()

				f[i].AddM(f[row].Mul(c))
				aug[i].AddM(aug[row].Mul(c))
			}
		}
	}

	return aug, true
}

func (e Matrix) Size() (int, int) {
	return len(e), e[0].Size()
}
