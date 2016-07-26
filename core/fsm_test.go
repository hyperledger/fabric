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
)

func simulateConn(tb testing.TB) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	err := peerConn.FSM.Event("HELLO")
	if err != nil {
		tb.Error(err)
	}

	err = peerConn.FSM.Event("DISCONNECT")
	if err != nil {
		tb.Error(err)
	}

}

func TestFSM_PeerConnection(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	err := peerConn.FSM.Event("HELLO")
	if err != nil {
		t.Error(err)
	}
	if peerConn.FSM.Current() != "established" {
		t.Error("Expected to be in establised state")
	}

	err = peerConn.FSM.Event("DISCONNECT")
	if err != nil {
		t.Error(err)
	}
}

func TestFSM_PeerConnection2(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	err := peerConn.FSM.Event("HELLO")
	if err != nil {
		t.Error(err)
	}
	if peerConn.FSM.Current() != "established" {
		t.Error("Expected to be in establised state")
	}

	err = peerConn.FSM.Event("DISCONNECT")
	if err != nil {
		t.Error(err)
	}
}

func TestFSM_PeerConnection_BadState_1(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	// Try to move from created state
	err := peerConn.FSM.Event("GET_PEERS")
	if err == nil {
		t.Error("Expected bad state message")
	}

	err = peerConn.FSM.Event("PING")
	if err == nil {
		t.Error("Expected bad state message")
	}

	err = peerConn.FSM.Event("DISCONNECT")
	if err != nil {
		t.Error(err)
	}

}

func TestFSM_PeerConnection_BadState_2(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	// Try to move from created state
	err := peerConn.FSM.Event("GET_PEERS")
	if err == nil {
		t.Error("Expected bad state message")
	}

	err = peerConn.FSM.Event("PING")
	if err == nil {
		t.Error("Expected bad state message")
	}
}

func TestFSM_PeerConnection_BadEvent(t *testing.T) {
	peerConn := NewPeerConnectionFSM("10.10.10.10:30303")

	// Try to move from created state
	err := peerConn.FSM.Event("UNDEFINED_EVENT")
	if err == nil {
		t.Error("Expected bad event message")
	} else {
		// Make sure expected error type
		if _, ok := err.(*fsm.UnknownEventError); !ok {
			t.Error("expected only 'fsm.UnknownEventError'")
		}
		t.Logf("Received expected error: %s", err)
	}

}

func Benchmark_FSM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		simulateConn(b)
	}
}
