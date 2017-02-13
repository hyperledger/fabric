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

package discovery

import (
	"testing"

	"github.com/hyperledger/fabric/gossip/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func TestMembershipStore(t *testing.T) {
	membershipStore := make(membershipStore, 0)

	id1 := common.PKIidType("id1")
	id2 := common.PKIidType("id2")

	msg1 := &message{}
	msg2 := &message{SignedGossipMessage: &proto.SignedGossipMessage{}}

	// Test initially created store is empty
	assert.Nil(t, membershipStore.msgByID(id1))
	assert.Len(t, membershipStore, 0)
	// Test put works as expected
	membershipStore.Put(id1, msg1)
	assert.NotNil(t, membershipStore.msgByID(id1))
	// Test msgByID returns the right instance stored
	membershipStore.Put(id2, msg2)
	assert.Equal(t, msg1, membershipStore.msgByID(id1))
	assert.NotEqual(t, msg2, membershipStore.msgByID(id1))
	// Test capacity grows
	assert.Len(t, membershipStore, 2)
	// Test remove works
	membershipStore.Remove(id1)
	assert.Nil(t, membershipStore.msgByID(id1))
	assert.Len(t, membershipStore, 1)
	// Test returned instance is not a copy
	msg3 := &message{GossipMessage: &proto.GossipMessage{}}
	msg3Clone := &message{GossipMessage: &proto.GossipMessage{}}
	id3 := common.PKIidType("id3")
	membershipStore.Put(id3, msg3)
	assert.Equal(t, msg3Clone, msg3)
	membershipStore.msgByID(id3).Channel = []byte{0, 1, 2, 3}
	assert.NotEqual(t, msg3Clone, msg3)
}

func TestToSlice(t *testing.T) {
	membershipStore := make(membershipStore, 0)
	id1 := common.PKIidType("id1")
	id2 := common.PKIidType("id2")
	id3 := common.PKIidType("id3")
	id4 := common.PKIidType("id4")

	msg1 := &message{}
	msg2 := &message{SignedGossipMessage: &proto.SignedGossipMessage{}}
	msg3 := &message{GossipMessage: &proto.GossipMessage{}}
	msg4 := &message{GossipMessage: &proto.GossipMessage{}, SignedGossipMessage: &proto.SignedGossipMessage{}}

	membershipStore.Put(id1, msg1)
	membershipStore.Put(id2, msg2)
	membershipStore.Put(id3, msg3)
	membershipStore.Put(id4, msg4)

	assert.Len(t, membershipStore.ToSlice(), 4)

	existsInSlice := func(slice []*message, msg *message) bool {
		for _, m := range slice {
			if assert.ObjectsAreEqual(m, msg) {
				return true
			}
		}
		return false
	}

	expectedMsgs := []*message{msg1, msg2, msg3, msg4}
	for _, msg := range membershipStore.ToSlice() {
		assert.True(t, existsInSlice(expectedMsgs, msg))
	}

}
