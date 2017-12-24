/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNotSame(t *testing.T) {
	id := PKIidType("1")
	assert.True(t, id.IsNotSameFilter(PKIidType("2")))
	assert.False(t, id.IsNotSameFilter(PKIidType("1")))
	assert.False(t, id.IsNotSameFilter(id))
}
