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

import "fmt"

const maxBacklogSeq = 4
const msgPerSeq = 3 // (pre)prepare, commit, checkpoint

func (s *SBFT) testBacklogMessage(m *Msg, src uint64) bool {
	record := func(seq *SeqView) bool {
		if !s.activeView {
			return true
		}
		if seq.Seq > s.cur.subject.Seq.Seq || seq.View > s.view {
			return true
		}
		return false
	}

	if pp := m.GetPreprepare(); pp != nil {
		return record(pp.Seq) && !s.cur.checkpointDone
	} else if p := m.GetPrepare(); p != nil {
		return record(p.Seq)
	} else if c := m.GetCommit(); c != nil {
		return record(c.Seq)
	} else if cs := m.GetCheckpoint(); cs != nil {
		c := &Checkpoint{}
		return record(&SeqView{Seq: c.Seq})
	}
	return false
}

func (s *SBFT) recordBacklogMsg(m *Msg, src uint64) {
	if src == s.id {
		panic(fmt.Sprintf("should never have to backlog my own message (replica ID: %d)", src))
	}

	s.replicaState[src].backLog = append(s.replicaState[src].backLog, m)

	if len(s.replicaState[src].backLog) > maxBacklogSeq*msgPerSeq {
		s.discardBacklog(src)
		s.sys.Reconnect(src)
	}
}

func (s *SBFT) discardBacklog(src uint64) {
	s.replicaState[src].backLog = nil
}

func (s *SBFT) processBacklog() {
	processed := true
	notReady := uint64(0)

	for processed {
		processed = false
		for src := range s.replicaState {
			state := &s.replicaState[src]
			src := uint64(src)

			for len(state.backLog) > 0 {
				m, rest := state.backLog[0], state.backLog[1:]
				if s.testBacklogMessage(m, src) {
					notReady++
					break
				}
				state.backLog = rest

				log.Debugf("replica %d: processing stored message from %d: %s", s.id, src, m)

				s.handleQueueableMessage(m, src)
				processed = true
			}
		}
	}
}
