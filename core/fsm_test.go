/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package core

import (
	"testing"

	"github.com/looplab/fsm"
	"github.com/stretchr/testify/assert"
)

func simulateConn(tb testing.TB) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:7051")

	err := peerConn.FSM.Event("HELLO")
	assert.Nil(tb, err, "Error should have been nil")

	err = peerConn.FSM.Event("DISCONNECT")
	assert.Nil(tb, err, "Error should have been nil")
}

func TestFSM_PeerConnection(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:7051")
	assert.Equal(t, peerConn.FSM.Current(), "created", "Expected to be in created state, got: %s", peerConn.FSM.Current())

	err := peerConn.FSM.Event("HELLO")
	assert.Nil(t, err, "Error should have been nil")
	assert.Equal(t, peerConn.FSM.Current(), "established", "Expected to be in established state, got: %s", peerConn.FSM.Current())

	err = peerConn.FSM.Event("PING")
	assert.NotNil(t, err, "Expected bad state message")
	assert.Equal(t, peerConn.FSM.Current(), "established", "Expected to be in established state, got: %s", peerConn.FSM.Current())

	err = peerConn.FSM.Event("DISCONNECT")
	assert.Nil(t, err, "Error should have been nil")
	assert.Equal(t, peerConn.FSM.Current(), "closed", "Expected to be in closed state, got: %s", peerConn.FSM.Current())
}

func TestFSM_PeerConnection_BadState_1(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:7051")

	// Try to move from created state
	err := peerConn.FSM.Event("GET_PEERS")
	assert.NotNil(t, err, "Expected bad state message")

	err = peerConn.FSM.Event("PING")
	assert.NotNil(t, err, "Expected bad state message")

	err = peerConn.FSM.Event("DISCONNECT")
	assert.Nil(t, err, "Error should have been nil")
}

func TestFSM_PeerConnection_BadState_2(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:7051")

	// Try to move from created state
	err := peerConn.FSM.Event("GET_PEERS")
	assert.NotNil(t, err, "Expected bad state message")

	err = peerConn.FSM.Event("PING")
	assert.NotNil(t, err, "Expected bad state message")
}

func TestFSM_PeerConnection_BadEvent(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:7051")

	// Try to move from created state
	err := peerConn.FSM.Event("UNDEFINED_EVENT")
	assert.NotNil(t, err, "Expected bad state message")
	// Make sure expected error type
	_, ok := err.(*fsm.UnknownEventError)
	assert.True(t, ok, "expected only 'fsm.UnknownEventError'")
}

func Benchmark_FSM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		simulateConn(b)
	}
}
