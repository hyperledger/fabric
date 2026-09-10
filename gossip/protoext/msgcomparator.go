/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"bytes"

	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/common"
)

// NewGossipMessageComparator creates a MessageReplacingPolicy given a maximum number of blocks to hold
func NewGossipMessageComparator(dataBlockStorageSize int) common.MessageReplacingPolicy {
	return (&msgComparator{dataBlockStorageSize: dataBlockStorageSize}).getMsgReplacingPolicy()
}

type msgComparator struct {
	dataBlockStorageSize int
}

func (mc *msgComparator) getMsgReplacingPolicy() common.MessageReplacingPolicy {
	return func(this any, that any) common.InvalidationResult {
		return mc.invalidationPolicy(this, that)
	}
}

func (mc *msgComparator) invalidationPolicy(this any, that any) common.InvalidationResult {
	thisMsg := this.(*SignedGossipMessage)
	thatMsg := that.(*SignedGossipMessage)

	if IsAliveMsg(thisMsg.GossipMessage) && IsAliveMsg(thatMsg.GossipMessage) {
		return aliveInvalidationPolicy(thisMsg.GetAliveMsg(), thatMsg.GetAliveMsg())
	}

	if IsDataMsg(thisMsg.GossipMessage) && IsDataMsg(thatMsg.GossipMessage) {
		return mc.dataInvalidationPolicy(thisMsg.GetDataMsg(), thatMsg.GetDataMsg())
	}

	if IsStateInfoMsg(thisMsg.GossipMessage) && IsStateInfoMsg(thatMsg.GossipMessage) {
		return mc.stateInvalidationPolicy(thisMsg.GetStateInfo(), thatMsg.GetStateInfo())
	}

	if IsIdentityMsg(thisMsg.GossipMessage) && IsIdentityMsg(thatMsg.GossipMessage) {
		return mc.identityInvalidationPolicy(thisMsg.GetPeerIdentity(), thatMsg.GetPeerIdentity())
	}

	if IsLeadershipMsg(thisMsg.GossipMessage) && IsLeadershipMsg(thatMsg.GossipMessage) {
		return leaderInvalidationPolicy(thisMsg.GetLeadershipMsg(), thatMsg.GetLeadershipMsg())
	}

	return common.MessageNoAction
}

func (mc *msgComparator) stateInvalidationPolicy(thisStateMsg *gossip.StateInfo, thatStateMsg *gossip.StateInfo) common.InvalidationResult {
	if !bytes.Equal(thisStateMsg.GetPkiId(), thatStateMsg.GetPkiId()) {
		return common.MessageNoAction
	}
	return compareTimestamps(thisStateMsg.GetTimestamp(), thatStateMsg.GetTimestamp())
}

func (mc *msgComparator) identityInvalidationPolicy(thisIdentityMsg *gossip.PeerIdentity, thatIdentityMsg *gossip.PeerIdentity) common.InvalidationResult {
	if bytes.Equal(thisIdentityMsg.GetPkiId(), thatIdentityMsg.GetPkiId()) {
		return common.MessageInvalidated
	}

	return common.MessageNoAction
}

func (mc *msgComparator) dataInvalidationPolicy(thisDataMsg *gossip.DataMessage, thatDataMsg *gossip.DataMessage) common.InvalidationResult {
	if thisDataMsg.GetPayload().GetSeqNum() == thatDataMsg.GetPayload().GetSeqNum() {
		return common.MessageInvalidated
	}

	diff := abs(thisDataMsg.GetPayload().GetSeqNum(), thatDataMsg.GetPayload().GetSeqNum())
	if diff <= uint64(mc.dataBlockStorageSize) {
		return common.MessageNoAction
	}

	if thisDataMsg.GetPayload().GetSeqNum() > thatDataMsg.GetPayload().GetSeqNum() {
		return common.MessageInvalidates
	}
	return common.MessageInvalidated
}

func aliveInvalidationPolicy(thisMsg *gossip.AliveMessage, thatMsg *gossip.AliveMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.GetMembership().GetPkiId(), thatMsg.GetMembership().GetPkiId()) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.GetTimestamp(), thatMsg.GetTimestamp())
}

func leaderInvalidationPolicy(thisMsg *gossip.LeadershipMessage, thatMsg *gossip.LeadershipMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.GetPkiId(), thatMsg.GetPkiId()) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.GetTimestamp(), thatMsg.GetTimestamp())
}

func compareTimestamps(thisTS *gossip.PeerTime, thatTS *gossip.PeerTime) common.InvalidationResult {
	if thisTS.GetIncNum() == thatTS.GetIncNum() {
		if thisTS.GetSeqNum() > thatTS.GetSeqNum() {
			return common.MessageInvalidates
		}

		return common.MessageInvalidated
	}
	if thisTS.GetIncNum() < thatTS.GetIncNum() {
		return common.MessageInvalidated
	}
	return common.MessageInvalidates
}

// abs returns abs(a-b)
func abs(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}
