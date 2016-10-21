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
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

var testLog = logging.MustGetLogger("test")

func init() {
	logging.SetLevel(logging.NOTICE, "")
	logging.SetLevel(logging.NOTICE, "test")
	logging.SetLevel(logging.DEBUG, "sbft")
}

func connectAll(sys *testSystem) {
	for _, a := range sys.adapters {
		for _, b := range sys.adapters {
			if a.id != b.id {
				a.receiver.Connection(b.id)
			}
		}
	}
	sys.Run()
}

func TestSBFT(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	connectAll(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestSBFTDelayed(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	// make replica 3 lag out against 1 and 2
	for i := uint64(1); i < 3; i++ {
		adapters[i].arrivals[3] = 200 * time.Millisecond
		adapters[3].arrivals[i] = 200 * time.Millisecond
	}

	connectAll(sys)
	r1 := []byte{1, 2, 3}
	r2 := []byte{3, 1, 2}
	repls[0].Request(r1)
	repls[1].Request(r2)
	sys.Run()
	for i, a := range adapters {
		if len(a.batches) != 2 {
			t.Errorf("expected execution of 2 batches on %d", i)
			continue
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestN1(t *testing.T) {
	N := uint64(1)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 0, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	connectAll(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 1 {
			t.Fatal("expected execution of 1 batch")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestByzPrimary(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	r1 := []byte{1, 2, 3}
	r2 := []byte{5, 6, 7}

	// change preprepare to 2, 3
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if pp := msg.msg.GetPreprepare(); pp != nil && msg.src == 0 && msg.dst >= 2 {
				pp := *pp
				batch := *pp.Batch
				batch.Payloads = [][]byte{r2}
				pp.Batch = &batch
				h := merkleHashData(batch.Payloads)
				bh := &BatchHeader{}
				proto.Unmarshal(pp.Batch.Header, bh)
				bh.DataHash = h
				bhraw, _ := proto.Marshal(bh)
				pp.Batch.Header = bhraw
				msg.msg = &Msg{&Msg_Preprepare{&pp}}
			}
		}
		return e, true
	}

	connectAll(sys)
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 1 {
			t.Fatal("expected execution of 1 batch")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[0].Payloads) {
			t.Error("wrong request executed")
		}
	}
}

func TestViewChange(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if c := msg.msg.GetCommit(); c != nil && c.Seq.View == 0 {
				return e, false
			}
		}
		return e, true
	}

	connectAll(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 1 {
			t.Fatal("expected execution of 1 batch")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestViewChangeXset(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	phase := 1

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == msg.dst {
				return e, true
			}

			switch phase {
			case 1:
				if p := msg.msg.GetPrepare(); p != nil && p.Seq.View == 0 {
					return e, false
				}
			case 2:
				if nv := msg.msg.GetNewView(); nv != nil {
					phase = 3
					return e, true
				}
				if msg.src == 3 || msg.dst == 3 {
					return e, false
				}
				if c := msg.msg.GetCommit(); c != nil && c.Seq.View == 1 {
					return e, false
				}
			case 3:
				if msg.src == 3 || msg.dst == 3 {
					return e, false
				}
			}
		}
		return e, true
	}

	connectAll(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	phase = 2

	r2 := []byte{5, 6, 7}
	repls[1].Request(r2)
	sys.Run()

	for i, a := range adapters {
		// 3 is disconnected
		if i == 3 {
			continue
		}
		if len(a.batches) != 2 {
			t.Fatal("expected execution of 1 null request + 1 batch")
		}
		if len(a.batches[0].Payloads) != 0 {
			t.Error("not a null request")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[1].Payloads) {
			t.Error("wrong request executed")
		}
	}
}

func TestRestart(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	connectAll(sys)
	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	testLog.Notice("restarting 0")
	repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range sys.adapters {
		if a.id != 0 {
			a.receiver.Connection(0)
			adapters[0].receiver.Connection(a.id)
		}
	}

	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 3 {
			t.Fatal("expected execution of 3 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[1].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[2].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartAfterPrepare(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	restarted := false

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == msg.dst || msg.src != 0 {
				return e, true
			}

			if p := msg.msg.GetPrepare(); p != nil && p.Seq.Seq == 3 && !restarted {
				restarted = true
				repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range sys.adapters {
					if a.id != 0 {
						a.receiver.Connection(0)
						adapters[0].receiver.Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAll(sys)
	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 3 {
			t.Fatal("expected execution of 3 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[1].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[2].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartAfterCommit(t *testing.T) {
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	restarted := false

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == msg.dst || msg.src != 0 {
				return e, true
			}

			if c := msg.msg.GetCommit(); c != nil && c.Seq.Seq == 3 && !restarted {
				restarted = true
				testLog.Notice("restarting 0")
				repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range sys.adapters {
					if a.id != 0 {
						a.receiver.Connection(0)
						adapters[0].receiver.Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAll(sys)
	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 3 {
			t.Fatal("expected execution of 3 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[1].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[2].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartAfterCheckpoint(t *testing.T) {
	// TODO re-enable this test after https://jira.hyperledger.org/browse/FAB-624 has been resolved
	t.Skip()
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	restarted := false

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == msg.dst || msg.src != 0 {
				return e, true
			}

			if c := msg.msg.GetCheckpoint(); c != nil && c.Seq == 3 && !restarted {
				restarted = true
				testLog.Notice("restarting 0")
				repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range sys.adapters {
					if a.id != 0 {
						a.receiver.Connection(0)
						adapters[0].receiver.Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAll(sys)
	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 3 {
			t.Fatal("expected execution of 3 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[1].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[2].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}
