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
)

func (s *SBFT) makeCheckpoint() *Checkpoint {
	sig := s.sys.Sign(s.cur.subject.Digest)
	c := &Checkpoint{
		Seq:       s.cur.subject.Seq.Seq,
		Digest:    s.cur.subject.Digest,
		Signature: sig,
	}
	return c
}

func (s *SBFT) sendCheckpoint() {
	s.broadcast(&Msg{&Msg_Checkpoint{s.makeCheckpoint()}})
}

func (s *SBFT) handleCheckpoint(c *Checkpoint, src uint64) {
	if s.cur.checkpointDone {
		return
	}

	if c.Seq < s.cur.subject.Seq.Seq {
		// old message
		return
	}

	err := s.checkBytesSig(c.Digest, src, c.Signature)
	if err != nil {
		log.Infof("replica %d: checkpoint signature invalid for %d from %d", s.id, c.Seq, src)
		return
	}

	// TODO should we always accept checkpoints?
	if c.Seq != s.cur.subject.Seq.Seq {
		log.Infof("replica %d: checkpoint does not match expected subject %v, got %v", s.id, &s.cur.subject, c)
		return
	}
	if _, ok := s.cur.checkpoint[src]; ok {
		log.Infof("replica %d: duplicate checkpoint for %d from %d", s.id, c.Seq, src)
	}
	s.cur.checkpoint[src] = c

	max := "_"
	sums := make(map[string][]uint64)
	for csrc, c := range s.cur.checkpoint {
		sum := fmt.Sprintf("%x", c.Digest)
		sums[sum] = append(sums[sum], csrc)

		if len(sums[sum]) >= s.oneCorrectQuorum() {
			max = sum
		}
	}

	replicas, ok := sums[max]
	if !ok {
		return
	}

	// got a weak checkpoint

	cpset := make(map[uint64][]byte)
	for _, r := range replicas {
		cp := s.cur.checkpoint[r]
		cpset[r] = cp.Signature
	}

	c = s.cur.checkpoint[replicas[0]]

	if !reflect.DeepEqual(c.Digest, s.cur.subject.Digest) {
		log.Warningf("replica %d: weak checkpoint %x does not match our state %x --- primary %d of view %d is probably Byzantine, sending view change",
			s.id, c.Digest, s.cur.subject.Digest, s.primaryID(), s.view)
		s.sendViewChange()
		return
	}

	// ignore null requests
	batch := *s.cur.preprep.Batch
	batch.Signatures = cpset
	s.deliverBatch(&batch, s.cur.committers)
	log.Infof("replica %d: request %s %s delivered on %d (completed common case)", s.id, s.cur.subject.Seq, hash2str(s.cur.subject.Digest), s.id)
	s.maybeSendNextBatch()
	s.processBacklog()
}
