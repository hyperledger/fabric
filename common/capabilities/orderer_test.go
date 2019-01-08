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
	assert.Equal(t, ordererTypeName, op.Type())
	assert.False(t, op.PredictableChannelTemplate())
	assert.False(t, op.Resubmission())
	assert.False(t, op.ExpirationCheck())
	assert.False(t, op.Kafka2RaftMigration())
}

func TestOrdererV11(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {},
	})
	assert.NoError(t, op.Supported())
	assert.True(t, op.PredictableChannelTemplate())
	assert.True(t, op.Resubmission())
	assert.True(t, op.ExpirationCheck())
	assert.False(t, op.Kafka2RaftMigration())
}

func TestOrdererV20(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {}, OrdererV2_0: {},
	})
	assert.NoError(t, op.Supported())
	assert.True(t, op.PredictableChannelTemplate())
	assert.True(t, op.Resubmission())
	assert.True(t, op.ExpirationCheck())
	assert.True(t, op.Kafka2RaftMigration())
}

func TestNotSuported(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {}, OrdererV2_0: {}, "Bogus_Not_suported": {},
	})
	assert.EqualError(t, op.Supported(), "Orderer capability Bogus_Not_suported is required but not supported")
}
