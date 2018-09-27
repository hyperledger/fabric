/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func TestShuffle(t *testing.T) {
	endorsers := make(Endorsers, 1000)
	for i := 0; i < len(endorsers); i++ {
		endorsers[i] = &Peer{
			StateInfoMessage: stateInfoWithHeight(uint64(i)),
		}
	}

	isHeightAscending := func(endorsers Endorsers) bool {
		for i := 0; i < len(endorsers)-1; i++ {
			currHeight := endorsers[i].StateInfoMessage.GetStateInfo().Properties.LedgerHeight
			nextHeight := endorsers[i+1].StateInfoMessage.GetStateInfo().Properties.LedgerHeight
			if currHeight > nextHeight {
				return false
			}
		}
		return true
	}

	assert.True(t, isHeightAscending(endorsers))
	assert.False(t, isHeightAscending(endorsers.Shuffle()))
}

func TestExclusionAndPriority(t *testing.T) {
	newPeer := func(i int) *Peer {
		si := stateInfoWithHeight(uint64(i))
		am, _ := aliveMessage(i).ToGossipMessage()
		return &Peer{
			StateInfoMessage: si,
			AliveMessage:     am,
		}
	}

	excludeFirst := selectionFunc(func(p Peer) bool {
		return p.AliveMessage.GetAliveMsg().Timestamp.SeqNum == uint64(1)
	})

	givenPeers := Endorsers{newPeer(3), newPeer(5), newPeer(1), newPeer(4), newPeer(2), newPeer(3)}
	assert.Equal(t, []int{5, 4, 3, 3, 2}, heights(givenPeers.Filter(excludeFirst).Sort(PrioritiesByHeight)))
}

func TestExcludeEndpoints(t *testing.T) {
	secretEndpoint := &gossip.Secret{
		Content: &gossip.Secret_InternalEndpoint{
			InternalEndpoint: "s2",
		},
	}
	secret, _ := proto.Marshal(secretEndpoint)
	am1 := aliveMessage(1)
	am2 := aliveMessage(2)
	am2.SecretEnvelope = &gossip.SecretEnvelope{
		Payload: secret,
	}
	am3 := aliveMessage(3)
	g1, _ := am1.ToGossipMessage()
	g2, _ := am2.ToGossipMessage()
	g3, _ := am3.ToGossipMessage()
	p1 := Peer{
		AliveMessage: g1,
	}
	p2 := Peer{
		AliveMessage: g2,
	}
	p3 := Peer{
		AliveMessage: g3,
	}

	s := ExcludeHosts("p1", "s2")
	assert.True(t, s.Exclude(p1))
	assert.True(t, s.Exclude(p2))
	assert.False(t, s.Exclude(p3))

	s = NoExclusion
	assert.False(t, s.Exclude(p1))
	assert.False(t, s.Exclude(p2))
	assert.False(t, s.Exclude(p3))
}

func TestNoPriorities(t *testing.T) {
	s1 := stateInfoWithHeight(100)
	s2 := stateInfoWithHeight(200)
	p1 := Peer{
		StateInfoMessage: s1,
	}
	p2 := Peer{
		StateInfoMessage: s2,
	}
	assert.Equal(t, Priority(0), NoPriorities.Compare(p1, p2))
}

func TestPrioritiesByHeight(t *testing.T) {
	tests := []struct {
		name        string
		expected    Priority
		leftHeight  uint64
		rightHeight uint64
	}{
		{
			name:        "Same heights",
			expected:    0,
			leftHeight:  100,
			rightHeight: 100,
		},
		{
			name:        "Right height bigger",
			expected:    -1,
			leftHeight:  100,
			rightHeight: 200,
		},
		{
			name:        "Left height bigger",
			expected:    1,
			leftHeight:  200,
			rightHeight: 100,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			s1 := stateInfoWithHeight(test.leftHeight)
			s2 := stateInfoWithHeight(test.rightHeight)
			p1 := Peer{
				StateInfoMessage: s1,
			}
			p2 := Peer{
				StateInfoMessage: s2,
			}
			p := PrioritiesByHeight.Compare(p1, p2)
			assert.Equal(t, test.expected, p)
		})
	}

}

func stateInfoWithHeight(h uint64) *gossip.SignedGossipMessage {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{
				Properties: &gossip.Properties{
					LedgerHeight: h,
				},
				Timestamp: &gossip.PeerTime{},
			},
		},
	}
	sMsg, _ := g.NoopSign()
	return sMsg
}

func heights(endorsers Endorsers) []int {
	var res []int
	for _, e := range endorsers {
		res = append(res, int(e.StateInfoMessage.GetStateInfo().Properties.LedgerHeight))
	}
	return res
}
