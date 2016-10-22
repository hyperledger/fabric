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

import "time"

func (s *SBFT) sendViewChange() {
	s.seq.View = s.nextView()
	s.cur.timeout.Cancel()
	s.activeView = false
	for r, vs := range s.viewchange {
		if vs.vc.View < s.seq.View {
			delete(s.viewchange, r)
		}
	}
	log.Noticef("sending viewchange for view %d", s.seq.View)

	var q, p []*Subject
	if s.cur.sentCommit {
		p = append(p, &s.cur.subject)
	}
	if s.cur.preprep != nil {
		q = append(q, &s.cur.subject)
	}

	vc := &ViewChange{
		View:     s.seq.View,
		Qset:     q,
		Pset:     p,
		Executed: s.seq.Seq,
	}
	svc := s.sign(vc)
	s.viewChangeTimer.Cancel()
	s.viewChangeTimer = s.sys.Timer(s.viewChangeTimeout, func() {
		s.viewChangeTimeout *= 2
		log.Notice("view change timed out, sending next")
		s.sendViewChange()
	})
	s.broadcast(&Msg{&Msg_ViewChange{svc}})

	s.processNewView()
}

func (s *SBFT) cancelViewChangeTimer() {
	s.viewChangeTimer.Cancel()
	s.viewChangeTimeout = time.Duration(s.config.RequestTimeoutNsec) * 2
}

func (s *SBFT) handleViewChange(svc *Signed, src uint64) {
	vc := &ViewChange{}
	err := s.checkSig(svc, src, vc)
	if err != nil {
		log.Noticef("invalid viewchange: %s", err)
		return
	}
	if vc.View < s.seq.View {
		log.Debugf("old view change from %s for view %d, we are in view %d", src, vc.View, s.seq.View)
		return
	}
	if ovc, ok := s.viewchange[src]; ok && vc.View <= ovc.vc.View {
		log.Noticef("duplicate view change for %d from %d", vc.View, src)
		return
	}

	log.Infof("viewchange from %d for view %d", src, vc.View)
	s.viewchange[src] = &viewChangeInfo{svc: svc, vc: vc}

	if len(s.viewchange) == s.oneCorrectQuorum() {
		min := vc.View
		for _, vc := range s.viewchange {
			if vc.vc.View < min {
				min = vc.vc.View
			}
		}
		// catch up to the minimum view
		if s.seq.View < min {
			log.Notice("we are behind on view change, resending for newer view")
			s.seq.View = min - 1
			s.sendViewChange()
			return
		}
	}

	if s.isPrimary() {
		s.maybeSendNewView()
	}
}
