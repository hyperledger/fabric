package msp

import (
	"testing"
)

func TestRecovery(t *testing.T) {
	// Generate the matrix.
	height, width := 10, 10
	M := Matrix(make([]Row, height))

	for i := 0; i < height; i++ {
		M[i] = NewRow(width)

		for j := 0; j < width; j++ {
			M[i][j][0] = byte(i + 1)
			M[i][j] = M[i][j].Exp(j)
		}
	}

	// Find the recovery vector.
	r, ok := M.Recovery()
	if !ok {
		t.Fatalf("Failed to find the recovery vector!")
	}

	// Find the output vector.
	out := NewRow(width)

	for i := 0; i < height; i++ {
		out.AddM(M[i].Mul(r[i]))
	}

	// Check that it is the target vector.
	if !out[0].IsOne() {
		t.Fatalf("Output is not the target vector!")
	}

	for i := 1; i < width; i++ {
		if !out[i].IsZero() {
			t.Fatalf("Output is not the target vector!")
		}
	}
}
