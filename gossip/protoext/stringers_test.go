/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

func TestMembershipResponseToString(t *testing.T) {
	mr := &gossip.MembershipResponse{
		Alive: envelopes(),
		Dead:  envelopes(),
	}
	output := "MembershipResponse with Alive: 1, Dead: 1"
	require.Equal(t, output, protoext.MembershipResponseToString(mr))
}

func TestMembershipRequestToString(t *testing.T) {
	gossipMessage := &gossip.GossipMessage{
		Nonce:   5,
		Channel: []byte("A"),
		Tag:     0,
		Content: &gossip.GossipMessage_DataMsg{
			DataMsg: &gossip.DataMessage{
				Payload: &gossip.Payload{
					SeqNum: 3,
					Data:   []byte{2, 2, 2, 2, 2},
				},
			},
		},
	}
	nn, _ := protoext.NoopSign(gossipMessage)
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
			Payload:   nn.Envelope.Payload,
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &gossip.SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
	}
	mr := &gossip.MembershipRequest{
		SelfInformation: sMsg.Envelope,
		Known:           [][]byte{},
	}

	output := "Membership Request with self information of GossipMessage: Channel: A, nonce: 5, tag: UNDEFINED Block message: {Data: 5 bytes, seq: 3}, Envelope: 18 bytes, Signature: 3 bytes Secret payload: 6 bytes, Secret Signature: 3 bytes "
	require.Equal(t, output, protoext.MembershipRequestToString(mr))

	mr1 := &gossip.MembershipRequest{
		SelfInformation: &gossip.Envelope{
			Payload:   []byte{1, 2, 3},
			Signature: []byte{0, 1, 2},
			SecretEnvelope: &gossip.SecretEnvelope{
				Payload:   []byte{0, 1, 2, 3, 4, 5},
				Signature: []byte{0, 1, 2},
			},
		},
		Known: [][]byte{},
	}
	require.Equal(t, "", protoext.MembershipRequestToString(mr1))

	mr2 := &gossip.MembershipRequest{
		SelfInformation: nil,
		Known:           [][]byte{},
	}

	require.Equal(t, "", protoext.MembershipRequestToString(mr2))
}

func TestToStringMember(t *testing.T) {
	member := &gossip.Member{
		Endpoint: "localhost",
		Metadata: []byte{1, 2, 3, 4, 5},
		PkiId:    []byte{15},
	}
	output := "Membership: Endpoint:localhost PKI-id:0f"
	require.Equal(t, output, protoext.MemberToString(member))
}

func TestToStringAliveMessage(t *testing.T) {
	am1 := &gossip.AliveMessage{
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
	}
	output1 := "Alive Message:Membership: Endpoint:localhost PKI-id:11Identity:Timestamp:inc_num:1 seq_num:1 "
	require.Equal(t, output1, protoext.AliveMessageToString(am1))
	am2 := &gossip.AliveMessage{
		Membership: nil,
		Timestamp: &gossip.PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		Identity: []byte("peerID1"),
	}
	output2 := "nil Membership"
	require.Equal(t, output2, protoext.AliveMessageToString(am2))
}

func TestToStringStateInfoPullRequest(t *testing.T) {
	// Create State info pull request
	sipr := &gossip.StateInfoPullRequest{
		Channel_MAC: []byte{17},
	}

	output := "state_info_pull_req: Channel MAC:11"
	require.Equal(t, output, protoext.StateInfoPullRequestToString(sipr))
}

func TestToStringStateInfo(t *testing.T) {
	si := &gossip.StateInfo{
		Timestamp: &gossip.PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		PkiId:       []byte{17},
		Channel_MAC: []byte{17},
		Properties:  nil,
	}
	output := "state_info_message: Timestamp:inc_num:1 seq_num:1 PKI-id:11 channel MAC:11 properties:<nil>"
	require.Equal(t, output, protoext.StateInfoToString(si))
}

func TestToStringDataDigest(t *testing.T) {
	dig1 := &gossip.DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: gossip.PullMsgType_BLOCK_MSG,
	}
	output1 := "data_dig: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	require.Equal(t, output1, protoext.DataDigestToString(dig1))
	dig2 := &gossip.DataDigest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: gossip.PullMsgType_IDENTITY_MSG,
	}
	output2 := "data_dig: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	require.Equal(t, output2, protoext.DataDigestToString(dig2))
}

func TestToStringDataRequest(t *testing.T) {
	dataReq1 := &gossip.DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: gossip.PullMsgType_BLOCK_MSG,
	}
	output1 := "data request: nonce: 0 , Msg_type: BLOCK_MSG, digests: [msg1 msg2 msg3]"
	require.Equal(t, output1, protoext.DataRequestToString(dataReq1))
	dataReq2 := &gossip.DataRequest{
		Nonce:   0,
		Digests: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		MsgType: gossip.PullMsgType_IDENTITY_MSG,
	}
	output2 := "data request: nonce: 0 , Msg_type: IDENTITY_MSG, digests: [6d736731 6d736732 6d736733]"
	require.Equal(t, output2, protoext.DataRequestToString(dataReq2))
}

func TestToStringLeadershipMessage(t *testing.T) {
	lm := &gossip.LeadershipMessage{
		Timestamp: &gossip.PeerTime{
			IncNum: 1,
			SeqNum: 1,
		},
		PkiId:         []byte{17},
		IsDeclaration: true,
	}
	output := "Leadership Message: PKI-id:11 Timestamp:inc_num:1 seq_num:1 Is Declaration true"
	require.Equal(t, output, protoext.LeadershipMessageToString(lm))
}

func TestRemotePvtDataResponseToString(t *testing.T) {
	res := &gossip.RemotePvtDataResponse{
		Elements: []*gossip.PvtDataElement{
			{Digest: &gossip.PvtDataDigest{TxId: "tx-id"}, Payload: [][]byte{[]byte("abcde")}},
		},
	}

	output := `[tx_id:"tx-id"  with 1 elements]`
	require.Equal(t, output, protoext.RemovePvtDataResponseToString(res))
}
