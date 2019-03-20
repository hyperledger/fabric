/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"math/big"

	"github.com/pkg/errors"
)

// Quantity models a token quantity and its basic operations.
type Quantity interface {

	// Add returns this + b without modify this.
	// If an overflow occurs, it returns an error.
	Add(b Quantity) (Quantity, error)

	// Add returns this - b without modify this.
	// If an overflow occurs, it returns an error.
	Sub(b Quantity) (Quantity, error)

	// Cmd compare this with b
	Cmp(b Quantity) (int, error)

	// Hex returns the hexadecimal representation of this quantity
	Hex() string

	// Decimal returns the decimal representation of this quantity
	Decimal() string
}

type BigQuantity struct {
	*big.Int
	Precision uint64
}

// ToQuantity converts a string q to a BigQuantity of a given precision.
// Argument q is supposed to be formatted following big.Int#scan specification.
// The precision is expressed in bits.
func ToQuantity(q string, precision uint64) (Quantity, error) {
	v, success := big.NewInt(0).SetString(q, 0)
	if !success {
		return nil, errors.New("invalid input")
	}
	if v.Cmp(big.NewInt(0)) <= 0 {
		return nil, errors.New("quantity must be larger than 0")
	}
	if precision == 0 {
		return nil, errors.New("precision be larger than 0")
	}
	if v.BitLen() > int(precision) {
		return nil, errors.Errorf("%s has precision %d > %d", q, v.BitLen(), precision)
	}

	return &BigQuantity{Int: v, Precision: precision}, nil
}

// NewZeroQuantity returns to zero quantity at the passed precision/
// The precision is expressed in bits.
func NewZeroQuantity(precision uint64) Quantity {
	b := BigQuantity{Int: big.NewInt(0), Precision: precision}
	return &b
}

func (q *BigQuantity) Add(b Quantity) (Quantity, error) {
	bq, ok := b.(*BigQuantity)
	if !ok {
		return nil, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	// Check overflow
	sum := big.NewInt(0)
	sum = sum.Add(q.Int, bq.Int)

	if sum.BitLen() > int(q.Precision) {
		return nil, errors.Errorf("%d + %d = overflow", q, b)
	}

	sumq := BigQuantity{Int: sum, Precision: q.Precision}
	return &sumq, nil
}

func (q *BigQuantity) Sub(b Quantity) (Quantity, error) {
	bq, ok := b.(*BigQuantity)
	if !ok {
		return nil, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	// Check overflow
	if q.Int.Cmp(bq.Int) < 0 {
		return nil, errors.Errorf("%d < %d", q, b)
	}
	diff := big.NewInt(0)
	diff.Sub(q.Int, b.(*BigQuantity).Int)

	diffq := BigQuantity{Int: diff, Precision: q.Precision}
	return &diffq, nil
}

func (q *BigQuantity) Cmp(b Quantity) (int, error) {
	bq, ok := b.(*BigQuantity)
	if !ok {
		return 0, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	return q.Int.Cmp(bq.Int), nil
}

func (q *BigQuantity) Hex() string {
	return "0x" + q.Int.Text(16)
}

func (q *BigQuantity) Decimal() string {
	return q.Int.Text(10)
}
