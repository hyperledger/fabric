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

func TestOrdererV10(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{})
	assert.NoError(t, op.Supported())
	assert.False(t, op.PredictableChannelTemplate())
}

func TestOrdererV11(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {},
	})
	assert.NoError(t, op.Supported())
	assert.True(t, op.PredictableChannelTemplate())
}
