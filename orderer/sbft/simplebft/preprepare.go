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
	"bytes"
	"time"
)

func (s *SBFT) sendPreprepare(batch []*Request) {
	seq := s.nextSeq()

	data := make([][]byte, len(batch))
	for i, req := range batch {
		data[i] = req.Payload
	}

	lasthash := hash(s.sys.LastBatch().Header)

	m := &Preprepare{
		Seq:   &seq,
		Batch: s.makeBatch(seq.Seq, lasthash, data),
	}

	s.sys.Persist("preprepare", m)
	s.broadcast(&Msg{&Msg_Preprepare{m}})
	s.handleCheckedPreprepare(m)
}

func (s *SBFT) handlePreprepare(pp *Preprepare, src uint64) {
	if src == s.id {
		log.Infof("replica %d: ignoring preprepare from self: %d", s.id, src)
		return
	}
	if src != s.primaryID() {
		log.Infof("replica %d: preprepare from non-primary %d", s.id, src)
		return
	}
	nextSeq := s.nextSeq()
	if *pp.Seq != nextSeq {
		log.Infof("replica %d: preprepare does not match expected %v, got %v", s.id, nextSeq, *pp.Seq)
		return
	}
	if s.cur.subject.Seq.Seq == pp.Seq.Seq {
		log.Infof("replica %d: duplicate preprepare for %v", s.id, *pp.Seq)
		return
	}
	if pp.Batch == nil {
		log.Infof("replica %d: preprepare without batch", s.id)
		return
	}

	batchheader, err := s.checkBatch(pp.Batch, true, false)
	if err != nil || batchheader.Seq != pp.Seq.Seq {
		log.Infof("replica %d: preprepare %v batch head inconsistent from %d: %s", s.id, pp.Seq, src, err)
		return
	}

	prevhash := s.sys.LastBatch().Hash()
	if !bytes.Equal(batchheader.PrevHash, prevhash) {
		log.Infof("replica %d: preprepare batch prev hash does not match expected %s, got %s", s.id, hash2str(batchheader.PrevHash), hash2str(prevhash))
		return
	}

	s.handleCheckedPreprepare(pp)
}

func (s *SBFT) acceptPreprepare(pp *Preprepare) {
	sub := Subject{Seq: pp.Seq, Digest: pp.Batch.Hash()}

	log.Infof("replica %d: accepting preprepare for %v, %x", s.id, sub.Seq, sub.Digest)
	s.sys.Persist("preprepare", pp)

	s.cur = reqInfo{
		subject:    sub,
		timeout:    s.sys.Timer(time.Duration(s.config.RequestTimeoutNsec)*time.Nanosecond, s.requestTimeout),
		preprep:    pp,
		prep:       make(map[uint64]*Subject),
		commit:     make(map[uint64]*Subject),
		checkpoint: make(map[uint64]*Checkpoint),
	}
}

func (s *SBFT) handleCheckedPreprepare(pp *Preprepare) {
	s.acceptPreprepare(pp)
	if !s.isPrimary() {
		s.sendPrepare()
		s.processBacklog()
	}

	s.maybeSendCommit()
}

////////////////////////////////////////////////

func (s *SBFT) requestTimeout() {
	log.Infof("replica %d: request timed out: %s", s.id, s.cur.subject.Seq)
	s.sendViewChange()
}
