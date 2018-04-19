/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	// check the defaults
	assert.EqualValues(t, keepaliveOptions, DefaultKeepaliveOptions())
}
