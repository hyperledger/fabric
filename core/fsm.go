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

import "github.com/looplab/fsm"

// PeerConnectionFSM example FSM for demonstration purposes.
type PeerConnectionFSM struct {
	To  string
	FSM *fsm.FSM
}

// NewPeerConnectionFSM creates and returns a PeerConnectionFSM
func NewPeerConnectionFSM(to string) *PeerConnectionFSM {
	d := &PeerConnectionFSM{
		To: to,
	}

	d.FSM = fsm.NewFSM(
		"created",
		fsm.Events{
			{Name: "HELLO", Src: []string{"created"}, Dst: "established"},
			{Name: "GET_PEERS", Src: []string{"established"}, Dst: "established"},
			{Name: "PEERS", Src: []string{"established"}, Dst: "established"},
			{Name: "PING", Src: []string{"established"}, Dst: "established"},
			{Name: "DISCONNECT", Src: []string{"created", "established"}, Dst: "closed"},
		},
		fsm.Callbacks{
			"enter_state":  func(e *fsm.Event) { d.enterState(e) },
			"before_HELLO": func(e *fsm.Event) { d.beforeHello(e) },
			"after_HELLO":  func(e *fsm.Event) { d.afterHello(e) },
			"before_PING":  func(e *fsm.Event) { d.beforePing(e) },
			"after_PING":   func(e *fsm.Event) { d.afterPing(e) },
		},
	)

	return d
}

func (d *PeerConnectionFSM) enterState(e *fsm.Event) {
	log.Debugf("The bi-directional stream to %s is %s, from event %s\n", d.To, e.Dst, e.Event)
}

func (d *PeerConnectionFSM) beforeHello(e *fsm.Event) {
	log.Debugf("Before reception of %s, dest is %s, current is %s", e.Event, e.Dst, d.FSM.Current())
}

func (d *PeerConnectionFSM) afterHello(e *fsm.Event) {
	log.Debugf("After reception of %s, dest is %s, current is %s", e.Event, e.Dst, d.FSM.Current())
}

func (d *PeerConnectionFSM) afterPing(e *fsm.Event) {
	log.Debugf("After reception of %s, dest is %s, current is %s", e.Event, e.Dst, d.FSM.Current())
}

func (d *PeerConnectionFSM) beforePing(e *fsm.Event) {
	log.Debugf("Before %s, dest is %s, current is %s", e.Event, e.Dst, d.FSM.Current())
}
