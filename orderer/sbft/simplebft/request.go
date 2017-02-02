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
	"time"
)

// Request proposes a new request to the BFT network.
func (s *SBFT) Request(req []byte) {
	log.Debugf("replica %d: broadcasting a request", s.id)
	s.broadcast(&Msg{&Msg_Request{&Request{req}}})
}

func (s *SBFT) handleRequest(req *Request, src uint64) {
	key := hash2str(hash(req.Payload))
	log.Infof("replica %d: inserting %x into pending", s.id, key)
	s.pending[key] = req
	if s.isPrimary() && s.activeView {
		batches, committers, valid := s.sys.Validate(s.chainId, req)
		if !valid {
			// this one is problematic, lets skip it
			delete(s.pending, key)
			return
		}
		s.validated[key] = valid
		if len(batches) == 0 {
			s.startBatchTimer()
		} else {
			s.batches = append(s.batches, batches...)
			s.primarycommitters = append(s.primarycommitters, committers...)
			s.maybeSendNextBatch()
		}
	}
}

////////////////////////////////////////////////

func (s *SBFT) startBatchTimer() {
	if s.batchTimer == nil {
		s.batchTimer = s.sys.Timer(time.Duration(s.config.BatchDurationNsec), s.cutAndMaybeSend)
	}
}

func (s *SBFT) cutAndMaybeSend() {
	batch, committers := s.sys.Cut(s.chainId)
	s.batches = append(s.batches, batch)
	s.primarycommitters = append(s.primarycommitters, committers)
	s.maybeSendNextBatch()
}

func (s *SBFT) batchSize() uint64 {
	size := uint64(0)
	if len(s.batches) == 0 {
		return size
	}
	for _, req := range s.batches[0] {
		size += uint64(len(req.Payload))
	}
	return size
}

func (s *SBFT) maybeSendNextBatch() {
	if s.batchTimer != nil {
		s.batchTimer.Cancel()
		s.batchTimer = nil
	}

	if !s.isPrimary() || !s.activeView {
		return
	}

	if !s.cur.checkpointDone {
		return
	}

	if len(s.batches) == 0 {
		hasPending := len(s.pending) != 0
		for k, req := range s.pending {
			if s.validated[k] == false {
				batches, committers, valid := s.sys.Validate(s.chainId, req)
				s.batches = append(s.batches, batches...)
				s.primarycommitters = append(s.primarycommitters, committers...)
				if !valid {
					log.Panicf("Replica %d: one of our own pending requests is erroneous.", s.id)
					delete(s.pending, k)
					continue
				}
				s.validated[k] = true
			}
		}
		if len(s.batches) == 0 {
			// if we have no pending, every req was included in batches
			if !hasPending {
				return
			}
			// we have pending reqs that were just sent for validation or
			// were already sent (they are in s.validated)
			batch, committers := s.sys.Cut(s.chainId)
			s.batches = append(s.batches, batch)
			s.primarycommitters = append(s.primarycommitters, committers)
		}
	}

	batch := s.batches[0]
	s.batches = s.batches[1:]
	committers := s.primarycommitters[0]
	s.primarycommitters = s.primarycommitters[1:]
	s.sendPreprepare(batch, committers)
}
