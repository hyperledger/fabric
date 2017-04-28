/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gossip

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/stretchr/testify/assert"
)

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
}

func TestAliveMessageNoActionTaken(t *testing.T) {
	comparator := NewGossipMessageComparator(1)

	sMsg1 := signedGossipMessage("testChannel", &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{
			Membership: &Member{
				Endpoint: "localhost",
				Metadata: []byte{1, 2, 3, 4, 5},
				PkiId:    []byte{17},
			},
			Timestamp: &PeerTime{
				IncNumber: 1,
				SeqNum:    1,
			},
			Identity: []byte("peerID1"),
		},
	})

	sMsg2 := signedGossipMessage("testChannel", &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{
			Membership: &Member{
				Endpoint: "localhost",
				Metadata: []byte{1, 2, 3, 4, 5},
				PkiId:    []byte{15},
			},
			Timestamp: &PeerTime{
				IncNumber: 2,
				SeqNum:    2,
			},
			Identity: []byte("peerID1"),
		},
	})

	assert.Equal(t, comparator(sMsg1, sMsg2), common.MessageNoAction)
}

func TestStateInfoMessageNoActionTaken(t *testing.T) {
	comparator := NewGossipMessageComparator(1)

	// msg1 and msg2 have same channel mac, while different pkid, while
	// msg and msg3 same pkid and different channel mac

	sMsg1 := signedGossipMessage("testChannel", stateInfoMessage(1, 1, []byte{17}, []byte{17, 13}))
	sMsg2 := signedGossipMessage("testChannel", stateInfoMessage(1, 1, []byte{13}, []byte{17, 13}))

	// We only should compare comparable messages, e.g. message from same peer
	// In any other cases no invalidation should be taken.
	assert.Equal(t, comparator(sMsg1, sMsg2), common.MessageNoAction)
}

func TestStateInfoMessagesInvalidation(t *testing.T) {
	comparator := NewGossipMessageComparator(1)

	sMsg1 := signedGossipMessage("testChannel", stateInfoMessage(1, 1, []byte{17}, []byte{17}))
	sMsg2 := signedGossipMessage("testChannel", stateInfoMessage(1, 1, []byte{17}, []byte{17}))
	sMsg3 := signedGossipMessage("testChannel", stateInfoMessage(1, 2, []byte{17}, []byte{17}))
	sMsg4 := signedGossipMessage("testChannel", stateInfoMessage(2, 1, []byte{17}, []byte{17}))

	assert.Equal(t, comparator(sMsg1, sMsg2), common.MessageInvalidated)

	assert.Equal(t, comparator(sMsg1, sMsg3), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg3, sMsg1), common.MessageInvalidates)

	assert.Equal(t, comparator(sMsg1, sMsg4), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg4, sMsg1), common.MessageInvalidates)

	assert.Equal(t, comparator(sMsg3, sMsg4), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg4, sMsg3), common.MessageInvalidates)
}

func TestAliveMessageInvalidation(t *testing.T) {
	comparator := NewGossipMessageComparator(1)

	sMsg1 := signedGossipMessage("testChannel", &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{
			Membership: &Member{
				Endpoint: "localhost",
				Metadata: []byte{1, 2, 3, 4, 5},
				PkiId:    []byte{17},
			},
			Timestamp: &PeerTime{
				IncNumber: 1,
				SeqNum:    1,
			},
			Identity: []byte("peerID1"),
		},
	})

	sMsg2 := signedGossipMessage("testChannel", &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{
			Membership: &Member{
				Endpoint: "localhost",
				Metadata: []byte{1, 2, 3, 4, 5},
				PkiId:    []byte{17},
			},
			Timestamp: &PeerTime{
				IncNumber: 2,
				SeqNum:    2,
			},
			Identity: []byte("peerID1"),
		},
	})

	sMsg3 := signedGossipMessage("testChannel", &GossipMessage_AliveMsg{
		AliveMsg: &AliveMessage{
			Membership: &Member{
				Endpoint: "localhost",
				Metadata: []byte{1, 2, 3, 4, 5},
				PkiId:    []byte{17},
			},
			Timestamp: &PeerTime{
				IncNumber: 1,
				SeqNum:    2,
			},
			Identity: []byte("peerID1"),
		},
	})

	assert.Equal(t, comparator(sMsg1, sMsg2), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg2, sMsg1), common.MessageInvalidates)
	assert.Equal(t, comparator(sMsg1, sMsg3), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg3, sMsg1), common.MessageInvalidates)
}

func TestDataMessageInvalidation(t *testing.T) {
	comparator := NewGossipMessageComparator(5)

	data := []byte{1, 1, 1}
	sMsg1 := signedGossipMessage("testChannel", dataMessage(1, "hash", data))
	sMsg2 := signedGossipMessage("testChannel", dataMessage(1, "hash", data))
	sMsg3 := signedGossipMessage("testChannel", dataMessage(1, "newHash", data))
	sMsg4 := signedGossipMessage("testChannel", dataMessage(2, "newHash", data))
	sMsg5 := signedGossipMessage("testChannel", dataMessage(7, "newHash", data))

	assert.Equal(t, comparator(sMsg1, sMsg2), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg1, sMsg3), common.MessageNoAction)
	assert.Equal(t, comparator(sMsg1, sMsg4), common.MessageNoAction)
	assert.Equal(t, comparator(sMsg1, sMsg5), common.MessageInvalidated)
	assert.Equal(t, comparator(sMsg5, sMsg1), common.MessageInvalidates)
}

func TestIdentityMessagesInvalidation(t *testing.T) {
	comparator := NewGossipMessageComparator(5)

	msg1 := signedGossipMessage("testChannel", &GossipMessage_PeerIdentity{
		PeerIdentity: &PeerIdentity{
			PkiId:    []byte{17},
			Cert:     []byte{1, 2, 3, 4},
			Metadata: nil,
		},
	})

	msg2 := signedGossipMessage("testChannel", &GossipMessage_PeerIdentity{
		PeerIdentity: &PeerIdentity{
			PkiId:    []byte{17},
			Cert:     []byte{1, 2, 3, 4},
			Metadata: nil,
		},
	})

	msg3 := signedGossipMessage("testChannel", &GossipMessage_PeerIdentity{
		PeerIdentity: &PeerIdentity{
			PkiId:    []byte{11},
			Cert:     []byte{11, 21, 31, 41},
			Metadata: nil,
		},
	})

	assert.Equal(t, comparator(msg1, msg2), common.MessageInvalidated)
	assert.Equal(t, comparator(msg1, msg3), common.MessageNoAction)
}

func TestLeadershipMessagesNoAction(t *testing.T) {
	comparator := NewGossipMessageComparator(5)

	msg1 := signedGossipMessage("testChannel", leadershipMessage(1, 1, []byte{17}))
	msg2 := signedGossipMessage("testChannel", leadershipMessage(1, 1, []byte{11}))

	// If message with different pkid's no action should be taken
	assert.Equal(t, comparator(msg1, msg2), common.MessageNoAction)
}

func TestLeadershipMessagesInvalidation(t *testing.T) {
	comparator := NewGossipMessageComparator(5)

	pkiID := []byte{17}
	msg1 := signedGossipMessage("testChannel", leadershipMessage(1, 1, pkiID))
	msg2 := signedGossipMessage("testChannel", leadershipMessage(1, 2, pkiID))
	msg3 := signedGossipMessage("testChannel", leadershipMessage(2, 1, pkiID))

	// If message with different pkid's no action should be taken
	assert.Equal(t, comparator(msg1, msg2), common.MessageInvalidated)
	assert.Equal(t, comparator(msg2, msg1), common.MessageInvalidates)
	assert.Equal(t, comparator(msg1, msg3), common.MessageInvalidated)
	assert.Equal(t, comparator(msg3, msg1), common.MessageInvalidates)
	assert.Equal(t, comparator(msg2, msg3), common.MessageInvalidated)
	assert.Equal(t, comparator(msg3, msg2), common.MessageInvalidates)
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
				IncNumber: incNum,
				SeqNum:    seqNum,
			},
		},
	}
}

func stateInfoMessage(incNumber uint64, seqNum uint64, pkid []byte, mac []byte) *GossipMessage_StateInfo {
	return &GossipMessage_StateInfo{
		StateInfo: &StateInfo{
			Metadata: []byte{},
			Timestamp: &PeerTime{
				IncNumber: incNumber,
				SeqNum:    seqNum,
			},
			PkiId:      pkid,
			ChannelMAC: mac,
		},
	}
}

func dataMessage(seqNum uint64, hash string, data []byte) *GossipMessage_DataMsg {
	return &GossipMessage_DataMsg{
		DataMsg: &DataMessage{
			Payload: &Payload{
				SeqNum: seqNum,
				Hash:   hash,
				Data:   data,
			},
		},
	}
}

func signedGossipMessage(channelID string, content isGossipMessage_Content) *SignedGossipMessage {
	return &SignedGossipMessage{
		GossipMessage: &GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     GossipMessage_EMPTY,
			Nonce:   0,
			Content: content,
		},
		Envelope: envelopes()[0],
	}
}
