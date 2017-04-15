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
