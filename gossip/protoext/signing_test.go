/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestToGossipMessageNilEnvelope(t *testing.T) {
	memReq := &gossip.MembershipRequest{}
	_, err := protoext.EnvelopeToGossipMessage(memReq.SelfInformation)
	require.EqualError(t, err, "nil envelope")
}

func TestToString(t *testing.T) {
	// Ensure we don't print the byte content when we
	// log messages.
	// Each payload or signature contains '2' so we would've logged
	// them if not for the overloading of the String() method in SignedGossipMessage

	// The following line proves that the envelopes constructed in this test
	// have "2" in them when they are printed
	require.Contains(t, fmt.Sprintf("%v", envelopes()[0]), "2")
	// and the following does the same for payloads:
	dMsg := &gossip.DataMessage{
		Payload: &gossip.Payload{
			SeqNum: 3,
			Data:   []byte{2, 2, 2, 2, 2},
		},
	}
	require.Contains(t, fmt.Sprintf("%v", dMsg), "2")

	// Now we construct all types of messages that have envelopes or payloads in them
	// and see that "2" is not outputted into their formatting even though it is found
	// as a sub-message of the outer message.

	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Channel: []byte("A"),
			Content: &gossip.GossipMessage_DataMsg{
				DataMsg: &gossip.DataMessage{
					Payload: &gossip.Payload{
						SeqNum: 3,
						Data:   []byte{2, 2, 2, 2, 2},
					},
				},
			},
		},
		Envelope: &gossip.Envelope{
			Payload:   []byte{0, 1, 2, 3, 4, 5, 6},
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &gossip.SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")
	sMsg.GetDataMsg().Payload = nil
	require.NotPanics(t, func() {
		_ = sMsg.String()
	})

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_DataUpdate{
				DataUpdate: &gossip.DataUpdate{
					Nonce:   11,
					MsgType: gossip.PullMsgType_BLOCK_MSG,
					Data:    envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_MemRes{
				MemRes: &gossip.MembershipResponse{
					Alive: envelopes(),
					Dead:  envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_StateSnapshot{
				StateSnapshot: &gossip.StateInfoSnapshot{
					Elements: envelopes(),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{
						Endpoint: "localhost",
						Metadata: []byte{1, 2, 3, 4, 5},
						PkiId:    []byte{17},
					},
					Timestamp: &gossip.PeerTime{
						IncNum: 1,
						SeqNum: 1,
					},
					Identity: []byte("peerID1"),
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_StateResponse{
				StateResponse: &gossip.RemoteStateResponse{
					Payloads: []*gossip.Payload{
						{Data: []byte{2, 2, 2}},
					},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_MemReq{
				MemReq: &gossip.MembershipRequest{
					SelfInformation: sMsg.Envelope,
					Known:           [][]byte{},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_StateInfoPullReq{
				StateInfoPullReq: &gossip.StateInfoPullRequest{
					Channel_MAC: []byte{17},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_StateInfo{
				StateInfo: &gossip.StateInfo{
					Channel_MAC: []byte{17},
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_DataDig{
				DataDig: &gossip.DataDigest{
					Nonce:   0,
					Digests: [][]byte{[]byte("msg1"), []byte("msg2")},
					MsgType: gossip.PullMsgType_BLOCK_MSG,
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.Contains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_DataReq{
				DataReq: &gossip.DataRequest{
					Nonce:   0,
					Digests: [][]byte{[]byte("msg1"), []byte("msg2")},
					MsgType: gossip.PullMsgType_BLOCK_MSG,
				},
			},
		},
		Envelope: envelopes()[0],
	}
	require.Contains(t, fmt.Sprintf("%v", sMsg), "2")

	sMsg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("A"),
			Tag:     gossip.GossipMessage_EMPTY,
			Nonce:   5,
			Content: &gossip.GossipMessage_LeadershipMsg{
				LeadershipMsg: &gossip.LeadershipMessage{
					Timestamp: &gossip.PeerTime{
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
	require.NotContains(t, fmt.Sprintf("%v", sMsg), "2")
}

func TestSignedGossipMessageSign(t *testing.T) {
	idSigner := func(msg []byte) ([]byte, error) {
		return msg, nil
	}

	errSigner := func(msg []byte) ([]byte, error) {
		return nil, errors.New("Error")
	}

	msg := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannelID"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_DataMsg{
				DataMsg: &gossip.DataMessage{},
			},
		},
	}
	signedMsg, _ := msg.Sign(idSigner)

	// Since checking the identity signer, signature will be same as the payload
	require.Equal(t, signedMsg.Payload, signedMsg.Signature)

	env, err := msg.Sign(errSigner)
	require.Error(t, err)
	require.Nil(t, env)
}

func TestEnvelope_NoopSign(t *testing.T) {
	msg := &gossip.GossipMessage{
		Channel: []byte("testChannelID"),
		Tag:     gossip.GossipMessage_EMPTY,
		Content: &gossip.GossipMessage_DataMsg{
			DataMsg: &gossip.DataMessage{},
		},
	}

	signedMsg, err := protoext.NoopSign(msg)

	// Since checking the identity signer, signature will be same as the payload
	require.Nil(t, signedMsg.Signature)
	require.NoError(t, err)
}

func TestSignedGossipMessage_Verify(t *testing.T) {
	channelID := "testChannelID"
	peerID := []byte("peer")

	msg := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte(channelID),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_DataMsg{
				DataMsg: &gossip.DataMessage{},
			},
		},
		Envelope: envelopes()[0],
	}
	require.True(t, msg.IsSigned())

	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return nil
	}
	res := msg.Verify(peerID, verifier)
	require.Nil(t, res)

	msg = &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte(channelID),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_DataMsg{
				DataMsg: &gossip.DataMessage{},
			},
		},
		Envelope: envelopes()[0],
	}

	env := msg.Envelope
	msg.Envelope = nil
	res = msg.Verify(peerID, verifier)
	require.Error(t, res)

	msg.Envelope = env
	payload := msg.Envelope.Payload
	msg.Envelope.Payload = nil
	res = msg.Verify(peerID, verifier)
	require.Error(t, res)

	msg.Envelope.Payload = payload
	sig := msg.Signature
	msg.Signature = nil
	res = msg.Verify(peerID, verifier)
	require.Error(t, res)
	msg.Signature = sig

	errVerifier := func(peerIdentity []byte, signature, message []byte) error {
		return errors.New("Test")
	}

	res = msg.Verify(peerID, errVerifier)
	require.Error(t, res)
}

func TestEnvelope(t *testing.T) {
	dataMsg := &gossip.GossipMessage{
		Content: dataMessage(1, []byte("data")),
	}
	bytes, err := proto.Marshal(dataMsg)
	require.NoError(t, err)

	env := envelopes()[0]
	env.Payload = bytes

	msg, err := protoext.EnvelopeToGossipMessage(env)
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.True(t, protoext.IsDataMsg(msg.GossipMessage))
}

func TestEnvelope_SignSecret(t *testing.T) {
	dataMsg := &gossip.GossipMessage{
		Content: dataMessage(1, []byte("data")),
	}
	bytes, err := proto.Marshal(dataMsg)
	require.NoError(t, err)

	env := envelopes()[0]
	env.Payload = bytes
	env.SecretEnvelope = nil

	protoext.SignSecret(env, func(message []byte) ([]byte, error) {
		return message, nil
	}, &gossip.Secret{
		Content: &gossip.Secret_InternalEndpoint{
			InternalEndpoint: "localhost:5050",
		},
	})

	require.NotNil(t, env.SecretEnvelope)
	require.Equal(t, protoext.InternalEndpoint(env.SecretEnvelope), "localhost:5050")
}

func TestInternalEndpoint(t *testing.T) {
	require.Empty(t, protoext.InternalEndpoint(nil))
	require.Empty(t, protoext.InternalEndpoint(&gossip.SecretEnvelope{
		Payload: []byte{1, 2, 3},
	}))
	require.Equal(t, "foo", protoext.InternalEndpoint(&gossip.SecretEnvelope{
		Payload: protoutil.MarshalOrPanic(
			&gossip.Secret{
				Content: &gossip.Secret_InternalEndpoint{
					InternalEndpoint: "foo",
				},
			}),
	}))
}

func envelopes() []*gossip.Envelope {
	return []*gossip.Envelope{
		{
			Payload:   []byte{2, 2, 2},
			Signature: []byte{2, 2, 2},
			SecretEnvelope: &gossip.SecretEnvelope{
				Payload:   []byte{2, 2, 2},
				Signature: []byte{2, 2, 2},
			},
		},
	}
}
