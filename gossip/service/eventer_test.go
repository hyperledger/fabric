/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
)

func init() {
	util.SetupTestLogging()
}

type mockReceiver struct {
	cu          ConfigUpdate
	updateCount int
}

func (mr *mockReceiver) updateAnchors(cu ConfigUpdate) {
	logger.Debugf("[TEST] Setting config to %d %v", cu.Sequence, cu.Organizations)
	mr.updateCount++
	mr.cu = cu
}

func TestInitialUpdate(t *testing.T) {
	cu := ConfigUpdate{
		ChannelID:        "channel-id",
		OrdererAddresses: []string{"localhost:7050"},
		Sequence:         7,
		Organizations: map[string]channelconfig.ApplicationOrg{
			"testID": &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
			},
		},
	}

	mr := &mockReceiver{}
	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(cu)

	require.Equal(t, cu, mr.cu, "should have updated config on initial update but did not")
}

func TestSecondUpdate(t *testing.T) {
	cu := ConfigUpdate{
		ChannelID:        "channel-id",
		OrdererAddresses: []string{"localhost:7050"},
		Sequence:         7,
		Organizations: map[string]channelconfig.ApplicationOrg{
			"testID": &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
			},
		},
	}

	mr := &mockReceiver{}
	ce := newConfigEventer(mr)

	ce.ProcessConfigUpdate(cu)
	require.Equal(t, cu, mr.cu, "should have updated config on initial update but did not")
	require.Equal(t, 1, mr.updateCount)

	cu.Sequence = 8
	cu.Organizations["testID"] = &appGrp{
		anchorPeers: []*peer.AnchorPeer{{Port: 10}},
	}

	ce.ProcessConfigUpdate(cu)
	require.Equal(t, cu, mr.cu, "should have updated config on second update but did not")
	require.Equal(t, 2, mr.updateCount)
}

func TestSecondSameUpdate(t *testing.T) {
	cu := ConfigUpdate{
		ChannelID:        "channel-id",
		OrdererAddresses: []string{"localhost:7050"},
		Sequence:         7,
		Organizations: map[string]channelconfig.ApplicationOrg{
			"testID": &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
			},
		},
	}

	mr := &mockReceiver{}
	ce := newConfigEventer(mr)

	ce.ProcessConfigUpdate(cu)
	require.Equal(t, cu, mr.cu, "should have updated config on initial update but did not")
	require.Equal(t, 1, mr.updateCount)

	ce.ProcessConfigUpdate(
		ConfigUpdate{Organizations: cu.Organizations},
	)
	require.Equal(t, cu, mr.cu, "should not have updated when reprocessing the same config")
	require.Equal(t, 1, mr.updateCount)
}

func TestUpdatedSeqOnly(t *testing.T) {
	cu := ConfigUpdate{
		ChannelID:        "channel-id",
		OrdererAddresses: []string{"localhost:7050"},
		Sequence:         7,
		Organizations: map[string]channelconfig.ApplicationOrg{
			"testID": &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
			},
		},
	}

	mr := &mockReceiver{}
	ce := newConfigEventer(mr)

	ce.ProcessConfigUpdate(cu)
	require.Equal(t, cu, mr.cu, "should have updated config on initial update but did not")
	require.Equal(t, 1, mr.updateCount)

	cu2 := cu
	cu2.Sequence = 9
	ce.ProcessConfigUpdate(cu2)
	require.Equal(t, cu, mr.cu, "should not have updated config when reprocessing same config")
	require.Equal(t, 1, mr.updateCount)
}
