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
	"fmt"
	"reflect"
)

func (s *SBFT) maybeSendNewView() {
	if s.lastNewViewSent == s.seq.View {
		return
	}

	vset := make(map[uint64]*Signed)
	var vcs []*ViewChange

	for src, vc := range s.viewchange {
		if vc.vc.View == s.seq.View {
			vset[src] = vc.svc
			vcs = append(vcs, vc.vc)
		}
	}

	xset, ok := s.makeXset(vcs)
	if !ok {
		log.Debug("xset not yet sufficient")
		return
	}

	var batch *Batch
	if xset.Digest != nil {
		if reflect.DeepEqual(s.cur.subject.Digest, xset.Digest) {
			batch = s.cur.preprep.Batch
		} else {
			log.Warningf("forfeiting primary - do not have request in store for %d %x", xset.Seq.Seq, xset.Digest)
			xset = nil
		}
	} else {
		batch = s.makeBatch(xset.Seq.Seq, s.sys.LastBatch().Hash(), nil)
		xset.Digest = batch.Hash()
	}

	nv := &NewView{
		View:  s.seq.View,
		Vset:  vset,
		Xset:  xset,
		Batch: batch,
	}

	log.Noticef("sending new view for %d", nv.View)
	s.lastNewViewSent = nv.View
	s.broadcast(&Msg{&Msg_NewView{nv}})
}

func (s *SBFT) handleNewView(nv *NewView, src uint64) {
	if src != s.primaryIDView(nv.View) {
		log.Warningf("invalid new view from %d for %d", src, nv.View)
		return
	}

	if onv, ok := s.newview[s.primaryIDView(nv.View)]; ok && onv.View >= nv.View {
		log.Debugf("discarding duplicate new view for %d", nv.View)
		return
	}

	var vcs []*ViewChange
	for vcsrc, svc := range nv.Vset {
		vc := &ViewChange{}
		err := s.checkSig(svc, vcsrc, vc)
		if err == nil {
			if vc.View != nv.View {
				err = fmt.Errorf("view does not match")
			}
		}
		if err != nil {
			log.Warningf("invalid new view from %d: view change for %d: %s", src, vcsrc, err)
			s.sendViewChange()
			return
		}
		vcs = append(vcs, vc)
	}

	xset, ok := s.makeXset(vcs)
	if xset.Digest == nil {
		// null request special treatment
		xset.Digest = s.makeBatch(nv.Xset.Seq.Seq, s.sys.LastBatch().Hash(), nil).Hash()
	}

	if !ok || !reflect.DeepEqual(nv.Xset, xset) {
		log.Warningf("invalid new view from %d: xset incorrect: %v, %v", src, nv.Xset, xset)
		s.sendViewChange()
		return
	}

	if nv.Batch == nil {
		log.Warningf("invalid new view from %d: batch empty", src)
		s.sendViewChange()
		return
	}

	if !reflect.DeepEqual(hash(nv.Batch.Header), nv.Xset.Digest) {
		log.Warningf("invalid new view from %d: batch head hash does not match xset: %x, %x, %v",
			src, hash(nv.Batch.Header), nv.Xset.Digest, nv)
		s.sendViewChange()
		return
	}

	_, err := s.checkBatch(nv.Batch)
	if err != nil {
		log.Warningf("invalid new view from %d: invalid batch, %s",
			src, err)
		s.sendViewChange()
		return
	}

	s.newview[s.primaryIDView(nv.View)] = nv

	s.processNewView()
}

func (s *SBFT) processNewView() {
	if s.activeView {
		return
	}

	nv, ok := s.newview[s.primaryIDView(s.seq.View)]
	if !ok || nv.View != s.seq.View {
		return
	}

	nextSeq := s.nextSeq()
	if *nv.Xset.Seq != nextSeq {
		log.Infof("we are outdated")
		return
	}

	pp := &Preprepare{
		Seq:   nv.Xset.Seq,
		Batch: nv.Batch,
	}

	s.activeView = true
	var h []byte
	if nv.Batch != nil {
		h = hash(nv.Batch.Header)
	}
	s.acceptPreprepare(Subject{Seq: &nextSeq, Digest: h}, pp)
}
