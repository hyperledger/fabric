/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

// TestDirectMessageOwnership pins the handoff contract: directMessage reports
// whether the message was forwarded, drops instead of blocking on a full
// queue, and drops responses while no state transfer is active.
func TestDirectMessageOwnership(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 1),
		stateResponseCh: make(chan protoext.ReceivedMessage, 1),
		stopCh:          make(chan struct{}),
	}

	messageOf := func(nonce uint64, response bool) protoext.ReceivedMessage {
		return stateTestMessage(t, nonce, response, "testchannel")
	}

	// Inactive state transfer: responses are dropped, not forwarded.
	require.False(t, s.directMessage(messageOf(1, true)))
	require.Empty(t, s.stateResponseCh)

	atomic.StoreInt32(&s.stateTransferActive, 1)

	// Forwarded while the queue has room; dropped when full, never blocking.
	require.True(t, s.directMessage(messageOf(2, true)))
	require.Len(t, s.stateResponseCh, 1)
	require.False(t, s.directMessage(messageOf(3, true)), "full queue must drop, not block")
	require.Equal(t, uint64(2), (<-s.stateResponseCh).GetGossipMessage().GetNonce())
	require.True(t, s.directMessage(messageOf(4, true)))

	// Requests behave the same on stateRequestCh.
	require.True(t, s.directMessage(messageOf(5, false)))
	require.False(t, s.directMessage(messageOf(6, false)), "full queue must drop, not block")
	require.Equal(t, uint64(5), (<-s.stateRequestCh).GetGossipMessage().GetNonce())

	// A message for another channel is dropped.
	wrongChannel := &comm.ReceivedMessageImpl{SignedGossipMessage: func() *protoext.SignedGossipMessage {
		sMsg, err := protoext.NoopSign(&proto.GossipMessage{
			Channel: []byte("otherchannel"),
			Content: &proto.GossipMessage_StateResponse{StateResponse: &proto.RemoteStateResponse{}},
		})
		require.NoError(t, err)
		return sMsg
	}()}
	require.False(t, s.directMessage(wrongChannel))
}
