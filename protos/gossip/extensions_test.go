/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToStringMembershipResponse(t *testing.T) {
	mr := &MembershipResponse{
		Alive: []*Envelope{{}},
		Dead:  []*Envelope{{}},
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
