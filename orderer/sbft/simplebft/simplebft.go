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

package simplebft

import (
	"fmt"
	"reflect"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

// Receiver defines the API that is exposed by SBFT to the system.
type Receiver interface {
	Receive(msg *Msg, src uint64)
	Request(req []byte)
	Connection(replica uint64)
}

// System defines the API that needs to be provided for SBFT.
type System interface {
	Send(msg *Msg, dest uint64)
	Timer(d time.Duration, f func()) Canceller
	Deliver(batch *Batch)
	SetReceiver(receiver Receiver)
	Persist(key string, data proto.Message)
	Restore(key string, out proto.Message) bool
	LastBatch() *Batch
	Sign(data []byte) []byte
	CheckSig(data []byte, src uint64, sig []byte) error
	Reconnect(replica uint64)
}

// Canceller allows cancelling of a scheduled timer event.
type Canceller interface {
	Cancel()
}

// SBFT is a simplified PBFT implementation.
type SBFT struct {
	sys System

	config            Config
	id                uint64
	view              uint64
	batch             []*Request
	batchTimer        Canceller
	cur               reqInfo
	activeView        bool
	lastNewViewSent   *NewView
	viewChangeTimeout time.Duration
	viewChangeTimer   Canceller
	replicaState      []replicaInfo
	pending           map[string]*Request
}

type reqInfo struct {
	subject        Subject
	timeout        Canceller
	preprep        *Preprepare
	prep           map[uint64]*Subject
	commit         map[uint64]*Subject
	sentCommit     bool
	executed       bool
	checkpoint     map[uint64]*Checkpoint
	checkpointDone bool
}

type replicaInfo struct {
	backLog          []*Msg
	hello            *Hello
	signedViewchange *Signed
	viewchange       *ViewChange
	newview          *NewView
}

var log = logging.MustGetLogger("sbft")

type dummyCanceller struct{}

func (d dummyCanceller) Cancel() {}

// New creates a new SBFT instance.
func New(id uint64, config *Config, sys System) (*SBFT, error) {
	if config.F*3+1 > config.N {
		return nil, fmt.Errorf("invalid combination of N and F")
	}

	s := &SBFT{
		config:          *config,
		sys:             sys,
		id:              id,
		viewChangeTimer: dummyCanceller{},
		replicaState:    make([]replicaInfo, config.N),
		pending:         make(map[string]*Request),
	}
	s.sys.SetReceiver(s)

	s.view = 0
	s.cur.subject.Seq = &SeqView{}
	s.cur.sentCommit = true
	s.cur.executed = true
	s.cur.checkpointDone = true
	s.cur.timeout = dummyCanceller{}

	pp := &Preprepare{}
	if s.sys.Restore("preprepare", pp) {
		s.view = pp.Seq.View
		if pp.Seq.Seq > s.seq() {
			s.acceptPreprepare(pp)
		}
	}
	c := &Subject{}
	if s.sys.Restore("commit", c) && reflect.DeepEqual(c, &s.cur.subject) {
		s.cur.sentCommit = true
	}
	ex := &Subject{}
	if s.sys.Restore("execute", ex) && reflect.DeepEqual(c, &s.cur.subject) {
		s.cur.executed = true
	}

	if s.seq() == 0 {
		s.activeView = true
	}

	s.cancelViewChangeTimer()
	return s, nil
}

////////////////////////////////////////////////

func (s *SBFT) primaryIDView(v uint64) uint64 {
	return v % s.config.N
}

func (s *SBFT) primaryID() uint64 {
	return s.primaryIDView(s.view)
}

func (s *SBFT) isPrimary() bool {
	return s.primaryID() == s.id
}

func (s *SBFT) seq() uint64 {
	return s.sys.LastBatch().DecodeHeader().Seq
}

func (s *SBFT) nextSeq() SeqView {
	return SeqView{Seq: s.seq() + 1, View: s.view}
}

func (s *SBFT) nextView() uint64 {
	return s.view + 1
}

func (s *SBFT) noFaultyQuorum() int {
	return int(s.config.N - s.config.F)
}

func (s *SBFT) oneCorrectQuorum() int {
	return int(s.config.F + 1)
}

func (s *SBFT) broadcast(m *Msg) {
	for i := uint64(0); i < s.config.N; i++ {
		s.sys.Send(m, i)
	}
}

////////////////////////////////////////////////

// Receive is the ingress method for SBFT messages.
func (s *SBFT) Receive(m *Msg, src uint64) {
	log.Debugf("replica %d: received message from %d: %s", s.id, src, m)

	if h := m.GetHello(); h != nil {
		s.handleHello(h, src)
		return
	} else if req := m.GetRequest(); req != nil {
		s.handleRequest(req, src)
		return
	} else if vs := m.GetViewChange(); vs != nil {
		s.handleViewChange(vs, src)
		return
	} else if nv := m.GetNewView(); nv != nil {
		s.handleNewView(nv, src)
		return
	}

	if s.testBacklogMessage(m, src) {
		log.Debugf("replica %d: message for future seq, storing for later", s.id)
		s.recordBacklogMsg(m, src)
		return
	}

	s.handleQueueableMessage(m, src)
}

func (s *SBFT) handleQueueableMessage(m *Msg, src uint64) {
	if pp := m.GetPreprepare(); pp != nil {
		s.handlePreprepare(pp, src)
		return
	} else if p := m.GetPrepare(); p != nil {
		s.handlePrepare(p, src)
		return
	} else if c := m.GetCommit(); c != nil {
		s.handleCommit(c, src)
		return
	} else if c := m.GetCheckpoint(); c != nil {
		s.handleCheckpoint(c, src)
		return
	}

	log.Warningf("replica %d: received invalid message from %d", s.id, src)
}

func (s *SBFT) deliverBatch(batch *Batch) {
	s.sys.Deliver(batch)

	for _, req := range batch.Payloads {
		key := hash2str(hash(req))
		log.Infof("replica %d: attempting to remove %x from pending", s.id, key)
		delete(s.pending, key)
	}
}
