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

package pbft

import (
	"encoding/base64"
	"fmt"
	"reflect"

	"github.com/hyperledger/fabric/consensus/util/events"
)

// viewChangeQuorumEvent is returned to the event loop when a new ViewChange message is received which is part of a quorum cert
type viewChangeQuorumEvent struct{}

func (instance *pbftCore) correctViewChange(vc *ViewChange) bool {
	for _, p := range append(vc.Pset, vc.Qset...) {
		if !(p.View < vc.View && p.SequenceNumber > vc.H && p.SequenceNumber <= vc.H+instance.L) {
			logger.Debugf("Replica %d invalid p entry in view-change: vc(v:%d h:%d) p(v:%d n:%d)",
				instance.id, vc.View, vc.H, p.View, p.SequenceNumber)
			return false
		}
	}

	for _, c := range vc.Cset {
		// PBFT: the paper says c.n > vc.h
		if !(c.SequenceNumber >= vc.H && c.SequenceNumber <= vc.H+instance.L) {
			logger.Debugf("Replica %d invalid c entry in view-change: vc(v:%d h:%d) c(n:%d)",
				instance.id, vc.View, vc.H, c.SequenceNumber)
			return false
		}
	}

	return true
}

func (instance *pbftCore) calcPSet() map[uint64]*ViewChange_PQ {
	pset := make(map[uint64]*ViewChange_PQ)

	for n, p := range instance.pset {
		pset[n] = p
	}

	// P set: requests that have prepared here
	//
	// "<n,d,v> has a prepared certificate, and no request
	// prepared in a later view with the same number"

	for idx, cert := range instance.certStore {
		if cert.prePrepare == nil {
			continue
		}

		digest := cert.digest
		if !instance.prepared(digest, idx.v, idx.n) {
			continue
		}

		if p, ok := pset[idx.n]; ok && p.View > idx.v {
			continue
		}

		pset[idx.n] = &ViewChange_PQ{
			SequenceNumber: idx.n,
			BatchDigest:    digest,
			View:           idx.v,
		}
	}

	return pset
}

func (instance *pbftCore) calcQSet() map[qidx]*ViewChange_PQ {
	qset := make(map[qidx]*ViewChange_PQ)

	for n, q := range instance.qset {
		qset[n] = q
	}

	// Q set: requests that have pre-prepared here (pre-prepare or
	// prepare sent)
	//
	// "<n,d,v>: requests that pre-prepared here, and did not
	// pre-prepare in a later view with the same number"

	for idx, cert := range instance.certStore {
		if cert.prePrepare == nil {
			continue
		}

		digest := cert.digest
		if !instance.prePrepared(digest, idx.v, idx.n) {
			continue
		}

		qi := qidx{digest, idx.n}
		if q, ok := qset[qi]; ok && q.View > idx.v {
			continue
		}

		qset[qi] = &ViewChange_PQ{
			SequenceNumber: idx.n,
			BatchDigest:    digest,
			View:           idx.v,
		}
	}

	return qset
}

func (instance *pbftCore) sendViewChange() events.Event {
	instance.stopTimer()

	delete(instance.newViewStore, instance.view)
	instance.view++
	instance.activeView = false

	instance.pset = instance.calcPSet()
	instance.qset = instance.calcQSet()

	// clear old messages
	for idx := range instance.certStore {
		if idx.v < instance.view {
			delete(instance.certStore, idx)
		}
	}
	for idx := range instance.viewChangeStore {
		if idx.v < instance.view {
			delete(instance.viewChangeStore, idx)
		}
	}

	vc := &ViewChange{
		View:      instance.view,
		H:         instance.h,
		ReplicaId: instance.id,
	}

	for n, id := range instance.chkpts {
		vc.Cset = append(vc.Cset, &ViewChange_C{
			SequenceNumber: n,
			Id:             id,
		})
	}

	for _, p := range instance.pset {
		if p.SequenceNumber < instance.h {
			logger.Errorf("BUG! Replica %d should not have anything in our pset less than h, found %+v", instance.id, p)
		}
		vc.Pset = append(vc.Pset, p)
	}

	for _, q := range instance.qset {
		if q.SequenceNumber < instance.h {
			logger.Errorf("BUG! Replica %d should not have anything in our qset less than h, found %+v", instance.id, q)
		}
		vc.Qset = append(vc.Qset, q)
	}

	instance.sign(vc)

	logger.Infof("Replica %d sending view-change, v:%d, h:%d, |C|:%d, |P|:%d, |Q|:%d",
		instance.id, vc.View, vc.H, len(vc.Cset), len(vc.Pset), len(vc.Qset))

	instance.innerBroadcast(&Message{Payload: &Message_ViewChange{ViewChange: vc}})

	instance.vcResendTimer.Reset(instance.vcResendTimeout, viewChangeResendTimerEvent{})

	return instance.recvViewChange(vc)
}

func (instance *pbftCore) recvViewChange(vc *ViewChange) events.Event {
	logger.Infof("Replica %d received view-change from replica %d, v:%d, h:%d, |C|:%d, |P|:%d, |Q|:%d",
		instance.id, vc.ReplicaId, vc.View, vc.H, len(vc.Cset), len(vc.Pset), len(vc.Qset))

	if err := instance.verify(vc); err != nil {
		logger.Warningf("Replica %d found incorrect signature in view-change message: %s", instance.id, err)
		return nil
	}

	if vc.View < instance.view {
		logger.Warningf("Replica %d found view-change message for old view", instance.id)
		return nil
	}

	if !instance.correctViewChange(vc) {
		logger.Warningf("Replica %d found view-change message incorrect", instance.id)
		return nil
	}

	if _, ok := instance.viewChangeStore[vcidx{vc.View, vc.ReplicaId}]; ok {
		logger.Warningf("Replica %d already has a view change message for view %d from replica %d", instance.id, vc.View, vc.ReplicaId)
		return nil
	}

	instance.viewChangeStore[vcidx{vc.View, vc.ReplicaId}] = vc

	// PBFT TOCS 4.5.1 Liveness: "if a replica receives a set of
	// f+1 valid VIEW-CHANGE messages from other replicas for
	// views greater than its current view, it sends a VIEW-CHANGE
	// message for the smallest view in the set, even if its timer
	// has not expired"
	replicas := make(map[uint64]bool)
	minView := uint64(0)
	for idx := range instance.viewChangeStore {
		if idx.v <= instance.view {
			continue
		}

		replicas[idx.id] = true
		if minView == 0 || idx.v < minView {
			minView = idx.v
		}
	}

	// We only enter this if there are enough view change messages _greater_ than our current view
	if len(replicas) >= instance.f+1 {
		logger.Infof("Replica %d received f+1 view-change messages, triggering view-change to view %d",
			instance.id, minView)
		// subtract one, because sendViewChange() increments
		instance.view = minView - 1
		return instance.sendViewChange()
	}

	quorum := 0
	for idx := range instance.viewChangeStore {
		if idx.v == instance.view {
			quorum++
		}
	}
	logger.Debugf("Replica %d now has %d view change requests for view %d", instance.id, quorum, instance.view)

	if !instance.activeView && vc.View == instance.view && quorum >= instance.allCorrectReplicasQuorum() {
		instance.vcResendTimer.Stop()
		instance.startTimer(instance.lastNewViewTimeout, "new view change")
		instance.lastNewViewTimeout = 2 * instance.lastNewViewTimeout
		return viewChangeQuorumEvent{}
	}

	return nil
}

func (instance *pbftCore) sendNewView() events.Event {

	if _, ok := instance.newViewStore[instance.view]; ok {
		logger.Debugf("Replica %d already has new view in store for view %d, skipping", instance.id, instance.view)
		return nil
	}

	vset := instance.getViewChanges()

	cp, ok, _ := instance.selectInitialCheckpoint(vset)
	if !ok {
		logger.Infof("Replica %d could not find consistent checkpoint: %+v", instance.id, instance.viewChangeStore)
		return nil
	}

	msgList := instance.assignSequenceNumbers(vset, cp.SequenceNumber)
	if msgList == nil {
		logger.Infof("Replica %d could not assign sequence numbers for new view", instance.id)
		return nil
	}

	nv := &NewView{
		View:      instance.view,
		Vset:      vset,
		Xset:      msgList,
		ReplicaId: instance.id,
	}

	logger.Infof("Replica %d is new primary, sending new-view, v:%d, X:%+v",
		instance.id, nv.View, nv.Xset)

	instance.innerBroadcast(&Message{Payload: &Message_NewView{NewView: nv}})
	instance.newViewStore[instance.view] = nv
	return instance.processNewView()
}

func (instance *pbftCore) recvNewView(nv *NewView) events.Event {
	logger.Infof("Replica %d received new-view %d",
		instance.id, nv.View)

	if !(nv.View > 0 && nv.View >= instance.view && instance.primary(nv.View) == nv.ReplicaId && instance.newViewStore[nv.View] == nil) {
		logger.Infof("Replica %d rejecting invalid new-view from %d, v:%d",
			instance.id, nv.ReplicaId, nv.View)
		return nil
	}

	for _, vc := range nv.Vset {
		if err := instance.verify(vc); err != nil {
			logger.Warningf("Replica %d found incorrect view-change signature in new-view message: %s", instance.id, err)
			return nil
		}
	}

	instance.newViewStore[nv.View] = nv
	return instance.processNewView()
}

func (instance *pbftCore) processNewView() events.Event {
	var newReqBatchMissing bool
	nv, ok := instance.newViewStore[instance.view]
	if !ok {
		logger.Debugf("Replica %d ignoring processNewView as it could not find view %d in its newViewStore", instance.id, instance.view)
		return nil
	}

	if instance.activeView {
		logger.Infof("Replica %d ignoring new-view from %d, v:%d: we are active in view %d",
			instance.id, nv.ReplicaId, nv.View, instance.view)
		return nil
	}

	cp, ok, replicas := instance.selectInitialCheckpoint(nv.Vset)
	if !ok {
		logger.Warningf("Replica %d could not determine initial checkpoint: %+v",
			instance.id, instance.viewChangeStore)
		return instance.sendViewChange()
	}

	speculativeLastExec := instance.lastExec
	if instance.currentExec != nil {
		speculativeLastExec = *instance.currentExec
	}

	// If we have not reached the sequence number, check to see if we can reach it without state transfer
	// In general, executions are better than state transfer
	if speculativeLastExec < cp.SequenceNumber {
		canExecuteToTarget := true
	outer:
		for seqNo := speculativeLastExec + 1; seqNo <= cp.SequenceNumber; seqNo++ {
			found := false
			for idx, cert := range instance.certStore {
				if idx.n != seqNo {
					continue
				}

				quorum := 0
				for _, p := range cert.commit {
					// Was this committed in the previous view
					if p.View == idx.v && p.SequenceNumber == seqNo {
						quorum++
					}
				}

				if quorum < instance.intersectionQuorum() {
					logger.Debugf("Replica %d missing quorum of commit certificate for seqNo=%d, only has %d of %d", instance.id, quorum, instance.intersectionQuorum())
					continue
				}

				found = true
				break
			}

			if !found {
				canExecuteToTarget = false
				logger.Debugf("Replica %d missing commit certificate for seqNo=%d", instance.id, seqNo)
				break outer
			}

		}

		if canExecuteToTarget {
			logger.Debugf("Replica %d needs to process a new view, but can execute to the checkpoint seqNo %d, delaying processing of new view", instance.id, cp.SequenceNumber)
			return nil
		}

		logger.Infof("Replica %d cannot execute to the view change checkpoint with seqNo %d", instance.id, cp.SequenceNumber)
	}

	msgList := instance.assignSequenceNumbers(nv.Vset, cp.SequenceNumber)
	if msgList == nil {
		logger.Warningf("Replica %d could not assign sequence numbers: %+v",
			instance.id, instance.viewChangeStore)
		return instance.sendViewChange()
	}

	if !(len(msgList) == 0 && len(nv.Xset) == 0) && !reflect.DeepEqual(msgList, nv.Xset) {
		logger.Warningf("Replica %d failed to verify new-view Xset: computed %+v, received %+v",
			instance.id, msgList, nv.Xset)
		return instance.sendViewChange()
	}

	if instance.h < cp.SequenceNumber {
		instance.moveWatermarks(cp.SequenceNumber)
	}

	if speculativeLastExec < cp.SequenceNumber {
		logger.Warningf("Replica %d missing base checkpoint %d (%s), our most recent execution %d", instance.id, cp.SequenceNumber, cp.Id, speculativeLastExec)

		snapshotID, err := base64.StdEncoding.DecodeString(cp.Id)
		if nil != err {
			err = fmt.Errorf("Replica %d received a view change whose hash could not be decoded (%s)", instance.id, cp.Id)
			logger.Error(err.Error())
			return nil
		}

		target := &stateUpdateTarget{
			checkpointMessage: checkpointMessage{
				seqNo: cp.SequenceNumber,
				id:    snapshotID,
			},
			replicas: replicas,
		}

		instance.updateHighStateTarget(target)
		instance.stateTransfer(target)
	}

	for n, d := range nv.Xset {
		// PBFT: why should we use "h ≥ min{n | ∃d : (<n,d> ∈ X)}"?
		// "h ≥ min{n | ∃d : (<n,d> ∈ X)} ∧ ∀<n,d> ∈ X : (n ≤ h ∨ ∃m ∈ in : (D(m) = d))"
		if n <= instance.h {
			continue
		} else {
			if d == "" {
				// NULL request; skip
				continue
			}

			if _, ok := instance.reqBatchStore[d]; !ok {
				logger.Warningf("Replica %d missing assigned, non-checkpointed request batch %s",
					instance.id, d)
				if _, ok := instance.missingReqBatches[d]; !ok {
					logger.Warningf("Replica %v requesting to fetch batch %s",
						instance.id, d)
					newReqBatchMissing = true
					instance.missingReqBatches[d] = true
				}
			}
		}
	}

	if len(instance.missingReqBatches) == 0 {
		return instance.processNewView2(nv)
	} else if newReqBatchMissing {
		instance.fetchRequestBatches()
	}

	return nil
}

func (instance *pbftCore) processNewView2(nv *NewView) events.Event {
	logger.Infof("Replica %d accepting new-view to view %d", instance.id, instance.view)

	instance.stopTimer()
	instance.nullRequestTimer.Stop()

	instance.activeView = true
	delete(instance.newViewStore, instance.view-1)

	instance.seqNo = instance.h
	for n, d := range nv.Xset {
		if n <= instance.h {
			continue
		}

		reqBatch, ok := instance.reqBatchStore[d]
		if !ok && d != "" {
			logger.Criticalf("Replica %d is missing request batch for seqNo=%d with digest '%s' for assigned prepare after fetching, this indicates a serious bug", instance.id, n, d)
		}
		preprep := &PrePrepare{
			View:           instance.view,
			SequenceNumber: n,
			BatchDigest:    d,
			RequestBatch:   reqBatch,
			ReplicaId:      instance.id,
		}
		cert := instance.getCert(instance.view, n)
		cert.prePrepare = preprep
		cert.digest = d
		if n > instance.seqNo {
			instance.seqNo = n
		}
		instance.persistQSet()
	}

	instance.updateViewChangeSeqNo()

	if instance.primary(instance.view) != instance.id {
		for n, d := range nv.Xset {
			prep := &Prepare{
				View:           instance.view,
				SequenceNumber: n,
				BatchDigest:    d,
				ReplicaId:      instance.id,
			}
			if n > instance.h {
				cert := instance.getCert(instance.view, n)
				cert.sentPrepare = true
				instance.recvPrepare(prep)
			}
			instance.innerBroadcast(&Message{Payload: &Message_Prepare{Prepare: prep}})
		}
	} else {
		logger.Debugf("Replica %d is now primary, attempting to resubmit requests", instance.id)
		instance.resubmitRequestBatches()
	}

	instance.startTimerIfOutstandingRequests()

	logger.Debugf("Replica %d done cleaning view change artifacts, calling into consumer", instance.id)

	return viewChangedEvent{}
}

func (instance *pbftCore) getViewChanges() (vset []*ViewChange) {
	for _, vc := range instance.viewChangeStore {
		vset = append(vset, vc)
	}

	return
}

func (instance *pbftCore) selectInitialCheckpoint(vset []*ViewChange) (checkpoint ViewChange_C, ok bool, replicas []uint64) {
	checkpoints := make(map[ViewChange_C][]*ViewChange)
	for _, vc := range vset {
		for _, c := range vc.Cset { // TODO, verify that we strip duplicate checkpoints from this set
			checkpoints[*c] = append(checkpoints[*c], vc)
			logger.Debugf("Replica %d appending checkpoint from replica %d with seqNo=%d, h=%d, and checkpoint digest %s", instance.id, vc.ReplicaId, vc.H, c.SequenceNumber, c.Id)
		}
	}

	if len(checkpoints) == 0 {
		logger.Debugf("Replica %d has no checkpoints to select from: %d %s",
			instance.id, len(instance.viewChangeStore), checkpoints)
		return
	}

	for idx, vcList := range checkpoints {
		// need weak certificate for the checkpoint
		if len(vcList) <= instance.f { // type casting necessary to match types
			logger.Debugf("Replica %d has no weak certificate for n:%d, vcList was %d long",
				instance.id, idx.SequenceNumber, len(vcList))
			continue
		}

		quorum := 0
		// Note, this is the whole vset (S) in the paper, not just this checkpoint set (S') (vcList)
		// We need 2f+1 low watermarks from S below this seqNo from all replicas
		// We need f+1 matching checkpoints at this seqNo (S')
		for _, vc := range vset {
			if vc.H <= idx.SequenceNumber {
				quorum++
			}
		}

		if quorum < instance.intersectionQuorum() {
			logger.Debugf("Replica %d has no quorum for n:%d", instance.id, idx.SequenceNumber)
			continue
		}

		replicas = make([]uint64, len(vcList))
		for i, vc := range vcList {
			replicas[i] = vc.ReplicaId
		}

		if checkpoint.SequenceNumber <= idx.SequenceNumber {
			checkpoint = idx
			ok = true
		}
	}

	return
}

func (instance *pbftCore) assignSequenceNumbers(vset []*ViewChange, h uint64) (msgList map[uint64]string) {
	msgList = make(map[uint64]string)

	maxN := h + 1

	// "for all n such that h < n <= h + L"
nLoop:
	for n := h + 1; n <= h+instance.L; n++ {
		// "∃m ∈ S..."
		for _, m := range vset {
			// "...with <n,d,v> ∈ m.P"
			for _, em := range m.Pset {
				quorum := 0
				// "A1. ∃2f+1 messages m' ∈ S"
			mpLoop:
				for _, mp := range vset {
					if mp.H >= n {
						continue
					}
					// "∀<n,d',v'> ∈ m'.P"
					for _, emp := range mp.Pset {
						if n == emp.SequenceNumber && !(emp.View < em.View || (emp.View == em.View && emp.BatchDigest == em.BatchDigest)) {
							continue mpLoop
						}
					}
					quorum++
				}

				if quorum < instance.intersectionQuorum() {
					continue
				}

				quorum = 0
				// "A2. ∃f+1 messages m' ∈ S"
				for _, mp := range vset {
					// "∃<n,d',v'> ∈ m'.Q"
					for _, emp := range mp.Qset {
						if n == emp.SequenceNumber && emp.View >= em.View && emp.BatchDigest == em.BatchDigest {
							quorum++
						}
					}
				}

				if quorum < instance.f+1 {
					continue
				}

				// "then select the request with digest d for number n"
				msgList[n] = em.BatchDigest
				maxN = n

				continue nLoop
			}
		}

		quorum := 0
		// "else if ∃2f+1 messages m ∈ S"
	nullLoop:
		for _, m := range vset {
			// "m.P has no entry"
			for _, em := range m.Pset {
				if em.SequenceNumber == n {
					continue nullLoop
				}
			}
			quorum++
		}

		if quorum >= instance.intersectionQuorum() {
			// "then select the null request for number n"
			msgList[n] = ""

			continue nLoop
		}

		logger.Warningf("Replica %d could not assign value to contents of seqNo %d, found only %d missing P entries", instance.id, n, quorum)
		return nil
	}

	// prune top null requests
	for n, msg := range msgList {
		if n > maxN && msg == "" {
			delete(msgList, n)
		}
	}

	return
}
