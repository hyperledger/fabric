/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/require"
)

func TestChannelV10(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_0)
	require.False(t, cp.ConsensusTypeMigration())
	require.False(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV11(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_1)
	require.False(t, cp.ConsensusTypeMigration())
	require.False(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV13(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1: {},
		ChannelV1_3: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_3)
	require.False(t, cp.ConsensusTypeMigration())
	require.False(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_3)
	require.False(t, cp.ConsensusTypeMigration())
	require.False(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV142(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3:   {},
		ChannelV1_4_2: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_4_2: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV143(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_3:   {},
		ChannelV1_4_2: {},
		ChannelV1_4_3: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())

	cp = NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_4_3: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV20(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV2_0: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.False(t, cp.ConsensusTypeBFT())
}

func TestChannelV30(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV3_0: {},
	})
	require.NoError(t, cp.Supported())
	require.True(t, cp.MSPVersion() == msp.MSPv1_4_3)
	require.True(t, cp.ConsensusTypeMigration())
	require.True(t, cp.OrgSpecificOrdererEndpoints())
	require.True(t, cp.ConsensusTypeBFT())
}

func TestChannelNotSupported(t *testing.T) {
	cp := NewChannelProvider(map[string]*cb.Capability{
		ChannelV1_1:           {},
		ChannelV1_3:           {},
		"Bogus_Not_Supported": {},
	})
	require.EqualError(t, cp.Supported(), "Channel capability Bogus_Not_Supported is required but not supported")
}
