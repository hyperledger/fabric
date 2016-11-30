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

import (
	"bytes"
	"fmt"
	"reflect"
)

func (s *SBFT) maybeSendNewView() {
	if s.lastNewViewSent != nil && s.lastNewViewSent.View == s.view {
		return
	}

	vset := make(map[uint64]*Signed)
	var vcs []*ViewChange

	for src, state := range s.replicaState {
		if state.viewchange != nil && state.viewchange.View == s.view {
			vset[uint64(src)] = state.signedViewchange
			vcs = append(vcs, state.viewchange)
		}
	}

	xset, _, ok := s.makeXset(vcs)
	if !ok {
		log.Debugf("replica %d: xset not yet sufficient", s.id)
		return
	}

	var batch *Batch
	if xset == nil {
		// no need for batch, it is contained in the vset
	} else if reflect.DeepEqual(s.cur.subject.Digest, xset.Digest) {
		batch = s.cur.preprep.Batch
	} else {
		log.Warningf("replica %d: forfeiting primary - do not have request in store for %d %x", s.id, xset.Seq.Seq, xset.Digest)
		xset = nil
	}

	nv := &NewView{
		View:  s.view,
		Vset:  vset,
		Xset:  xset,
		Batch: batch,
	}

	log.Noticef("replica %d: sending new view for %d", s.id, nv.View)
	s.lastNewViewSent = nv
	s.broadcast(&Msg{&Msg_NewView{nv}})
}

func (s *SBFT) checkNewViewSignatures(nv *NewView) ([]*ViewChange, error) {
	var vcs []*ViewChange
	for vcsrc, svc := range nv.Vset {
		vc := &ViewChange{}
		err := s.checkSig(svc, vcsrc, vc)
		if err == nil {
			_, err = s.checkBatch(vc.Checkpoint, false, true)
			if vc.View != nv.View {
				err = fmt.Errorf("view does not match")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("viewchange from %d: %s", vcsrc, err)
		}
		vcs = append(vcs, vc)
	}

	return vcs, nil
}

func (s *SBFT) handleNewView(nv *NewView, src uint64) {
	if src != s.primaryIDView(nv.View) {
		log.Warningf("replica %d: invalid new view from %d for %d", s.id, src, nv.View)
		return
	}

	if onv := s.replicaState[s.primaryIDView(nv.View)].newview; onv != nil && onv.View >= nv.View {
		log.Debugf("replica %d: discarding duplicate new view for %d", s.id, nv.View)
		return
	}

	vcs, err := s.checkNewViewSignatures(nv)
	if err != nil {
		log.Warningf("replica %d: invalid new view from %d: %s", s.id, src, err)
		s.sendViewChange()
		return
	}

	xset, _, ok := s.makeXset(vcs)

	if !ok || !reflect.DeepEqual(nv.Xset, xset) {
		log.Warningf("replica %d: invalid new view from %d: xset incorrect: %v, %v", s.id, src, nv.Xset, xset)
		s.sendViewChange()
		return
	}

	if nv.Xset == nil {
		if nv.Batch != nil {
			log.Warningf("replica %d: invalid new view from %d: null request should come with null batch", s.id, src)
			s.sendViewChange()
			return
		}
	} else if nv.Batch == nil || !bytes.Equal(nv.Batch.Hash(), nv.Xset.Digest) {
		log.Warningf("replica %d: invalid new view from %d: batch head hash does not match xset: %x, %x, %v",
			s.id, src, hash(nv.Batch.Header), nv.Xset.Digest, nv)
		s.sendViewChange()
		return
	}

	if nv.Batch != nil {
		_, err = s.checkBatch(nv.Batch, true, false)
		if err != nil {
			log.Warningf("replica %d: invalid new view from %d: invalid batch, %s",
				s.id, src, err)
			s.sendViewChange()
			return
		}
	}

	s.replicaState[s.primaryIDView(nv.View)].newview = nv

	if nv.View > s.view {
		s.view = nv.View
		s.activeView = false
	}

	s.processNewView()
}

func (s *SBFT) processNewView() {
	if s.activeView {
		return
	}

	nv := s.replicaState[s.primaryIDView(s.view)].newview
	if nv == nil || nv.View != s.view {
		return
	}

	s.activeView = true
	s.discardBacklog(s.primaryID())

	s.maybeDeliverUsingXset(nv)

	// By now we cannot be waiting for any more outstanding
	// messages.  after a new-view message, by definition all
	// activity has acquiesced.  Prepare to accept a new request.
	s.cur.checkpointDone = true
	s.cur.subject.Seq.Seq = 0

	log.Infof("replica %d now active in view %d; primary: %v", s.id, s.view, s.isPrimary())

	if nv.Batch != nil {
		pp := &Preprepare{
			Seq:   &SeqView{Seq: nv.Batch.DecodeHeader().Seq, View: s.view},
			Batch: nv.Batch,
		}

		s.handleCheckedPreprepare(pp)
	} else {
		log.Debugf("replica %d: %+v", s.id, s)
		s.cancelViewChangeTimer()
		s.maybeSendNextBatch()
	}

	s.processBacklog()
}

func (s *SBFT) maybeDeliverUsingXset(nv *NewView) {
	// TODO we could cache vcs in replicaState
	vcs, err := s.checkNewViewSignatures(nv)
	if err != nil {
		panic(err)
	}

	_, prevBatch, ok := s.makeXset(vcs)
	if !ok {
		panic("invalid newview")
	}
	if s.sys.LastBatch().DecodeHeader().Seq < prevBatch.DecodeHeader().Seq {
		if prevBatch.DecodeHeader().Seq == s.cur.subject.Seq.Seq {
			// we just received a signature set for a request which we preprepared, but never delivered.
			prevBatch.Payloads = s.cur.preprep.Batch.Payloads
		}
		s.cur.checkpointDone = true
		s.deliverBatch(prevBatch)
	}
}
