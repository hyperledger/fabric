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

func TestChannelV10(t *testing.T) {
	op := NewChannelProvider(map[string]*cb.Capability{})
	assert.NoError(t, op.Supported())
	assert.True(t, op.MSPVersion() == MSPv1_0)
}

func TestChannelV11(t *testing.T) {
	op := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1: &cb.Capability{},
	})
	assert.NoError(t, op.Supported())
	assert.True(t, op.MSPVersion() == MSPv1_1)
}
