/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestToGossipMessageNilEnvelope(t *testing.T) {
	memReq := &MembershipRequest{}
	_, err := memReq.SelfInformation.ToGossipMessage()
	assert.EqualError(t, err, "nil envelope")
}

func TestToString(t *testing.T) {
	// Ensure we don't print the byte content when we
	// log messages.
	// Each payload or signature contains '2' so we would've logged
	// them if not for the overloading of the String() method in SignedGossipMessage

	// The following line proves that the envelopes constructed in this test
	// have "2" in them when they are printed
	assert.Contains(t, fmt.Sprintf("%v", envelopes()[0]), "2")
	// and the following does the same for payloads:
	dMsg := &DataMessage{
		Payload: &Payload{
			SeqNum: 3,
			Data:   []byte{2, 2, 2, 2, 2},
		},
	}
	assert.Contains(t, fmt.Sprintf("%v", dMsg), "2")

	// Now we construct all types of messages that have envelopes or payloads in them
	// and see that "2" is not outputted into their formatting even though it is found
	// as a sub-message of the outer message.

	sMsg := &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Channel: []byte("A"),
			Content: &GossipMessage_DataMsg{
				DataMsg: &DataMessage{
					Payload: &Payload{
						SeqNum: 3,
						Data:   []byte{2, 2, 2, 2, 2},
					},
				},
			},
		},
		Envelope: &Envelope{
			Payload:   []byte{0, 1, 2, 3, 4, 5, 6},
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")
	sMsg.GetDataMsg().Payload = nil
	assert.NotPanics(t, func() {
		_ = sMsg.String()
	})

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_DataUpdate{
				DataUpdate: &DataUpdate{
					Nonce:   11,
					MsgType: PullMsgType_BLOCK_MSG,
					Data:    envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_MemRes{
				MemRes: &MembershipResponse{
					Alive: envelopes(),
					Dead:  envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_StateSnapshot{
				StateSnapshot: &StateInfoSnapshot{
					Elements: envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_AliveMsg{
				AliveMsg: &AliveMessage{
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
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_StateResponse{
				StateResponse: &RemoteStateResponse{
					Payloads: []*Payload{
						{Data: []byte{2, 2, 2}},
					},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_MemReq{
				MemReq: &MembershipRequest{
					SelfInformation: sMsg.Envelope,
					Known:           [][]byte{},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_StateInfoPullReq{
				StateInfoPullReq: &StateInfoPullRequest{
					Channel_MAC: []byte{17},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_StateInfo{
				StateInfo: &StateInfo{
					Channel_MAC: []byte{17},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_DataDig{
				DataDig: &DataDigest{
					Nonce:   0,
					Digests: [][]byte{[]byte("msg1"), []byte("msg2")},
					MsgType: PullMsgType_BLOCK_MSG,
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.Contains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_DataReq{
				DataReq: &DataRequest{
					Nonce:   0,
					Digests: [][]byte{[]byte("msg1"), []byte("msg2")},
					MsgType: PullMsgType_BLOCK_MSG,
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.Contains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("A"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Content: &GossipMessage_LeadershipMsg{
				LeadershipMsg: &LeadershipMessage{
					Timestamp: &PeerTime{
						IncNum: 1,
						SeqNum: 1,
					},
					PkiId:         []byte{17},
					IsDeclaration: true,
				},
			},
		},
		Envelope: envelopes()[0],
	}
	assert.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

}

func TestCheckGossipMessageTypes(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	// Create State info pull request
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_StateInfoPullReq{
		StateInfoPullReq: &StateInfoPullRequest{
			Channel_MAC: []byte{17},
		},
	})

	assert.True(t, msg.IsStateInfoPullRequestMsg())

	// Create alive message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_AliveMsg{
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
	})

	assert.True(t, msg.IsAliveMsg())

	// Create gossip data message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, dataMessage(1, []byte{1, 2, 3, 4, 5}))
	assert.True(t, msg.IsDataMsg())

	// Create data request message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataReq{
		DataReq: &DataRequest{
			MsgType: PullMsgType_UNDEFINED,
			Nonce:   0,
			Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
	})
	assert.True(t, msg.IsDataReq())
	assert.True(t, msg.IsPullMsg())

	// Create data request message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataDig{
		DataDig: &DataDigest{
			MsgType: PullMsgType_UNDEFINED,
			Nonce:   0,
			Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
	})
	assert.True(t, msg.IsDigestMsg())
	assert.True(t, msg.IsPullMsg())

	// Create data update message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataUpdate{
		DataUpdate: &DataUpdate{
			MsgType: PullMsgType_UNDEFINED,
			Nonce:   0,
			Data:    []*Envelope{envelopes()[0]},
		},
	})
	assert.True(t, msg.IsDataUpdate())
	assert.True(t, msg.IsPullMsg())

	// Create gossip hello message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_Hello{
		Hello: &GossipHello{
			MsgType: PullMsgType_UNDEFINED,
			Nonce:   0,
		},
	})
	assert.True(t, msg.IsHelloMsg())
	assert.True(t, msg.IsPullMsg())

	// Create state request message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_StateRequest{
		StateRequest: &RemoteStateRequest{
			StartSeqNum: 1,
			EndSeqNum:   10,
		},
	})
	assert.True(t, msg.IsRemoteStateMessage())

	// Create state response message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_StateResponse{
		StateResponse: &RemoteStateResponse{
			Payloads: []*Payload{{
				SeqNum: 1,
				Data:   []byte{1, 2, 3, 4, 5},
			}},
		},
	})
	assert.True(t, msg.IsRemoteStateMessage())
}

func TestGossipPullMessageType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	// Create gossip hello message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_Hello{
		Hello: &GossipHello{
			MsgType: PullMsgType_BLOCK_MSG,
			Nonce:   0,
		},
	})

	assert.True(t, msg.IsHelloMsg())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_BLOCK_MSG)

	// Create data request message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataDig{
		DataDig: &DataDigest{
			MsgType: PullMsgType_IDENTITY_MSG,
			Nonce:   0,
			Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
	})
	assert.True(t, msg.IsDigestMsg())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_IDENTITY_MSG)

	// Create data request message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataReq{
		DataReq: &DataRequest{
			MsgType: PullMsgType_BLOCK_MSG,
			Nonce:   0,
			Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
	})
	assert.True(t, msg.IsDataReq())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_BLOCK_MSG)

	// Create data update message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataUpdate{
		DataUpdate: &DataUpdate{
			MsgType: PullMsgType_IDENTITY_MSG,
			Nonce:   0,
			Data:    []*Envelope{envelopes()[0]},
		},
	})
	assert.True(t, msg.IsDataUpdate())
	assert.True(t, msg.IsPullMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_IDENTITY_MSG)

	// Create gossip data message
	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, dataMessage(1, []byte{1, 2, 3, 4, 5}))
	assert.True(t, msg.IsDataMsg())
	assert.Equal(t, msg.GetPullMsgType(), PullMsgType_UNDEFINED)
}

func TestGossipMessageDataMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_AND_ORG, dataMessage(1, []byte{1}))
	assert.True(t, msg.IsChannelRestricted())
	assert.True(t, msg.IsOrgRestricted())
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, dataMessage(1, []byte{1}))
	assert.Error(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_UNDEFINED, dataMessage(1, []byte{1}))
	assert.Error(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_ORG_ONLY, dataMessage(1, []byte{1}))
	assert.False(t, msg.IsChannelRestricted())
	assert.True(t, msg.IsOrgRestricted())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, dataMessage(1, []byte{1}))
	assert.True(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, dataMessage(1, []byte{1}))
	assert.False(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())

	msg = signedGossipMessage(channelID, GossipMessage_UNDEFINED, dataMessage(1, []byte{1}))
	assert.False(t, msg.IsChannelRestricted())
	assert.False(t, msg.IsOrgRestricted())
}

func TestGossipMessageAliveMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_ORG_ONLY, &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{},
	})
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageMembershipMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_MemReq{
		MemReq: &MembershipRequest{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_MemRes{
		MemRes: &MembershipResponse{},
	})
	assert.NoError(t, msg.IsTagLegal())
}

func TestGossipMessageIdentityMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_ORG_ONLY, &GossipMessage_PeerIdentity{
		PeerIdentity: &PeerIdentity{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_PeerIdentity{
		PeerIdentity: &PeerIdentity{},
	})
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessagePullMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_AND_ORG, &GossipMessage_DataReq{
		DataReq: &DataRequest{
			MsgType: PullMsgType_BLOCK_MSG,
		},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataReq{
		DataReq: &DataRequest{
			MsgType: PullMsgType_BLOCK_MSG,
		},
	})
	assert.Error(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataDig{
		DataDig: &DataDigest{
			MsgType: PullMsgType_IDENTITY_MSG,
		},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_ORG_ONLY, &GossipMessage_DataDig{
		DataDig: &DataDigest{
			MsgType: PullMsgType_IDENTITY_MSG,
		},
	})
	assert.Error(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_ORG_ONLY, &GossipMessage_DataDig{
		DataDig: &DataDigest{
			MsgType: PullMsgType_UNDEFINED,
		},
	})
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageStateInfoMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_StateInfo{
		StateInfo: &StateInfo{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_StateInfoPullReq{
		StateInfoPullReq: &StateInfoPullRequest{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_StateResponse{
		StateResponse: &RemoteStateResponse{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_StateRequest{
		StateRequest: &RemoteStateRequest{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_StateSnapshot{
		StateSnapshot: &StateInfoSnapshot{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_StateInfo{
		StateInfo: &StateInfo{},
	})
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageLeadershipMessageTagType(t *testing.T) {
	var msg *SignedGossipMessage
	channelID := "testID1"

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_AND_ORG, &GossipMessage_LeadershipMsg{
		LeadershipMsg: &LeadershipMessage{},
	})
	assert.NoError(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_LeadershipMsg{
		LeadershipMsg: &LeadershipMessage{},
	})
	assert.Error(t, msg.IsTagLegal())

	msg = signedGossipMessage(channelID, GossipMessage_CHAN_OR_ORG, &GossipMessage_Empty{})
	assert.Error(t, msg.IsTagLegal())
}

func TestGossipMessageSign(t *testing.T) {
	idSigner := func(msg []byte) ([]byte, error) {
		return msg, nil
	}

	errSigner := func(msg []byte) ([]byte, error) {
		return nil, errors.New("Error")
	}

	msg := signedGossipMessage("testChannelID", GossipMessage_EMPTY, &GossipMessage_DataMsg{
		DataMsg: &DataMessage{},
	})

	signedMsg, _ := msg.Sign(idSigner)

	// Since checking the identity signer, signature will be same as the payload
	assert.Equal(t, signedMsg.Payload, signedMsg.Signature)

	env, err := msg.Sign(errSigner)
	assert.Error(t, err)
	assert.Nil(t, env)
}

func TestEnvelope_NoopSign(t *testing.T) {
	channelID := "testChannelID"
	msg := signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataMsg{
		DataMsg: &DataMessage{},
	})

	signedMsg, err := msg.NoopSign()

	// Since checking the identity signer, signature will be same as the payload
	assert.Nil(t, signedMsg.Signature)
	assert.NoError(t, err)
}

func TestSignedGossipMessage_Verify(t *testing.T) {
	channelID := "testChannelID"
	peerID := []byte("peer")
	msg := signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataMsg{
		DataMsg: &DataMessage{},
	})

	assert.True(t, msg.IsSigned())

	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return nil
	}

	res := msg.Verify(peerID, verifier)
	assert.Nil(t, res)

	msg = signedGossipMessage(channelID, GossipMessage_EMPTY, &GossipMessage_DataMsg{
		DataMsg: &DataMessage{},
	})

	env := msg.Envelope
	msg.Envelope = nil
	res = msg.Verify(peerID, verifier)
	assert.Error(t, res)

	msg.Envelope = env
	payload := msg.Envelope.Payload
	msg.Envelope.Payload = nil
	res = msg.Verify(peerID, verifier)
	assert.Error(t, res)

	msg.Envelope.Payload = payload
	sig := msg.Signature
	msg.Signature = nil
	res = msg.Verify(peerID, verifier)
	assert.Error(t, res)
	msg.Signature = sig

	errVerifier := func(peerIdentity []byte, signature, message []byte) error {
		return errors.New("Test")
	}

	res = msg.Verify(peerID, errVerifier)
	assert.Error(t, res)
}

func TestEnvelope(t *testing.T) {
	dataMsg := &GossipMessage{
		Content: dataMessage(1, []byte("data")),
	}
	bytes, err := proto.Marshal(dataMsg)
	assert.NoError(t, err)

	env := envelopes()[0]
	env.Payload = bytes

	msg, err := env.ToGossipMessage()
	assert.NoError(t, err)
	assert.NotNil(t, msg)

	assert.True(t, msg.IsDataMsg())
}

func TestEnvelope_SignSecret(t *testing.T) {
	dataMsg := &GossipMessage{
		Content: dataMessage(1, []byte("data")),
	}
	bytes, err := proto.Marshal(dataMsg)
	assert.NoError(t, err)

	env := envelopes()[0]
	env.Payload = bytes
	env.SecretEnvelope = nil

	env.SignSecret(func(message []byte) ([]byte, error) {
		return message, nil
	}, &Secret{
		Content: &Secret_InternalEndpoint{
			InternalEndpoint: "localhost:5050",
		},
	})

	assert.NotNil(t, env.SecretEnvelope)
	assert.Equal(t, env.SecretEnvelope.InternalEndpoint(), "localhost:5050")
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

func leadershipMessage(incNum uint64, seqNum uint64, pkid []byte) *GossipMessage_LeadershipMsg {
	return &GossipMessage_LeadershipMsg{
		LeadershipMsg: &LeadershipMessage{
			PkiId:         pkid,
			IsDeclaration: false,
			Timestamp: &PeerTime{
				IncNum: incNum,
				SeqNum: seqNum,
			},
		},
	}
}

func stateInfoMessage(incNum uint64, seqNum uint64, pkid []byte, mac []byte) *GossipMessage_StateInfo {
	return &GossipMessage_StateInfo{
		StateInfo: &StateInfo{
			Timestamp: &PeerTime{
				IncNum: incNum,
				SeqNum: seqNum,
			},
			PkiId:       pkid,
			Channel_MAC: mac,
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

func signedGossipMessage(channelID string, tag GossipMessage_Tag, content isGossipMessage_Content) *SignedGossipMessage {
	return &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte(channelID),
			Tag:     tag,
			Nonce:   0,
			Content: content,
		},
		Envelope: envelopes()[0],
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

func TestToStringMembershipRequest(t *testing.T) {
	gossipMessage := &GossipMessage{
		Nonce:   5,
		Channel: []byte("A"),
		Tag:     0,
		Content: &GossipMessage_DataMsg{
			DataMsg: &DataMessage{
				Payload: &Payload{
					SeqNum: 3,
					Data:   []byte{2, 2, 2, 2, 2},
				},
			},
		},
	}
	nn, _ := gossipMessage.NoopSign()
	sMsg := &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Tag:     GossipMessage_EMPTY,
			Nonce:   5,
			Channel: []byte("A"),
			Content: &GossipMessage_DataMsg{
				DataMsg: &DataMessage{
					Payload: &Payload{
						SeqNum: 3,
						Data:   []byte{2, 2, 2, 2, 2},
					},
				},
			},
		},
		Envelope: &Envelope{
			Payload:   nn.Envelope.Payload,
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
	}
	mr := &MembershipRequest{
		SelfInformation: sMsg.Envelope,
		Known:           [][]byte{},
	}

	output := "Membership Request with self information of GossipMessage: Channel: A, nonce: 5, tag: UNDEFINED Block message: {Data: 5 bytes, seq: 3}, Envelope: 18 bytes, Signature: 3 bytes Secret payload: 6 bytes, Secret Signature: 3 bytes "
	assert.Equal(t, output, mr.toString())

	mr1 := &MembershipRequest{
		SelfInformation: &Envelope{
			Payload:   []byte{1, 2, 3},
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
		Known: [][]byte{},
	}
	assert.Equal(t, "", mr1.toString())

	mr2 := &MembershipRequest{
		SelfInformation: nil,
		Known:           [][]byte{},
	}

	assert.Equal(t, "", mr2.toString())
}

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
	assert.Equal(t, output, sipr.toString())
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
	assert.Equal(t, output, si.toString())

}

func TestToStringDataDigest(t *testing.T) {
	dig1 := &DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_BLOCK_MSG,
	}
	output1 := "data_dig: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	assert.Equal(t, output1, dig1.toString())
	dig2 := &DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_IDENTITY_MSG,
	}
	output2 := "data_dig: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	assert.Equal(t, output2, dig2.toString())
}

func TestToStringDataRequest(t *testing.T) {
	dataReq1 := &DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_BLOCK_MSG,
	}
	output1 := "data request: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	assert.Equal(t, output1, dataReq1.toString())
	dataReq2 := &DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: PullMsgType_IDENTITY_MSG,
	}
	output2 := "data request: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	assert.Equal(t, output2, dataReq2.toString())
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
	assert.Equal(t, output, lm.toString())
}
