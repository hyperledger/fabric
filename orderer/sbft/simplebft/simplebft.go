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
	"math"
	"reflect"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/op/go-logging"
)

const preprepared string = "preprepared"
const prepared string = "prepared"
const committed string = "committed"
const viewchange string = "viewchange"

// Receiver defines the API that is exposed by SBFT to the system.
type Receiver interface {
	Receive(msg *Msg, src uint64)
	Request(req []byte)
	Connection(replica uint64)
	GetChainId() string
}

// System defines the API that needs to be provided for SBFT.
type System interface {
	Send(chainId string, msg *Msg, dest uint64)
	Timer(d time.Duration, f func()) Canceller
	Deliver(chainId string, batch *Batch, committers []filter.Committer)
	AddReceiver(chainId string, receiver Receiver)
	Persist(chainId string, key string, data proto.Message)
	Restore(chainId string, key string, out proto.Message) bool
	LastBatch(chainId string) *Batch
	Sign(data []byte) []byte
	CheckSig(data []byte, src uint64, sig []byte) error
	Reconnect(chainId string, replica uint64)
	Validate(chainID string, req *Request) ([][]*Request, [][]filter.Committer, bool)
	Cut(chainID string) ([]*Request, []filter.Committer)
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
	batches           [][]*Request
	batchTimer        Canceller
	cur               reqInfo
	activeView        bool
	lastNewViewSent   *NewView
	viewChangeTimeout time.Duration
	viewChangeTimer   Canceller
	replicaState      []replicaInfo
	pending           map[string]*Request
	validated         map[string]bool
	chainId           string
	primarycommitters [][]filter.Committer
}

type reqInfo struct {
	subject        Subject
	timeout        Canceller
	preprep        *Preprepare
	prep           map[uint64]*Subject
	commit         map[uint64]*Subject
	checkpoint     map[uint64]*Checkpoint
	prepared       bool
	committed      bool
	checkpointDone bool
	committers     []filter.Committer
}

type replicaInfo struct {
	backLog          []*Msg
	hello            *Hello
	signedViewchange *Signed
	viewchange       *ViewChange
}

var log = logging.MustGetLogger("sbft")

type dummyCanceller struct{}

func (d dummyCanceller) Cancel() {}

// New creates a new SBFT instance.
func New(id uint64, chainID string, config *Config, sys System) (*SBFT, error) {
	if config.F*3+1 > config.N {
		return nil, fmt.Errorf("invalid combination of N (%d) and F (%d)", config.N, config.F)
	}

	s := &SBFT{
		config:            *config,
		sys:               sys,
		id:                id,
		chainId:           chainID,
		viewChangeTimer:   dummyCanceller{},
		replicaState:      make([]replicaInfo, config.N),
		pending:           make(map[string]*Request),
		validated:         make(map[string]bool),
		batches:           make([][]*Request, 0, 3),
		primarycommitters: make([][]filter.Committer, 0),
	}
	s.sys.AddReceiver(chainID, s)

	s.view = 0
	s.cur.subject.Seq = &SeqView{}
	s.cur.prepared = true
	s.cur.committed = true
	s.cur.checkpointDone = true
	s.cur.timeout = dummyCanceller{}
	s.activeView = true

	svc := &Signed{}
	if s.sys.Restore(s.chainId, viewchange, svc) {
		vc := &ViewChange{}
		err := proto.Unmarshal(svc.Data, vc)
		if err != nil {
			return nil, err
		}
		fmt.Println(fmt.Sprintf("rep %d VIEW %d   %d", s.id, s.view, vc.View))
		s.view = vc.View
		s.replicaState[s.id].signedViewchange = svc
		s.activeView = false
	}

	pp := &Preprepare{}
	if s.sys.Restore(s.chainId, preprepared, pp) && pp.Seq.View >= s.view {
		s.view = pp.Seq.View
		s.activeView = true
		if pp.Seq.Seq > s.seq() {
			// TODO double add to BC?
			_, committers := s.getCommittersFromBatch(pp.Batch)
			s.acceptPreprepare(pp, committers)
		}
	}
	c := &Subject{}
	if s.sys.Restore(s.chainId, prepared, c) && reflect.DeepEqual(c, &s.cur.subject) && c.Seq.View >= s.view {
		s.cur.prepared = true
	}
	ex := &Subject{}
	if s.sys.Restore(s.chainId, committed, ex) && reflect.DeepEqual(c, &s.cur.subject) && ex.Seq.View >= s.view {
		s.cur.committed = true
	}

	s.cancelViewChangeTimer()
	return s, nil
}

////////////////////////////////////////////////

func (s *SBFT) GetChainId() string {
	return s.chainId
}

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
	return s.sys.LastBatch(s.chainId).DecodeHeader().Seq
}

func (s *SBFT) nextSeq() SeqView {
	return SeqView{Seq: s.seq() + 1, View: s.view}
}

func (s *SBFT) nextView() uint64 {
	return s.view + 1
}

func (s *SBFT) commonCaseQuorum() int {
	//When N=3F+1 this should be 2F+1 (N-F)
	//More generally, we need every two common case quorums of size X to intersect in at least F+1 orderers,
	//hence 2X>=N+F+1, or X is:
	return int(math.Ceil(float64(s.config.N+s.config.F+1) / float64(2)))
}

func (s *SBFT) viewChangeQuorum() int {
	//When N=3F+1 this should be 2F+1 (N-F)
	//More generally, we need every view change quorum to intersect with every common case quorum at least F+1 orderers, hence:
	//Y >= N-X+F+1
	return int(s.config.N+s.config.F+1) - s.commonCaseQuorum()
}

func (s *SBFT) oneCorrectQuorum() int {
	return int(s.config.F + 1)
}

func (s *SBFT) broadcast(m *Msg) {
	for i := uint64(0); i < s.config.N; i++ {
		s.sys.Send(s.chainId, m, i)
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

func (s *SBFT) deliverBatch(batch *Batch, committers []filter.Committer) {
	if committers == nil {
		log.Warningf("replica %d: commiter is nil", s.id)
		panic("Committer is nil.")
	}
	s.cur.checkpointDone = true
	s.cur.timeout.Cancel()
	// s.primarycommitters[0]
	s.sys.Deliver(s.chainId, batch, committers)
	// s.primarycommitters = s.primarycommitters[1:]

	for _, req := range batch.Payloads {
		key := hash2str(hash(req))
		log.Infof("replica %d: attempting to remove %x from pending", s.id, key)
		delete(s.pending, key)
		delete(s.validated, key)
	}
}
