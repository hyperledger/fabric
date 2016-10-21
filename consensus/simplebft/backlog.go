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
	// TODO
	//
	// Prevent DoS by limiting the number of messages per replica.
	//
	// If the backlog limit is exceeded, discard all messages with
	// Seq before the replica's hello message (we can, because we
	// can play forward to this batch via state transfer).  If
	// there is no hello message, we must be really slow or the
	// replica must be byzantine.  In this case we probably should
	// re-establish the connection.
	//
	// After the connection has been re-established, we will
	// receive a hello, and the following messages will trigger
	// the pruning of old messages.  If this pruning lead us not
	// to make progress, the backlog processing algorithm as lined
	// out below will take care of starting a state transfer,
	// using the hello message we received on reconnect.
	s.backLog[src] = append(s.backLog[src], m)
}

func (s *SBFT) processBacklog() {
	processed := true
	notReady := uint64(0)

	for processed {
		processed = false
		for src := range s.backLog {
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
	// If a noFaultyQuorum (-1, because we're not faulty, just
	// were disconnected) is backlogged, we know that we need to
	// perform a state transfer.  Of course, f of these might be
	// byzantine, and the remaining f that are not backlogged will
	// allow us to get unstuck.  To check against that, we need to
	// only consider backlogged replicas of which we have a hello
	// message that talks about a future Seq.
	//
	// We need to pick the highest Seq of all the hello messages
	// we received, perform a state transfer to that Batch, and
	// discard all backlogged messages that refer to a lower Seq.
	//
	// Do we need to detect that a connection is stuck and we
	// should reconnect?
}
