/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/assert"
)

func TestApplicationV10(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{})
	assert.NoError(t, ap.Supported())
}

func TestApplicationV11(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_1: {},
	})
	assert.NoError(t, ap.Supported())
	assert.True(t, ap.ForbidDuplicateTXIdInBlock())
	assert.True(t, ap.V1_1Validation())
}

func TestApplicationV12(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_2: {},
	})
	assert.NoError(t, ap.Supported())
	assert.True(t, ap.ForbidDuplicateTXIdInBlock())
	assert.True(t, ap.V1_1Validation())
	assert.True(t, ap.V1_2Validation())
	assert.True(t, ap.ACLs())
	assert.True(t, ap.CollectionUpgrade())
	assert.True(t, ap.PrivateChannelData())
}

func TestApplicationV13(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_3: {},
	})
	assert.NoError(t, ap.Supported())
	assert.True(t, ap.ForbidDuplicateTXIdInBlock())
	assert.True(t, ap.V1_1Validation())
	assert.True(t, ap.V1_2Validation())
	assert.True(t, ap.V1_3Validation())
	assert.True(t, ap.KeyLevelEndorsement())
	assert.True(t, ap.ACLs())
	assert.True(t, ap.CollectionUpgrade())
	assert.True(t, ap.PrivateChannelData())
}

func TestApplicationV142(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV1_4_2: {},
	})
	assert.NoError(t, ap.Supported())
	assert.True(t, ap.ForbidDuplicateTXIdInBlock())
	assert.True(t, ap.V1_1Validation())
	assert.True(t, ap.V1_2Validation())
	assert.True(t, ap.V1_3Validation())
	assert.True(t, ap.KeyLevelEndorsement())
	assert.True(t, ap.ACLs())
	assert.True(t, ap.CollectionUpgrade())
	assert.True(t, ap.PrivateChannelData())
	assert.True(t, ap.StorePvtDataOfInvalidTx())
}

func TestApplicationV20(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationV2_0: {},
	})
	assert.NoError(t, ap.Supported())
	assert.True(t, ap.ForbidDuplicateTXIdInBlock())
	assert.True(t, ap.V1_1Validation())
	assert.True(t, ap.V1_2Validation())
	assert.True(t, ap.V1_3Validation())
	assert.True(t, ap.V2_0Validation())
	assert.True(t, ap.KeyLevelEndorsement())
	assert.True(t, ap.ACLs())
	assert.True(t, ap.CollectionUpgrade())
	assert.True(t, ap.PrivateChannelData())
	assert.True(t, ap.LifecycleV20())
	assert.True(t, ap.StorePvtDataOfInvalidTx())
}

func TestApplicationPvtDataExperimental(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{
		ApplicationPvtDataExperimental: {},
	})
	assert.True(t, ap.PrivateChannelData())
}

func TestHasCapability(t *testing.T) {
	ap := NewApplicationProvider(map[string]*cb.Capability{})
	assert.True(t, ap.HasCapability(ApplicationV1_1))
	assert.True(t, ap.HasCapability(ApplicationV1_2))
	assert.True(t, ap.HasCapability(ApplicationV1_3))
	assert.True(t, ap.HasCapability(ApplicationV2_0))
	assert.True(t, ap.HasCapability(ApplicationPvtDataExperimental))
	assert.True(t, ap.HasCapability(ApplicationResourcesTreeExperimental))
	assert.False(t, ap.HasCapability("default"))
}
