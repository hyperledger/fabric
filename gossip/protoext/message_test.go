/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/assert"
)

func TestCheckGossipMessageTypes(t *testing.T) {
	var msg *gossip.GossipMessage

	// Create State info pull request
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &gossip.StateInfoPullRequest{
				Channel_MAC: []byte{17},
			},
		},
	}
	assert.True(t, protoext.IsStateInfoPullRequestMsg(msg))

	// Create alive message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{
				Identity: []byte("peerID"),
				Membership: &gossip.Member{
					PkiId:    []byte("pkiID"),
					Metadata: []byte{17},
					Endpoint: "localhost",
				},
				Timestamp: &gossip.PeerTime{
					SeqNum: 1,
					IncNum: 1,
				},
			},
		},
	}
	assert.True(t, protoext.IsAliveMsg(msg))

	// Create gossip data message
	msg = &gossip.GossipMessage{
		Content: dataMessage(1, []byte{1, 2, 3, 4, 5}),
	}
	assert.True(t, protoext.IsDataMsg(msg))

	// Create data request message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataReq{
			DataReq: &gossip.DataRequest{
				MsgType: gossip.PullMsgType_UNDEFINED,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, protoext.IsDataReq(msg))
	assert.True(t, protoext.IsPullMsg(msg))

	// Create data request message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: gossip.PullMsgType_UNDEFINED,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, protoext.IsDigestMsg(msg))
	assert.True(t, protoext.IsPullMsg(msg))

	// Create data update message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataUpdate{
			DataUpdate: &gossip.DataUpdate{
				MsgType: gossip.PullMsgType_UNDEFINED,
				Nonce:   0,
				Data:    []*gossip.Envelope{envelopes()[0]},
			},
		},
	}
	assert.True(t, protoext.IsDataUpdate(msg))
	assert.True(t, protoext.IsPullMsg(msg))

	// Create gossip hello message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_Hello{
			Hello: &gossip.GossipHello{
				MsgType: gossip.PullMsgType_UNDEFINED,
				Nonce:   0,
			},
		},
	}
	assert.True(t, protoext.IsHelloMsg(msg))
	assert.True(t, protoext.IsPullMsg(msg))

	// Create state request message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateRequest{
			StateRequest: &gossip.RemoteStateRequest{
				StartSeqNum: 1,
				EndSeqNum:   10,
			},
		},
	}
	assert.True(t, protoext.IsRemoteStateMessage(msg))

	// Create state response message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateResponse{
			StateResponse: &gossip.RemoteStateResponse{
				Payloads: []*gossip.Payload{{
					SeqNum: 1,
					Data:   []byte{1, 2, 3, 4, 5},
				}},
			},
		},
	}
	assert.True(t, protoext.IsRemoteStateMessage(msg))
}

func TestGossipPullMessageType(t *testing.T) {
	var msg *gossip.GossipMessage

	// Create gossip hello message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_Hello{
			Hello: &gossip.GossipHello{
				MsgType: gossip.PullMsgType_BLOCK_MSG,
				Nonce:   0,
			},
		},
	}
	assert.True(t, protoext.IsHelloMsg(msg))
	assert.True(t, protoext.IsPullMsg(msg))
	assert.Equal(t, protoext.GetPullMsgType(msg), gossip.PullMsgType_BLOCK_MSG)

	// Create data request message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: gossip.PullMsgType_IDENTITY_MSG,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, protoext.IsDigestMsg(msg))
	assert.True(t, protoext.IsPullMsg(msg))
	assert.Equal(t, protoext.GetPullMsgType(msg), gossip.PullMsgType_IDENTITY_MSG)

	// Create data request message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataReq{
			DataReq: &gossip.DataRequest{
				MsgType: gossip.PullMsgType_BLOCK_MSG,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, protoext.IsDataReq(msg))
	assert.True(t, protoext.IsPullMsg(msg))
	assert.Equal(t, protoext.GetPullMsgType(msg), gossip.PullMsgType_BLOCK_MSG)

	// Create data update message
	msg = &gossip.GossipMessage{
		Content: &gossip.GossipMessage_DataUpdate{
			DataUpdate: &gossip.DataUpdate{
				MsgType: gossip.PullMsgType_IDENTITY_MSG,
				Nonce:   0,
				Data:    []*gossip.Envelope{envelopes()[0]},
			},
		},
	}
	assert.True(t, protoext.IsDataUpdate(msg))
	assert.True(t, protoext.IsPullMsg(msg))
	assert.Equal(t, protoext.GetPullMsgType(msg), gossip.PullMsgType_IDENTITY_MSG)

	// Create gossip data message
	msg = &gossip.GossipMessage{
		Content: dataMessage(1, []byte{1, 2, 3, 4, 5}),
	}
	assert.True(t, protoext.IsDataMsg(msg))
	assert.Equal(t, protoext.GetPullMsgType(msg), gossip.PullMsgType_UNDEFINED)
}

func TestGossipMessageDataMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_CHAN_AND_ORG,
		Content: dataMessage(1, []byte{1}),
	}
	assert.True(t, protoext.IsChannelRestricted(msg))
	assert.True(t, protoext.IsOrgRestricted(msg))
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_EMPTY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.Error(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_UNDEFINED,
		Content: dataMessage(1, []byte{1}),
	}
	assert.Error(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_ORG_ONLY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, protoext.IsChannelRestricted(msg))
	assert.True(t, protoext.IsOrgRestricted(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_CHAN_OR_ORG,
		Content: dataMessage(1, []byte{1}),
	}
	assert.True(t, protoext.IsChannelRestricted(msg))
	assert.False(t, protoext.IsOrgRestricted(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_EMPTY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, protoext.IsChannelRestricted(msg))
	assert.False(t, protoext.IsOrgRestricted(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_UNDEFINED,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, protoext.IsChannelRestricted(msg))
	assert.False(t, protoext.IsOrgRestricted(msg))
}

func TestGossipMessageAliveMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_ORG_ONLY,
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))
}

func TestGossipMessageMembershipMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_MemReq{
			MemReq: &gossip.MembershipRequest{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_MemRes{
			MemRes: &gossip.MembershipResponse{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))
}

func TestGossipMessageIdentityMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_ORG_ONLY,
		Content: &gossip.GossipMessage_PeerIdentity{
			PeerIdentity: &gossip.PeerIdentity{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_PeerIdentity{
			PeerIdentity: &gossip.PeerIdentity{},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))
}

func TestGossipMessagePullMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_AND_ORG,
		Content: &gossip.GossipMessage_DataReq{
			DataReq: &gossip.DataRequest{
				MsgType: gossip.PullMsgType_BLOCK_MSG,
			},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_DataReq{
			DataReq: &gossip.DataRequest{
				MsgType: gossip.PullMsgType_BLOCK_MSG,
			},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: gossip.PullMsgType_IDENTITY_MSG,
			},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_ORG_ONLY,
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: gossip.PullMsgType_IDENTITY_MSG,
			},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_ORG_ONLY,
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: gossip.PullMsgType_UNDEFINED,
			},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))
}

func TestGossipMessageStateInfoMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &gossip.StateInfoPullRequest{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_StateResponse{
			StateResponse: &gossip.RemoteStateResponse{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_StateRequest{
			StateRequest: &gossip.RemoteStateRequest{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_StateSnapshot{
			StateSnapshot: &gossip.StateInfoSnapshot{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))
}

func TestGossipMessageLeadershipMessageTagType(t *testing.T) {
	var msg *gossip.GossipMessage

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_AND_ORG,
		Content: &gossip.GossipMessage_LeadershipMsg{
			LeadershipMsg: &gossip.LeadershipMessage{},
		},
	}
	assert.NoError(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag: gossip.GossipMessage_CHAN_OR_ORG, Content: &gossip.GossipMessage_LeadershipMsg{
			LeadershipMsg: &gossip.LeadershipMessage{},
		},
	}
	assert.Error(t, protoext.IsTagLegal(msg))

	msg = &gossip.GossipMessage{
		Tag:     gossip.GossipMessage_CHAN_OR_ORG,
		Content: &gossip.GossipMessage_Empty{},
	}
	assert.Error(t, protoext.IsTagLegal(msg))
}
