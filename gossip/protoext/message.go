/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/gossip"
)

// IsAliveMsg returns whether this GossipMessage is an AliveMessage
func IsAliveMsg(m *gossip.GossipMessage) bool {
	return m.GetAliveMsg() != nil
}

// IsDataMsg returns whether this GossipMessage is a data message
func IsDataMsg(m *gossip.GossipMessage) bool {
	return m.GetDataMsg() != nil
}

// IsStateInfoPullRequestMsg returns whether this GossipMessage is a stateInfoPullRequest
func IsStateInfoPullRequestMsg(m *gossip.GossipMessage) bool {
	return m.GetStateInfoPullReq() != nil
}

// IsStateInfoSnapshot returns whether this GossipMessage is a stateInfo snapshot
func IsStateInfoSnapshot(m *gossip.GossipMessage) bool {
	return m.GetStateSnapshot() != nil
}

// IsStateInfoMsg returns whether this GossipMessage is a stateInfo message
func IsStateInfoMsg(m *gossip.GossipMessage) bool {
	return m.GetStateInfo() != nil
}

// IsPullMsg returns whether this GossipMessage is a message that belongs
// to the pull mechanism
func IsPullMsg(m *gossip.GossipMessage) bool {
	return m.GetDataReq() != nil || m.GetDataUpdate() != nil ||
		m.GetHello() != nil || m.GetDataDig() != nil
}

// IsRemoteStateMessage returns whether this GossipMessage is related to state synchronization
func IsRemoteStateMessage(m *gossip.GossipMessage) bool {
	return m.GetStateRequest() != nil || m.GetStateResponse() != nil
}

// GetPullMsgType returns the phase of the pull mechanism this GossipMessage belongs to
// for example: Hello, Digest, etc.
// If this isn't a pull message, PullMsgType_UNDEFINED is returned.
func GetPullMsgType(m *gossip.GossipMessage) gossip.PullMsgType {
	if helloMsg := m.GetHello(); helloMsg != nil {
		return helloMsg.MsgType
	}

	if digMsg := m.GetDataDig(); digMsg != nil {
		return digMsg.MsgType
	}

	if reqMsg := m.GetDataReq(); reqMsg != nil {
		return reqMsg.MsgType
	}

	if resMsg := m.GetDataUpdate(); resMsg != nil {
		return resMsg.MsgType
	}

	return gossip.PullMsgType_UNDEFINED
}

// IsChannelRestricted returns whether this GossipMessage should be routed
// only in its channel
func IsChannelRestricted(m *gossip.GossipMessage) bool {
	return m.Tag == gossip.GossipMessage_CHAN_AND_ORG || m.Tag == gossip.GossipMessage_CHAN_ONLY || m.Tag == gossip.GossipMessage_CHAN_OR_ORG
}

// IsOrgRestricted returns whether this GossipMessage should be routed only
// inside the organization
func IsOrgRestricted(m *gossip.GossipMessage) bool {
	return m.Tag == gossip.GossipMessage_CHAN_AND_ORG || m.Tag == gossip.GossipMessage_ORG_ONLY
}

// IsIdentityMsg returns whether this GossipMessage is an identity message
func IsIdentityMsg(m *gossip.GossipMessage) bool {
	return m.GetPeerIdentity() != nil
}

// IsDataReq returns whether this GossipMessage is a data request message
func IsDataReq(m *gossip.GossipMessage) bool {
	return m.GetDataReq() != nil
}

// IsPrivateDataMsg returns whether this message is related to private data
func IsPrivateDataMsg(m *gossip.GossipMessage) bool {
	return m.GetPrivateReq() != nil || m.GetPrivateRes() != nil || m.GetPrivateData() != nil
}

// IsAck returns whether this GossipMessage is an acknowledgement
func IsAck(m *gossip.GossipMessage) bool {
	return m.GetAck() != nil
}

// IsDataUpdate returns whether this GossipMessage is a data update message
func IsDataUpdate(m *gossip.GossipMessage) bool {
	return m.GetDataUpdate() != nil
}

// IsHelloMsg returns whether this GossipMessage is a hello message
func IsHelloMsg(m *gossip.GossipMessage) bool {
	return m.GetHello() != nil
}

// IsDigestMsg returns whether this GossipMessage is a digest message
func IsDigestMsg(m *gossip.GossipMessage) bool {
	return m.GetDataDig() != nil
}

// IsLeadershipMsg returns whether this GossipMessage is a leadership (leader election) message
func IsLeadershipMsg(m *gossip.GossipMessage) bool {
	return m.GetLeadershipMsg() != nil
}

// IsTagLegal checks the GossipMessage tags and inner type
// and returns an error if the tag doesn't match the type.
func IsTagLegal(m *gossip.GossipMessage) error {
	if m.Tag == gossip.GossipMessage_UNDEFINED {
		return fmt.Errorf("Undefined tag")
	}
	if IsDataMsg(m) {
		if m.Tag != gossip.GossipMessage_CHAN_AND_ORG {
			return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_CHAN_AND_ORG)])
		}
		return nil
	}

	if IsAliveMsg(m) || m.GetMemReq() != nil || m.GetMemRes() != nil {
		if m.Tag != gossip.GossipMessage_EMPTY {
			return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_EMPTY)])
		}
		return nil
	}

	if IsIdentityMsg(m) {
		if m.Tag != gossip.GossipMessage_ORG_ONLY {
			return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_ORG_ONLY)])
		}
		return nil
	}

	if IsPullMsg(m) {
		switch GetPullMsgType(m) {
		case gossip.PullMsgType_BLOCK_MSG:
			if m.Tag != gossip.GossipMessage_CHAN_AND_ORG {
				return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_CHAN_AND_ORG)])
			}
			return nil
		case gossip.PullMsgType_IDENTITY_MSG:
			if m.Tag != gossip.GossipMessage_EMPTY {
				return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_EMPTY)])
			}
			return nil
		default:
			return fmt.Errorf("Invalid PullMsgType: %s", gossip.PullMsgType_name[int32(GetPullMsgType(m))])
		}
	}

	if IsStateInfoMsg(m) || IsStateInfoPullRequestMsg(m) || IsStateInfoSnapshot(m) || IsRemoteStateMessage(m) {
		if m.Tag != gossip.GossipMessage_CHAN_OR_ORG {
			return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_CHAN_OR_ORG)])
		}
		return nil
	}

	if IsLeadershipMsg(m) {
		if m.Tag != gossip.GossipMessage_CHAN_AND_ORG {
			return fmt.Errorf("Tag should be %s", gossip.GossipMessage_Tag_name[int32(gossip.GossipMessage_CHAN_AND_ORG)])
		}
		return nil
	}

	return fmt.Errorf("Unknown message type: %v", m)
}
