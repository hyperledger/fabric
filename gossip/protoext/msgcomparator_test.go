/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

func TestAliveMessageNoActionTaken(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(1)

	sMsg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
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
	}

	sMsg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{
						Endpoint: "localhost",
						Metadata: []byte{1, 2, 3, 4, 5},
						PkiId:    []byte{15},
					},
					Timestamp: &gossip.PeerTime{
						IncNum: 2,
						SeqNum: 2,
					},
					Identity: []byte("peerID1"),
				},
			},
		},
	}

	require.Equal(t, comparator(sMsg1, sMsg2), common.MessageNoAction)
}

func TestStateInfoMessageNoActionTaken(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(1)

	// msg1 and msg2 have same channel mac, while different pkid, while
	// msg and msg3 same pkid and different channel mac

	sMsg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(1, 1, []byte{17}, []byte{17, 13}),
		},
	}
	sMsg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(1, 1, []byte{13}, []byte{17, 13}),
		},
	}

	// We only should compare comparable messages, e.g. message from same peer
	// In any other cases no invalidation should be taken.
	require.Equal(t, comparator(sMsg1, sMsg2), common.MessageNoAction)
}

func TestStateInfoMessagesInvalidation(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(1)

	sMsg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(1, 1, []byte{17}, []byte{17}),
		},
	}
	sMsg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(1, 1, []byte{17}, []byte{17}),
		},
	}
	sMsg3 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(1, 2, []byte{17}, []byte{17}),
		},
	}
	sMsg4 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: stateInfoMessage(2, 1, []byte{17}, []byte{17}),
		},
	}

	require.Equal(t, comparator(sMsg1, sMsg2), common.MessageInvalidated)

	require.Equal(t, comparator(sMsg1, sMsg3), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg3, sMsg1), common.MessageInvalidates)

	require.Equal(t, comparator(sMsg1, sMsg4), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg4, sMsg1), common.MessageInvalidates)

	require.Equal(t, comparator(sMsg3, sMsg4), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg4, sMsg3), common.MessageInvalidates)
}

func TestAliveMessageInvalidation(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(1)

	sMsg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
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
	}

	sMsg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{
						Endpoint: "localhost",
						Metadata: []byte{1, 2, 3, 4, 5},
						PkiId:    []byte{17},
					},
					Timestamp: &gossip.PeerTime{
						IncNum: 2,
						SeqNum: 2,
					},
					Identity: []byte("peerID1"),
				},
			},
		},
	}

	sMsg3 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{
						Endpoint: "localhost",
						Metadata: []byte{1, 2, 3, 4, 5},
						PkiId:    []byte{17},
					},
					Timestamp: &gossip.PeerTime{
						IncNum: 1,
						SeqNum: 2,
					},
					Identity: []byte("peerID1"),
				},
			},
		},
	}

	require.Equal(t, comparator(sMsg1, sMsg2), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg2, sMsg1), common.MessageInvalidates)
	require.Equal(t, comparator(sMsg1, sMsg3), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg3, sMsg1), common.MessageInvalidates)
}

func TestDataMessageInvalidation(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(5)

	data := []byte{1, 1, 1}
	sMsg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: dataMessage(1, data),
		},
	}
	sMsg1Clone := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: dataMessage(1, data),
		},
	}
	sMsg3 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: dataMessage(2, data),
		},
	}
	sMsg4 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: dataMessage(7, data),
		},
	}

	require.Equal(t, comparator(sMsg1, sMsg1Clone), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg1, sMsg3), common.MessageNoAction)
	require.Equal(t, comparator(sMsg1, sMsg4), common.MessageInvalidated)
	require.Equal(t, comparator(sMsg4, sMsg1), common.MessageInvalidates)
}

func TestIdentityMessagesInvalidation(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(5)

	msg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_PeerIdentity{
				PeerIdentity: &gossip.PeerIdentity{
					PkiId:    []byte{17},
					Cert:     []byte{1, 2, 3, 4},
					Metadata: nil,
				},
			},
		},
	}

	msg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_PeerIdentity{
				PeerIdentity: &gossip.PeerIdentity{
					PkiId:    []byte{17},
					Cert:     []byte{1, 2, 3, 4},
					Metadata: nil,
				},
			},
		},
	}

	msg3 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: &gossip.GossipMessage_PeerIdentity{
				PeerIdentity: &gossip.PeerIdentity{
					PkiId:    []byte{11},
					Cert:     []byte{11, 21, 31, 41},
					Metadata: nil,
				},
			},
		},
	}

	require.Equal(t, comparator(msg1, msg2), common.MessageInvalidated)
	require.Equal(t, comparator(msg1, msg3), common.MessageNoAction)
}

func TestLeadershipMessagesNoAction(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(5)

	msg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: leadershipMessage(1, 1, []byte{17}),
		},
	}
	msg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: leadershipMessage(1, 1, []byte{11}),
		},
	}

	// If message with different pkid's no action should be taken
	require.Equal(t, comparator(msg1, msg2), common.MessageNoAction)
}

func TestLeadershipMessagesInvalidation(t *testing.T) {
	comparator := protoext.NewGossipMessageComparator(5)

	pkiID := []byte{17}
	msg1 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: leadershipMessage(1, 1, pkiID),
		},
	}
	msg2 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: leadershipMessage(1, 2, pkiID),
		},
	}
	msg3 := &protoext.SignedGossipMessage{
		GossipMessage: &gossip.GossipMessage{
			Channel: []byte("testChannel"),
			Tag:     gossip.GossipMessage_EMPTY,
			Content: leadershipMessage(2, 1, pkiID),
		},
	}

	// If message with different pkid's no action should be taken
	require.Equal(t, comparator(msg1, msg2), common.MessageInvalidated)
	require.Equal(t, comparator(msg2, msg1), common.MessageInvalidates)
	require.Equal(t, comparator(msg1, msg3), common.MessageInvalidated)
	require.Equal(t, comparator(msg3, msg1), common.MessageInvalidates)
	require.Equal(t, comparator(msg2, msg3), common.MessageInvalidated)
	require.Equal(t, comparator(msg3, msg2), common.MessageInvalidates)
}

func stateInfoMessage(incNum uint64, seqNum uint64, pkid []byte, mac []byte) *gossip.GossipMessage_StateInfo {
	return &gossip.GossipMessage_StateInfo{
		StateInfo: &gossip.StateInfo{
			Timestamp: &gossip.PeerTime{
				IncNum: incNum,
				SeqNum: seqNum,
			},
			PkiId:       pkid,
			Channel_MAC: mac,
		},
	}
}

func dataMessage(seqNum uint64, data []byte) *gossip.GossipMessage_DataMsg {
	return &gossip.GossipMessage_DataMsg{
		DataMsg: &gossip.DataMessage{
			Payload: &gossip.Payload{
				SeqNum: seqNum,
				Data:   data,
			},
		},
	}
}

func leadershipMessage(incNum uint64, seqNum uint64, pkid []byte) *gossip.GossipMessage_LeadershipMsg {
	return &gossip.GossipMessage_LeadershipMsg{
		LeadershipMsg: &gossip.LeadershipMessage{
			PkiId:         pkid,
			IsDeclaration: false,
			Timestamp: &gossip.PeerTime{
				IncNum: incNum,
				SeqNum: seqNum,
			},
		},
	}
}
