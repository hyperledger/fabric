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

import "testing"

func TestOrderedRequests(t *testing.T) {
	or := &orderedRequests{}
	or.empty()

	r1 := createPbftReq(2, 1)
	r2 := createPbftReq(2, 2)
	r3 := createPbftReq(19, 1)
	if or.has(or.wrapRequest(r1).key) {
		t.Errorf("should not have req")
	}
	or.add(r1)
	if !or.has(or.wrapRequest(r1).key) {
		t.Errorf("should have req")
	}
	if or.has(or.wrapRequest(r2).key) {
		t.Errorf("should not have req")
	}
	if or.remove(r2) {
		t.Errorf("should not have removed req")
	}
	if !or.remove(r1) {
		t.Errorf("should have removed req")
	}
	if or.remove(r1) {
		t.Errorf("should not have removed req")
	}
	if or.order.Len() != 0 || len(or.presence) != 0 {
		t.Errorf("should have 0 len")
	}
	or.adds([]*Request{r1, r2, r3})

	if or.order.Back().Value.(requestContainer).req != r3 {
		t.Errorf("incorrect order")
	}
}

func BenchmarkOrderedRequests(b *testing.B) {
	or := &orderedRequests{}
	or.empty()

	Nreq := 1000

	reqs := make(map[string]*Request)
	for i := 0; i < Nreq; i++ {
		rc := or.wrapRequest(createPbftReq(int64(i), 0))
		reqs[rc.key] = rc.req
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, r := range reqs {
			or.add(r)
		}

		for k := range reqs {
			_ = or.has(k)
		}

		for _, r := range reqs {
			or.remove(r)
		}
	}
}
