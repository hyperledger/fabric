/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func TestOrdererV10(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{})
	require.NoError(t, op.Supported())
	require.Equal(t, ordererTypeName, op.Type())
	require.False(t, op.PredictableChannelTemplate())
	require.False(t, op.Resubmission())
	require.False(t, op.ExpirationCheck())
	require.False(t, op.ConsensusTypeMigration())
	require.False(t, op.UseChannelCreationPolicyAsAdmins())
}

func TestOrdererV11(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {},
	})
	require.NoError(t, op.Supported())
	require.True(t, op.PredictableChannelTemplate())
	require.True(t, op.Resubmission())
	require.True(t, op.ExpirationCheck())
	require.False(t, op.ConsensusTypeMigration())
	require.False(t, op.UseChannelCreationPolicyAsAdmins())
}

func TestOrdererV142(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_4_2: {},
	})
	require.NoError(t, op.Supported())
	require.True(t, op.PredictableChannelTemplate())
	require.True(t, op.Resubmission())
	require.True(t, op.ExpirationCheck())
	require.True(t, op.ConsensusTypeMigration())
	require.False(t, op.UseChannelCreationPolicyAsAdmins())
}

func TestOrdererV20(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV2_0: {},
	})
	require.NoError(t, op.Supported())
	require.True(t, op.PredictableChannelTemplate())
	require.True(t, op.UseChannelCreationPolicyAsAdmins())
	require.True(t, op.Resubmission())
	require.True(t, op.ExpirationCheck())
	require.True(t, op.ConsensusTypeMigration())
}

func TestNotSupported(t *testing.T) {
	op := NewOrdererProvider(map[string]*cb.Capability{
		OrdererV1_1: {}, OrdererV2_0: {}, "Bogus_Not_Supported": {},
	})
	require.EqualError(t, op.Supported(), "Orderer capability Bogus_Not_Supported is required but not supported")
}
