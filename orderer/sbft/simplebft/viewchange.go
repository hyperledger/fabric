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
	s.view = s.nextView()
	s.cur.timeout.Cancel()
	s.activeView = false
	for src := range s.replicaState {
		state := &s.replicaState[src]
		if state.viewchange != nil && state.viewchange.View < s.view {
			state.viewchange = nil
		}
	}
	log.Noticef("replica %d: sending viewchange for view %d", s.id, s.view)

	var q, p []*Subject
	if s.cur.prepared {
		p = append(p, &s.cur.subject)
	}
	if s.cur.preprep != nil {
		q = append(q, &s.cur.subject)
	}

	// TODO fix batches synchronization as we send no payload here
	checkpoint := *s.sys.LastBatch(s.chainId)
	checkpoint.Payloads = nil // don't send the big payload

	vc := &ViewChange{
		View:       s.view,
		Qset:       q,
		Pset:       p,
		Checkpoint: &checkpoint,
	}
	svc := s.sign(vc)
	s.viewChangeTimer.Cancel()
	s.cur.timeout.Cancel()

	s.sys.Persist(s.chainId, viewchange, svc)
	s.broadcast(&Msg{&Msg_ViewChange{svc}})
}

func (s *SBFT) cancelViewChangeTimer() {
	s.viewChangeTimer.Cancel()
	s.viewChangeTimeout = time.Duration(s.config.RequestTimeoutNsec) * 2
}

func (s *SBFT) handleViewChange(svc *Signed, src uint64) {
	vc := &ViewChange{}
	err := s.checkSig(svc, src, vc)
	if err == nil {
		_, err = s.checkBatch(vc.Checkpoint, false, true)
	}
	if err != nil {
		log.Noticef("replica %d: invalid viewchange: %s", s.id, err)
		return
	}
	if vc.View < s.view {
		log.Debugf("replica %d: old view change from %d for view %d, we are in view %d", s.id, src, vc.View, s.view)
		return
	}
	if ovc := s.replicaState[src].viewchange; ovc != nil && vc.View <= ovc.View {
		log.Noticef("replica %d: duplicate view change for %d from %d", s.id, vc.View, src)
		return
	}

	log.Infof("replica %d: viewchange from %d: %v", s.id, src, vc)
	s.replicaState[src].viewchange = vc
	s.replicaState[src].signedViewchange = svc

	min := vc.View

	//amplify current primary abdication
	if s.view == min-1 && s.primaryID() == src {
		s.sendViewChange()
		return
	}

	quorum := 0
	for _, state := range s.replicaState {
		if state.viewchange != nil {
			quorum++
			if state.viewchange.View < min {
				min = state.viewchange.View
			}
		}
	}

	if quorum == s.oneCorrectQuorum() {
		// catch up to the minimum view
		if s.view < min {
			log.Noticef("replica %d: we are behind on view change, resending for newer view", s.id)
			s.view = min - 1
			s.sendViewChange()
			return
		}
	}

	if quorum == s.viewChangeQuorum() {
		log.Noticef("replica %d: received view change quorum, starting view change timer", s.id)
		s.viewChangeTimer = s.sys.Timer(s.viewChangeTimeout, func() {
			s.viewChangeTimeout *= 2
			log.Noticef("replica %d: view change timed out, sending next", s.id)
			s.sendViewChange()
		})
	}

	if s.isPrimary() {
		s.maybeSendNewView()
	}
}
