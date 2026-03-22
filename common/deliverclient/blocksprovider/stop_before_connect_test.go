/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/stretchr/testify/require"
)

// TestDelivererStopBeforeConnect proves that calling Stop() before DeliverBlocks()
// has established a connection to an orderer (and therefore before blockReceiver is
// initialised) does not panic.
//
// Prior to the fix, d.blockReceiver was nil in this path and Stop() dereferenced it
// unconditionally, causing a nil-pointer panic.  This scenario is reached in
// production whenever a peer renounces gossip leadership
// (gossip/service/gossip_service.go calls StopDeliverForChannel) while the orderer
// is unreachable, which is common during network partitions or orderer maintenance.
func TestDelivererStopBeforeConnect(t *testing.T) {
	d := &blocksprovider.Deliverer{
		ChannelID: "test-channel",
		DoneC:     make(chan struct{}),
		Logger:    flogging.MustGetLogger("blocksprovider.test"),
	}

	require.NotPanics(t, func() {
		d.Stop()
	})
}

// TestBFTDelivererStopBeforeConnect is the BFT-path equivalent of the same bug.
// BFTDeliverer.Stop() contained the identical unconditional d.blockReceiver.Stop()
// call at bft_deliverer.go without a nil guard.
func TestBFTDelivererStopBeforeConnect(t *testing.T) {
	d := &blocksprovider.BFTDeliverer{
		ChannelID: "test-channel",
		DoneC:     make(chan struct{}),
		Logger:    flogging.MustGetLogger("BFTDeliverer.test"),
	}

	require.NotPanics(t, func() {
		d.Stop()
	})
}
