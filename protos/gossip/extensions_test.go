/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckGossipMessageTypes(t *testing.T) {
	var msg *GossipMessage

	// Create State info pull request
	msg = &GossipMessage{
		Content: &GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &StateInfoPullRequest{
				Channel_MAC: []byte{17},
			},
		},
	}
	assert.True(t, msg.IsStateInfoPullRequestMsg())

	// Create alive message
	msg = &GossipMessage{
		Content: &GossipMessage_AliveMsg{
			AliveMsg: &AliveMessage{
				Identity: []byte("peerID"),
				Membership: &Member{
					PkiId:    []byte("pkiID"),
					Metadata: []byte{17},
					Endpoint: "localhost",
				},
				Timestamp: &PeerTime{
					SeqNum: 1,
					IncNum: 1,
				},
			},
		},
	}
	assert.True(t, msg.IsAliveMsg())

	// Create data message
	msg = &GossipMessage{
		Content: dataMessage(1, []byte{1, 2, 3, 4, 5}),
	}
	assert.True(t, msg.IsDataMsg())

	// Create data request message
	msg = &GossipMessage{
		Content: &GossipMessage_DataReq{
			DataReq: &DataRequest{
				MsgType: PullMsgType_UNDEFINED,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, msg.IsDataReq())
	assert.True(t, msg.IsPullMsg())

	// Create data request message
	msg = &GossipMessage{
		Content: &GossipMessage_DataDig{
			DataDig: &DataDigest{
				MsgType: PullMsgType_UNDEFINED,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, msg.IsDigestMsg())
	assert.True(t, msg.IsPullMsg())

	// Create data update message
	msg = &GossipMessage{
		Content: &GossipMessage_DataUpdate{
			DataUpdate: &DataUpdate{
				MsgType: PullMsgType_UNDEFINED,
				Nonce:   0,
				Data:    []*Envelope{envelopes()[0]},
			},
		},
	}
	assert.True(t, msg.IsDataUpdate())
	assert.True(t, msg.IsPullMsg())

	// Create hello message
	msg = &GossipMessage{
		Content: &GossipMessage_Hello{
			Hello: &GossipHello{
				MsgType: PullMsgType_UNDEFINED,
				Nonce:   0,
			},
		},
	}
	assert.True(t, msg.IsHelloMsg())
	assert.True(t, msg.IsPullMsg())

	// Create state request message
	msg = &GossipMessage{
		Content: &GossipMessage_StateRequest{
			StateRequest: &RemoteStateRequest{
				StartSeqNum: 1,
				EndSeqNum:   10,
			},
		},
	}
	assert.True(t, msg.IsRemoteStateMessage())

	// Create state response message
	msg = &GossipMessage{
		Content: &GossipMessage_StateResponse{
			StateResponse: &RemoteStateResponse{
				Payloads: []*Payload{{
					SeqNum: 1,
					Data:   []byte{1, 2, 3, 4, 5},
				}},
			},
		},
	}
	assert.True(t, msg.IsRemoteStateMessage())
}

func TestGossipPullMessageType(t *testing.T) {
	var msg *GossipMessage

	// Create hello message
	msg = &GossipMessage{
		Content: &GossipMessage_Hello{
			Hello: &GossipHello{
				MsgType: PullMsgType_BLOCK_MSG,
				Nonce:   0,
			},
		},
	}
	assert.True(t, msg.IsHelloMsg())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_BLOCK_MSG)

	// Create data request message
	msg = &GossipMessage{
		Content: &GossipMessage_DataDig{
			DataDig: &DataDigest{
				MsgType: PullMsgType_IDENTITY_MSG,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, msg.IsDigestMsg())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_IDENTITY_MSG)

	// Create data request message
	msg = &GossipMessage{
		Content: &GossipMessage_DataReq{
			DataReq: &DataRequest{
				MsgType: PullMsgType_BLOCK_MSG,
				Nonce:   0,
				Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
			},
		},
	}
	assert.True(t, msg.IsDataReq())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_BLOCK_MSG)

	// Create data update message
	msg = &GossipMessage{
		Content: &GossipMessage_DataUpdate{
			DataUpdate: &DataUpdate{
				MsgType: PullMsgType_IDENTITY_MSG,
				Nonce:   0,
				Data:    []*Envelope{envelopes()[0]},
			},
		},
	}
	assert.True(t, msg.IsDataUpdate())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_IDENTITY_MSG)

	// Create data message
	msg = &GossipMessage{
		Content: dataMessage(1, []byte{1, 2, 3, 4, 5}),
	}
	assert.True(t, msg.IsDataMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_UNDEFINED)
}

func TestGossipMessageDataMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag:     GossipMessage_CHAN_AND_ORG,
		Content: dataMessage(1, []byte{1}),
	}
	assert.True(t, msg.IsChannelRestricted())
	assert.True(t, msg.IsOrgRestricted())
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag:     GossipMessage_EMPTY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.Error(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag:     GossipMessage_UNDEFINED,
		Content: dataMessage(1, []byte{1}),
	}
	assert.Error(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag:     GossipMessage_ORG_ONLY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, msg.IsChannelRestricted())
	assert.True(t, msg.IsOrgRestricted())

	msg = &GossipMessage{
		Tag:     GossipMessage_CHAN_OR_ORG,
		Content: dataMessage(1, []byte{1}),
	}
	assert.True(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())

	msg = &GossipMessage{
		Tag:     GossipMessage_EMPTY,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())

	msg = &GossipMessage{
		Tag:     GossipMessage_UNDEFINED,
		Content: dataMessage(1, []byte{1}),
	}
	assert.False(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())
}

func TestGossipMessageAliveMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_AliveMsg{
			AliveMsg: &AliveMessage{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_ORG_ONLY,
		Content: &GossipMessage_AliveMsg{
			AliveMsg: &AliveMessage{},
		},
	}
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageMembershipMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_MemReq{
			MemReq: &MembershipRequest{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_MemRes{
			MemRes: &MembershipResponse{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())
}

func TestGossipMessageIdentityMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_ORG_ONLY,
		Content: &GossipMessage_PeerIdentity{
			PeerIdentity: &PeerIdentity{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_PeerIdentity{
			PeerIdentity: &PeerIdentity{},
		},
	}
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessagePullMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_AND_ORG,
		Content: &GossipMessage_DataReq{
			DataReq: &DataRequest{
				MsgType: PullMsgType_BLOCK_MSG,
			},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_DataReq{
			DataReq: &DataRequest{
				MsgType: PullMsgType_BLOCK_MSG,
			},
		},
	}
	assert.Error(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_DataDig{
			DataDig: &DataDigest{
				MsgType: PullMsgType_IDENTITY_MSG,
			},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_ORG_ONLY,
		Content: &GossipMessage_DataDig{
			DataDig: &DataDigest{
				MsgType: PullMsgType_IDENTITY_MSG,
			},
		},
	}
	assert.Error(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_ORG_ONLY,
		Content: &GossipMessage_DataDig{
			DataDig: &DataDigest{
				MsgType: PullMsgType_UNDEFINED,
			},
		},
	}
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageStateInfoMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_StateInfo{
			StateInfo: &StateInfo{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &StateInfoPullRequest{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_StateResponse{
			StateResponse: &RemoteStateResponse{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_StateRequest{
			StateRequest: &RemoteStateRequest{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_StateSnapshot{
			StateSnapshot: &StateInfoSnapshot{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_EMPTY,
		Content: &GossipMessage_StateInfo{
			StateInfo: &StateInfo{},
		},
	}
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageLeadershipMessageTagType(t *testing.T) {
	var msg *GossipMessage

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_AND_ORG,
		Content: &GossipMessage_LeadershipMsg{
			LeadershipMsg: &LeadershipMessage{},
		},
	}
	assert.NoError(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag: GossipMessage_CHAN_OR_ORG, Content: &GossipMessage_LeadershipMsg{
			LeadershipMsg: &LeadershipMessage{},
		},
	}
	assert.Error(t, msg.IsTagLegal())

	msg = &GossipMessage{
		Tag:     GossipMessage_CHAN_OR_ORG,
		Content: &GossipMessage_Empty{},
	}
	assert.Error(t, msg.IsTagLegal())
}

func envelopes() []*Envelope {
	return []*Envelope{
		{Payload: []byte{2, 2, 2},
			Signature: []byte{2, 2, 2},
			SecretEnvelope: &SecretEnvelope{
				Payload:   []byte{2, 2, 2},
				Signature: []byte{2, 2, 2},
			},
		},
	}
}

func dataMessage(seqNum uint64, data []byte) *GossipMessage_DataMsg {
	return &GossipMessage_DataMsg{
		DataMsg: &DataMessage{
			Payload: &Payload{
				SeqNum: seqNum,
				Data:   data,
			},
		},
	}
}

func TestToStringMembershipResponse(t *testing.T) {
	mr := &MembershipResponse{
		Alive: envelopes(),
		Dead:  envelopes(),
	}
	output := "MembershipResponse with Alive: 1, Dead: 1"
	assert.Equal(t, output, mr.ToString())
}

// func TestToStringMembershipRequest(t *testing.T) {
// 	gossipMessage := &GossipMessage{
// 		Nonce:   5,
// 		Channel: []byte("A"),
// 		Tag:     0,
// 		Content: &GossipMessage_DataMsg{
// 			DataMsg: &DataMessage{
// 				Payload: &Payload{
// 					SeqNum: 3,
// 					Data:   []byte{2, 2, 2, 2, 2},
// 				},
// 			},
// 		},
// 	}
// 	nn, _ := gossipMessage.NoopSign()
// 	sMsg := &SignedGossipMessage{
// 		GossipMessage: &GossipMessage{
// 			Tag:     GossipMessage_EMPTY,
// 			Nonce:   5,
// 			Channel: []byte("A"),
// 			Content: &GossipMessage_DataMsg{
// 				DataMsg: &DataMessage{
// 					Payload: &Payload{
// 						SeqNum: 3,
// 						Data:   []byte{2, 2, 2, 2, 2},
// 					},
// 				},
// 			},
// 		},
// 		Envelope: &Envelope{
// 			Payload:   nn.Envelope.Payload,
// 			Signature: []byte{0, 1, 2},
// 			SecretEnvelope: &SecretEnvelope{
// 				Payload:   []byte{0, 1, 2, 3, 4, 5},
// 				Signature: []byte{0, 1, 2},
// 			},
// 		},
// 	}
// 	mr := &MembershipRequest{
// 		SelfInformation: sMsg.Envelope,
// 		Known:           [][]byte{},
// 	}

// 	output := "Membership Request with self information of GossipMessage: Channel: A, nonce: 5, tag: UNDEFINED Block message: {Data: 5 bytes, seq: 3}, Envelope: 18 bytes, Signature: 3 bytes Secret payload: 6 bytes, Secret Signature: 3 bytes "
// 	assert.Equal(t, output, mr.toString())

// 	mr1 := &MembershipRequest{
// 		SelfInformation: &Envelope{
// 			Payload:   []byte{1, 2, 3},
// 			Signature: []byte{0, 1, 2},
// 			SecretEnvelope: &SecretEnvelope{
// 				Payload:   []byte{0, 1, 2, 3, 4, 5},
// 				Signature: []byte{0, 1, 2},
// 			},
// 		},
// 		Known: [][]byte{},
// 	}
// 	assert.Equal(t, "", mr1.toString())

// 	mr2 := &MembershipRequest{
// 		SelfInformation: nil,
// 		Known:           [][]byte{},
// 	}

// 	assert.Equal(t, "", mr2.toString())
// }

func TestToStringMember(t *testing.T) {
	member := &Member{
		Endpoint: "localhost",
		Metadata: []byte{1, 2, 3, 4, 5},
		PkiId:    []byte{15},
	}
	output := "Membership: Endpoint:localhost PKI-id:0f"
	assert.Equal(t, output, member.ToString())
}

func TestToStringAliveMessage(t *testing.T) {
	am1 := &AliveMessage{
		Membership: &Member{
			Endpoint: "localhost",
			Metadata: []byte{1, 2, 3, 4, 5},
			PkiId:    []byte{17},
		},
		Timestamp: &PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		Identity: []byte("peerID1"),
	}
	output1 := "Alive Message:Membership: Endpoint:localhost PKI-id:11Identity:Timestamp:inc_num:1 seq_num:1 "
	assert.Equal(t, output1, am1.ToString())
	am2 := &AliveMessage{
		Membership: nil,
		Timestamp: &PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		Identity: []byte("peerID1"),
	}
	output2 := "nil Membership"
	assert.Equal(t, output2, am2.ToString())
}

func TestToStringStateInfoPullRequest(t *testing.T) {
	// Create State info pull request
	sipr := &StateInfoPullRequest{
		Channel_MAC: []byte{17},
	}

	output := "state_info_pull_req: Channel MAC:11"
	assert.Equal(t, output, sipr.ToString())
}

func TestToStringStateInfo(t *testing.T) {
	si := &StateInfo{
		Timestamp: &PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		PkiId:       []byte{17},
		Channel_MAC: []byte{17},
		Properties:  nil,
	}
	output := "state_info_message: Timestamp:inc_num:1 seq_num:1 PKI-id:11 channel MAC:11 properties:<nil>"
	assert.Equal(t, output, si.ToString())

}

func TestToStringDataDigest(t *testing.T) {
	dig1 := &DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_BLOCK_MSG,
	}
	output1 := "data_dig: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	assert.Equal(t, output1, dig1.ToString())
	dig2 := &DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_IDENTITY_MSG,
	}
	output2 := "data_dig: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	assert.Equal(t, output2, dig2.ToString())
}

func TestToStringDataRequest(t *testing.T) {
	dataReq1 := &DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_BLOCK_MSG,
	}
	output1 := "data request: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	assert.Equal(t, output1, dataReq1.ToString())
	dataReq2 := &DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_IDENTITY_MSG,
	}
	output2 := "data request: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	assert.Equal(t, output2, dataReq2.ToString())
}

func TestToStringLeadershipMessage(t *testing.T) {
	lm := &LeadershipMessage{
		Timestamp: &PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		PkiId:         []byte{17},
		IsDeclaration: true,
	}
	output := "Leadership Message: PKI-id:11 Timestamp:inc_num:1 seq_num:1 Is Declaration true"
	assert.Equal(t, output, lm.ToString())
}
