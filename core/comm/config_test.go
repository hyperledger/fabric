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

	serverOptions := ServerKeepaliveOptions(nil)
	assert.NotNil(t, serverOptions)

	clientOptions := ClientKeepaliveOptions(nil)
	assert.NotNil(t, clientOptions)
}
