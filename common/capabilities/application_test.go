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

func TestApplicationV10(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{})
	require.NoError(t, ap.Supported())
}

func TestApplicationV11(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_1: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
}

func TestApplicationV12(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_2: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
	require.True(t, ap.V1_2Validation())
	require.True(t, ap.ACLs())
	require.True(t, ap.CollectionUpgrade())
	require.True(t, ap.PrivateChannelData())
}

func TestApplicationV13(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_3: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
	require.True(t, ap.V1_2Validation())
	require.True(t, ap.V1_3Validation())
	require.True(t, ap.KeyLevelEndorsement())
	require.True(t, ap.ACLs())
	require.True(t, ap.CollectionUpgrade())
	require.True(t, ap.PrivateChannelData())
}

func TestApplicationV142(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_4_2: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
	require.True(t, ap.V1_2Validation())
	require.True(t, ap.V1_3Validation())
	require.True(t, ap.KeyLevelEndorsement())
	require.True(t, ap.ACLs())
	require.True(t, ap.CollectionUpgrade())
	require.True(t, ap.PrivateChannelData())
	require.True(t, ap.StorePvtDataOfInvalidTx())
}

func TestApplicationV20(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV2_0: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
	require.True(t, ap.V1_2Validation())
	require.True(t, ap.V1_3Validation())
	require.True(t, ap.V2_0Validation())
	require.True(t, ap.KeyLevelEndorsement())
	require.True(t, ap.ACLs())
	require.True(t, ap.CollectionUpgrade())
	require.True(t, ap.PrivateChannelData())
	require.True(t, ap.LifecycleV20())
	require.True(t, ap.StorePvtDataOfInvalidTx())
}

func TestApplicationV25(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV2_5: {},
	})
	require.NoError(t, ap.Supported())
	require.True(t, ap.ForbidDuplicateTXIdInBlock())
	require.True(t, ap.V1_1Validation())
	require.True(t, ap.V1_2Validation())
	require.True(t, ap.V1_3Validation())
	require.True(t, ap.V2_0Validation())
	require.True(t, ap.KeyLevelEndorsement())
	require.True(t, ap.ACLs())
	require.True(t, ap.CollectionUpgrade())
	require.True(t, ap.PrivateChannelData())
	require.True(t, ap.LifecycleV20())
	require.True(t, ap.StorePvtDataOfInvalidTx())
	require.True(t, ap.PurgePvtData())
}

func TestApplicationPvtDataExperimental(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationPvtDataExperimental: {},
	})
	require.True(t, ap.PrivateChannelData())
}

func TestHasCapability(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{})
	require.True(t, ap.HasCapability(ApplicationV1_1))
	require.True(t, ap.HasCapability(ApplicationV1_2))
	require.True(t, ap.HasCapability(ApplicationV1_3))
	require.True(t, ap.HasCapability(ApplicationV2_0))
	require.True(t, ap.HasCapability(ApplicationV2_5))
	require.True(t, ap.HasCapability(ApplicationPvtDataExperimental))
	require.True(t, ap.HasCapability(ApplicationResourcesTreeExperimental))
	require.False(t, ap.HasCapability("default"))
}
