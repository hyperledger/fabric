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

import "github.com/golang/protobuf/proto"

// Connection is an event from system to notify a new connection with
// replica.
// On connection, we send our latest (weak) checkpoint, and we expect
// to receive one from replica.
func (s *SBFT) Connection(replica uint64) {
	batch := *s.sys.LastBatch()
	batch.Payloads = nil // don't send the big payload
	s.sys.Send(&Msg{&Msg_Hello{&batch}}, replica)

	// A reconnecting replica can play forward its blockchain to
	// the batch listed in the hello message.  However, the
	// currently in-flight batch will not be reflected in the
	// Hello message, nor will all messages be present to actually
	// commit the in-flight batch at the reconnecting replica.
	//
	// Therefore we also send the most recent (pre)prepare,
	// commit, checkpoint so that the reconnecting replica can
	// catch up on the in-flight batch.
	//
	// TODO We need to communicate the latest view to the
	// connecting replica.  The new view message is not signed, so
	// we cannot send that message.  The worst corner case is
	// connecting right after a new-view message was received, and
	// its xset batch is in-flight.

	batchheader := &BatchHeader{}
	err := proto.Unmarshal(batch.Header, batchheader)
	if err != nil {
		panic(err)
	}

	if s.cur.subject.Seq.Seq > batchheader.Seq {
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

func (s *SBFT) handleHello(h *Batch, src uint64) {
	s.replicaState[src].hello = h
}
