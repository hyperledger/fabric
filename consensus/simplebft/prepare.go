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

func (s *SBFT) sendPrepare() {
	p := s.cur.subject
	s.broadcast(&Msg{&Msg_Prepare{&p}})
}

func (s *SBFT) handlePrepare(p *Subject, src uint64) {
	if p.Seq.Seq < s.cur.subject.Seq.Seq {
		// old message
		return
	}

	if !reflect.DeepEqual(p, &s.cur.subject) {
		log.Infof("prepare does not match expected subject %v, got %v", &s.cur.subject, p)
		return
	}
	if _, ok := s.cur.prep[src]; ok {
		log.Infof("duplicate prepare for %v from %d", *p.Seq, src)
		return
	}
	s.cur.prep[src] = p
	s.maybeSendCommit()
}
