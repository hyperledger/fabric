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

func (s *SBFT) testBacklog(m *Msg, src uint64) bool {
	if len(s.backLog[src]) > 0 {
		return true
	}

	return s.testBacklog2(m, src)
}

func (s *SBFT) testBacklog2(m *Msg, src uint64) bool {
	record := func(seq uint64) bool {
		if seq > s.cur.subject.Seq.Seq {
			return true
		}
		return false
	}

	if pp := m.GetPreprepare(); pp != nil && !s.cur.executed {
		return true
	} else if p := m.GetPrepare(); p != nil {
		return record(p.Seq.Seq)
	} else if c := m.GetCommit(); c != nil {
		return record(c.Seq.Seq)
	} else if cs := m.GetCheckpoint(); cs != nil {
		c := &Checkpoint{}
		return record(c.Seq)
	}
	return false
}

func (s *SBFT) recordBacklogMsg(m *Msg, src uint64) {
	if src == s.id {
		panic("should never have to backlog my own message")
	}
	// TODO prevent DoS by limiting the number of messages per replica
	s.backLog[src] = append(s.backLog[src], m)
}

func (s *SBFT) processBacklog() {
	processed := true

	for processed {
		processed = false
		notReady := uint64(0)
		for src, _ := range s.backLog {
			for len(s.backLog[src]) > 0 {
				m, rest := s.backLog[src][0], s.backLog[src][1:]
				if s.testBacklog2(m, src) {
					notReady++
					break
				}
				s.backLog[src] = rest

				log.Debugf("processing stored message from %d: %s", src, m)

				s.handleQueueableMessage(m, src)
				processed = true
			}
		}

		// all minus us
		if notReady >= s.config.N-1 {
			// This is a problem - we consider all other replicas
			// too far ahead for us.  We need to do a state transfer
			// to get out of this rut.
			for src := range s.backLog {
				delete(s.backLog, src)
			}
			// TODO trigger state transfer
		}
	}
}
