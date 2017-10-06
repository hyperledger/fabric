/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func TestApplicationV10(t *testing.T) {
	op := NewApplicationProvider(map[string]*cb.Capability{})
	assert.NoError(t, op.Supported())
	assert.False(t, op.LifecycleViaConfig())
}

func TestApplicationV11(t *testing.T) {
	op := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_1: &cb.Capability{},
	})
	assert.NoError(t, op.Supported())
	assert.True(t, op.LifecycleViaConfig())
}
