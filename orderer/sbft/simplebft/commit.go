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

import "reflect"

func (s *SBFT) maybeSendCommit() {
	if s.cur.prepared || len(s.cur.prep) < s.commonCaseQuorum()-1 {
		return
	}
	s.sendCommit()
	s.processBacklog()
}

func (s *SBFT) sendCommit() {
	s.cur.prepared = true
	c := s.cur.subject
	s.sys.Persist(s.chainId, prepared, &c)
	s.broadcast(&Msg{&Msg_Commit{&c}})
}

func (s *SBFT) handleCommit(c *Subject, src uint64) {
	if c.Seq.Seq < s.cur.subject.Seq.Seq {
		// old message
		return
	}

	if !reflect.DeepEqual(c, &s.cur.subject) {
		log.Warningf("replica %d: commit does not match expected subject %v %x, got %v %x",
			s.id, s.cur.subject.Seq, s.cur.subject.Digest, c.Seq, c.Digest)
		return
	}
	if _, ok := s.cur.commit[src]; ok {
		log.Infof("replica %d: duplicate commit for %v from %d", s.id, *c.Seq, src)
		return
	}
	s.cur.commit[src] = c
	s.cancelViewChangeTimer()

	//maybe mark as comitted
	if s.cur.committed || len(s.cur.commit) < s.commonCaseQuorum() {
		return
	}
	s.cur.committed = true
	log.Noticef("replica %d: executing %v %x", s.id, s.cur.subject.Seq, s.cur.subject.Digest)

	s.sys.Persist(s.chainId, committed, &s.cur.subject)

	s.sendCheckpoint()
	s.processBacklog()
}
