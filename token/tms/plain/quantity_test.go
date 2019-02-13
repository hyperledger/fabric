/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverflow(t *testing.T) {
	half := uint64(math.MaxUint64 / 2)
	assert.Equal(t, uint64(math.MaxUint64), uint64(half+half+1))

	a, err := ToQuantity(1)
	assert.NoError(t, err)
	b, err := ToQuantity(uint64(math.MaxUint64))
	assert.NoError(t, err)

	_, err = a.Add(b)
	assert.Error(t, err)

	_, err = b.Add(a)
	assert.Error(t, err)
}
