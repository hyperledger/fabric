/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"math"
	"testing"

	. "github.com/hyperledger/fabric/token/tms/plain"
	"github.com/stretchr/testify/assert"
)

func TestToQuantity(t *testing.T) {
	_, err := ToQuantity(ToHex(100), 0)
	assert.Equal(t, "precision be larger than 0", err.Error())

	_, err = ToQuantity(ToHex(0), 64)
	assert.Equal(t, "quantity must be larger than 0", err.Error())

	_, err = ToQuantity(IntToHex(-100), 64)
	assert.Equal(t, "invalid input", err.Error())

	_, err = ToQuantity("abc", 64)
	assert.Equal(t, "invalid input", err.Error())

	_, err = ToQuantity("0babc", 64)
	assert.Equal(t, "invalid input", err.Error())

	_, err = ToQuantity("0abc", 64)
	assert.Equal(t, "invalid input", err.Error())

	_, err = ToQuantity("0xabc", 2)
	assert.Equal(t, "0xabc has precision 12 > 2", err.Error())

	_, err = ToQuantity("10231", 64)
	assert.NoError(t, err)

	_, err = ToQuantity("0xABC", 64)
	assert.NoError(t, err)

	_, err = ToQuantity("0XABC", 64)
	assert.NoError(t, err)

	_, err = ToQuantity("0XAbC", 64)
	assert.NoError(t, err)

	_, err = ToQuantity("0xAbC", 64)
	assert.NoError(t, err)
}

func TestDecimal(t *testing.T) {
	q, err := ToQuantity("10231", 64)
	assert.NoError(t, err)
	assert.Equal(t, "10231", q.Decimal())
}

func TestHex(t *testing.T) {
	q, err := ToQuantity("0xabc", 64)
	assert.NoError(t, err)
	assert.Equal(t, "0xabc", q.Hex())
}

func TestOverflow(t *testing.T) {
	half := uint64(math.MaxUint64 / 2)
	assert.Equal(t, uint64(math.MaxUint64), uint64(half+half+1))

	a, err := ToQuantity(ToHex(1), 64)
	assert.NoError(t, err)
	b, err := ToQuantity(ToHex(uint64(math.MaxUint64)), 64)
	assert.NoError(t, err)

	_, err = a.Add(b)
	assert.Error(t, err)

	_, err = b.Add(a)
	assert.Error(t, err)
}
