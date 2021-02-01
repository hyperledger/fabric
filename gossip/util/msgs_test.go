/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"reflect"
	"testing"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

func init() {
	SetupTestLogging()
}

func TestMembershipStore(t *testing.T) {
	membershipStore := NewMembershipStore()

	id1 := common.PKIidType("id1")
	id2 := common.PKIidType("id2")

	msg1 := &protoext.SignedGossipMessage{}
	msg2 := &protoext.SignedGossipMessage{Envelope: &proto.Envelope{}}

	// Test initially created store is empty
	require.Nil(t, membershipStore.MsgByID(id1))
	require.Equal(t, membershipStore.Size(), 0)
	// Test put works as expected
	membershipStore.Put(id1, msg1)
	require.NotNil(t, membershipStore.MsgByID(id1))
	// Test MsgByID returns the right instance stored
	membershipStore.Put(id2, msg2)
	require.Equal(t, msg1, membershipStore.MsgByID(id1))
	require.NotEqual(t, msg2, membershipStore.MsgByID(id1))
	// Test capacity grows
	require.Equal(t, membershipStore.Size(), 2)
	// Test remove works
	membershipStore.Remove(id1)
	require.Nil(t, membershipStore.MsgByID(id1))
	require.Equal(t, membershipStore.Size(), 1)
	// Test returned instance is not a copy
	msg3 := &protoext.SignedGossipMessage{GossipMessage: &proto.GossipMessage{}}
	msg3Clone := &protoext.SignedGossipMessage{GossipMessage: &proto.GossipMessage{}}
	id3 := common.PKIidType("id3")
	membershipStore.Put(id3, msg3)
	require.Equal(t, msg3Clone, msg3)
	membershipStore.MsgByID(id3).Channel = []byte{0, 1, 2, 3}
	require.NotEqual(t, msg3Clone, msg3)
}

func TestToSlice(t *testing.T) {
	membershipStore := NewMembershipStore()
	id1 := common.PKIidType("id1")
	id2 := common.PKIidType("id2")
	id3 := common.PKIidType("id3")
	id4 := common.PKIidType("id4")

	msg1 := &protoext.SignedGossipMessage{}
	msg2 := &protoext.SignedGossipMessage{Envelope: &proto.Envelope{}}
	msg3 := &protoext.SignedGossipMessage{GossipMessage: &proto.GossipMessage{}}
	msg4 := &protoext.SignedGossipMessage{GossipMessage: &proto.GossipMessage{}, Envelope: &proto.Envelope{}}

	membershipStore.Put(id1, msg1)
	membershipStore.Put(id2, msg2)
	membershipStore.Put(id3, msg3)
	membershipStore.Put(id4, msg4)

	require.Len(t, membershipStore.ToSlice(), 4)

	existsInSlice := func(slice []*protoext.SignedGossipMessage, msg *protoext.SignedGossipMessage) bool {
		for _, m := range slice {
			if reflect.DeepEqual(m, msg) {
				return true
			}
		}
		return false
	}

	expectedMsgs := []*protoext.SignedGossipMessage{msg1, msg2, msg3, msg4}
	for _, msg := range membershipStore.ToSlice() {
		require.True(t, existsInSlice(expectedMsgs, msg))
	}
}
