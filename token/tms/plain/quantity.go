/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
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

	// ToUInt64 returns the uint64 representation of this quantity
	ToUInt64() uint64
}

// UInt64Quantity implements Quantity based on a uint64
type UInt64Quantity uint64

// ToQuantity returns a Quantity from the passed uint65
func ToQuantity(q uint64) (Quantity, error) {
	if q == 0 {
		return nil, errors.New("must be larger than 0")
	}

	return UInt64Quantity(q), nil
}

// NewZeroQuantity returns a zero Quantity
func NewZeroQuantity() Quantity {
	return UInt64Quantity(0)
}

func (q UInt64Quantity) Add(b Quantity) (Quantity, error) {
	bq, ok := b.(UInt64Quantity)
	if !ok {
		return nil, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	// Check overflow
	s := q + bq
	if uint64(s) < uint64(q) || uint64(s) < uint64(bq) {
		return nil, errors.Errorf("%d + %d = overflow", q, b)
	}

	return s, nil
}

func (q UInt64Quantity) Sub(b Quantity) (Quantity, error) {
	bq, ok := b.(UInt64Quantity)
	if !ok {
		return nil, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	// Check overflow
	if q < bq {
		return nil, errors.Errorf("%d < %d", q, b)
	}

	return UInt64Quantity(q - b.(UInt64Quantity)), nil
}

func (q UInt64Quantity) Cmp(b Quantity) (int, error) {
	bq, ok := b.(UInt64Quantity)
	if !ok {
		return 0, errors.Errorf("expected uint64 quantity, got '%t", b)
	}

	if q == bq {
		return 0, nil
	}
	if q < bq {
		return -1, nil
	}
	return 1, nil
}

func (q UInt64Quantity) ToUInt64() uint64 {
	return uint64(q)
}
