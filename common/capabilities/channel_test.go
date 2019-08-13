/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestChannelV10(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_0)
	assert.False(t, cp.ConsensusTypeMigration())
	assert.False(t, cp.OrgSpecificOrdererEndpoints())
}

func TestChannelV11(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_1)
	assert.False(t, cp.ConsensusTypeMigration())
	assert.False(t, cp.OrgSpecificOrdererEndpoints())
}

func TestChannelV13(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1: {},
		ChannelV1_3: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_3)
	assert.False(t, cp.ConsensusTypeMigration())
	assert.False(t, cp.OrgSpecificOrdererEndpoints())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_3)
	assert.False(t, cp.ConsensusTypeMigration())
	assert.False(t, cp.OrgSpecificOrdererEndpoints())
}

func TestChannelV142(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3:   {},
		ChannelV1_4_2: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_3)
	assert.True(t, cp.ConsensusTypeMigration())
	assert.True(t, cp.OrgSpecificOrdererEndpoints())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_4_2: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_3)
	assert.True(t, cp.ConsensusTypeMigration())
	assert.True(t, cp.OrgSpecificOrdererEndpoints())
}

func TestChannelV143(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3:   {},
		ChannelV1_4_2: {},
		ChannelV1_4_3: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	assert.True(t, cp.ConsensusTypeMigration())
	assert.True(t, cp.OrgSpecificOrdererEndpoints())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_4_3: {},
	})
	assert.NoError(t, cp.Supported())
	assert.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	assert.True(t, cp.ConsensusTypeMigration())
	assert.True(t, cp.OrgSpecificOrdererEndpoints())
}

func TestChannelNotSuported(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1:          {},
		ChannelV1_3:          {},
		"Bogus_Not_suported": {},
	})
	assert.EqualError(t, cp.Supported(), "Channel capability Bogus_Not_suported is required but not supported")
}
