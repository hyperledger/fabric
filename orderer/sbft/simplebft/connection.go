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

// Connection is an event from system to notify a new connection with
// replica.
// On connection, we send our latest (weak) checkpoint, and we expect
// to receive one from replica.
func (s *SBFT) Connection(replica uint64) {
	batch := *s.sys.LastBatch()
	batch.Payloads = nil // don't send the big payload
	hello := &Hello{Batch: &batch}
	if s.isPrimary() && s.activeView && s.lastNewViewSent != nil {
		hello.NewView = s.lastNewViewSent
	}
	s.sys.Send(&Msg{&Msg_Hello{hello}}, replica)

	svc := s.replicaState[s.id].signedViewchange
	if svc != nil {
		s.sys.Send(&Msg{&Msg_ViewChange{svc}}, replica)
	}

	// A reconnecting replica can play forward its blockchain to
	// the batch listed in the hello message.  However, the
	// currently in-flight batch will not be reflected in the
	// Hello message, nor will all messages be present to actually
	// commit the in-flight batch at the reconnecting replica.
	//
	// Therefore we also send the most recent (pre)prepare,
	// commit, checkpoint so that the reconnecting replica can
	// catch up on the in-flight batch.

	batchheader, err := s.checkBatch(&batch, false, true)
	if err != nil {
		panic(err)
	}

	if s.cur.subject.Seq.Seq > batchheader.Seq && s.activeView {
		if s.isPrimary() {
			s.sys.Send(&Msg{&Msg_Preprepare{s.cur.preprep}}, replica)
		} else {
			s.sys.Send(&Msg{&Msg_Prepare{&s.cur.subject}}, replica)
		}
		if s.cur.sentCommit {
			s.sys.Send(&Msg{&Msg_Commit{&s.cur.subject}}, replica)
		}
		if s.cur.executed {
			s.sys.Send(&Msg{&Msg_Checkpoint{s.makeCheckpoint()}}, replica)
		}
	}
}

func (s *SBFT) handleHello(h *Hello, src uint64) {
	bh, err := s.checkBatch(h.Batch, false, true)
	if err != nil {
		log.Warningf("replica %d: invalid hello batch from %d: %s", s.id, src, err)
		return
	}

	if s.sys.LastBatch().DecodeHeader().Seq < bh.Seq {
		s.deliverBatch(h.Batch)
	}

	if h.NewView != nil {
		if s.primaryIDView(h.NewView.View) != src {
			log.Warningf("replica %d: invalid hello with new view from non-primary %d", s.id, src)
			return
		}

		vcs, err := s.checkNewViewSignatures(h.NewView)
		if err != nil {
			log.Warningf("replica %d: invalid hello new view from %d: %s", s.id, src, err)
			return
		}

		_, _, ok := s.makeXset(vcs)
		if !ok {
			log.Warningf("replica %d: invalid hello new view xset from %d", s.id, src)
			return
		}

		if s.view <= h.NewView.View {
			s.view = h.NewView.View
			s.activeView = true
		}

		s.maybeDeliverUsingXset(h.NewView)
	}

	s.replicaState[src].hello = h

	s.discardBacklog(src)
	s.processBacklog()
}
