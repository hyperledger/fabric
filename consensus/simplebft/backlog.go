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
	if len(s.replicaState[src].backLog) > 0 {
		return true
	}

	return s.testBacklog2(m, src)
}

func (s *SBFT) testBacklog2(m *Msg, src uint64) bool {
	record := func(seq *SeqView) bool {
		if !s.activeView {
			return true
		}
		if seq.Seq > s.cur.subject.Seq.Seq || seq.View > s.seq.View {
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
		panic("should never have to backlog my own message")
	}
	// TODO
	//
	// Prevent DoS by limiting the number of messages per replica.
	//
	// If the backlog limit is exceeded, re-establish the
	// connection.
	//
	// After the connection has been re-established, we will
	// receive a hello, which will advance our state and discard
	// old messages.
	s.replicaState[src].backLog = append(s.replicaState[src].backLog, m)
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
				if s.testBacklog2(m, src) {
					notReady++
					break
				}
				state.backLog = rest

				log.Debugf("processing stored message from %d: %s", src, m)

				s.handleQueueableMessage(m, src)
				processed = true
			}
		}
	}

	// TODO
	//
	// Detect when we need to reconsider our options.
	//
	// We arrived here because either all is fine, we're with the
	// pack.  Or we have messages in the backlog because we're
	// connected asymmetrically, and a close replica already
	// started talking about the next batch while we're still
	// waiting for rounds to arrive for our current batch.  That's
	// still fine.
	//
	// We might also be here because we lost connectivity, and we
	// either missed some messages, or our connection is bad and
	// we should reconnect to get a working connection going
	// again.
	//
	// Do we need to detect that a connection is stuck and we
	// should reconnect?
}
