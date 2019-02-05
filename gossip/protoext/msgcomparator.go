/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"bytes"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/protos/gossip"
)

// Abs returns abs(a-b)
func abs(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

// NewGossipMessageComparator creates a MessageReplacingPolicy given a maximum number of blocks to hold
func NewGossipMessageComparator(dataBlockStorageSize int) common.MessageReplacingPolicy {
	return (&msgComparator{dataBlockStorageSize: dataBlockStorageSize}).getMsgReplacingPolicy()
}

type msgComparator struct {
	dataBlockStorageSize int
}

func (mc *msgComparator) getMsgReplacingPolicy() common.MessageReplacingPolicy {
	return func(this interface{}, that interface{}) common.InvalidationResult {
		return mc.invalidationPolicy(this, that)
	}
}

func (mc *msgComparator) invalidationPolicy(this interface{}, that interface{}) common.InvalidationResult {
	thisMsg := this.(*SignedGossipMessage)
	thatMsg := that.(*SignedGossipMessage)

	if thisMsg.IsAliveMsg() && thatMsg.IsAliveMsg() {
		return aliveInvalidationPolicy(thisMsg.GetAliveMsg(), thatMsg.GetAliveMsg())
	}

	if thisMsg.IsDataMsg() && thatMsg.IsDataMsg() {
		return mc.dataInvalidationPolicy(thisMsg.GetDataMsg(), thatMsg.GetDataMsg())
	}

	if thisMsg.IsStateInfoMsg() && thatMsg.IsStateInfoMsg() {
		return mc.stateInvalidationPolicy(thisMsg.GetStateInfo(), thatMsg.GetStateInfo())
	}

	if thisMsg.IsIdentityMsg() && thatMsg.IsIdentityMsg() {
		return mc.identityInvalidationPolicy(thisMsg.GetPeerIdentity(), thatMsg.GetPeerIdentity())
	}

	if thisMsg.IsLeadershipMsg() && thatMsg.IsLeadershipMsg() {
		return leaderInvalidationPolicy(thisMsg.GetLeadershipMsg(), thatMsg.GetLeadershipMsg())
	}

	return common.MessageNoAction
}

func (mc *msgComparator) stateInvalidationPolicy(thisStateMsg *gossip.StateInfo, thatStateMsg *gossip.StateInfo) common.InvalidationResult {
	if !bytes.Equal(thisStateMsg.PkiId, thatStateMsg.PkiId) {
		return common.MessageNoAction
	}
	return compareTimestamps(thisStateMsg.Timestamp, thatStateMsg.Timestamp)
}

func (mc *msgComparator) identityInvalidationPolicy(thisIdentityMsg *gossip.PeerIdentity, thatIdentityMsg *gossip.PeerIdentity) common.InvalidationResult {
	if bytes.Equal(thisIdentityMsg.PkiId, thatIdentityMsg.PkiId) {
		return common.MessageInvalidated
	}

	return common.MessageNoAction
}

func (mc *msgComparator) dataInvalidationPolicy(thisDataMsg *gossip.DataMessage, thatDataMsg *gossip.DataMessage) common.InvalidationResult {
	if thisDataMsg.Payload.SeqNum == thatDataMsg.Payload.SeqNum {
		return common.MessageInvalidated
	}

	diff := abs(thisDataMsg.Payload.SeqNum, thatDataMsg.Payload.SeqNum)
	if diff <= uint64(mc.dataBlockStorageSize) {
		return common.MessageNoAction
	}

	if thisDataMsg.Payload.SeqNum > thatDataMsg.Payload.SeqNum {
		return common.MessageInvalidates
	}
	return common.MessageInvalidated
}

func aliveInvalidationPolicy(thisMsg *gossip.AliveMessage, thatMsg *gossip.AliveMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.Membership.PkiId, thatMsg.Membership.PkiId) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.Timestamp, thatMsg.Timestamp)
}

func leaderInvalidationPolicy(thisMsg *gossip.LeadershipMessage, thatMsg *gossip.LeadershipMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.PkiId, thatMsg.PkiId) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.Timestamp, thatMsg.Timestamp)
}

func compareTimestamps(thisTS *gossip.PeerTime, thatTS *gossip.PeerTime) common.InvalidationResult {
	if thisTS.IncNum == thatTS.IncNum {
		if thisTS.SeqNum > thatTS.SeqNum {
			return common.MessageInvalidates
		}

		return common.MessageInvalidated
	}
	if thisTS.IncNum < thatTS.IncNum {
		return common.MessageInvalidated
	}
	return common.MessageInvalidates
}
