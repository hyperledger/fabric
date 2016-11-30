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
	//logging.SetLevel(logging.DEBUG, "test")
	//logging.SetLevel(logging.DEBUG, "sbft")
}

func connectAll(sys *testSystem) {
	// map iteration is non-deterministic, so use linear iteration instead
	max := uint64(0)
	for _, a := range sys.adapters {
		if a.id > max {
			max = a.id
		}
	}

	for i := uint64(0); i <= max; i++ {
		a, ok := sys.adapters[i]
		if !ok {
			continue
		}

		for j := uint64(0); j <= max; j++ {
			b, ok := sys.adapters[j]
			if !ok {
				continue
			}
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
		if len(a.batches) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[0].Payloads) {
			t.Error("wrong request executed first")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[1].Payloads) {
			t.Error("wrong request executed second")
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

func TestMsgReordering(t *testing.T) {
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

	var preprep *testMsgEvent

	// forcing pre-prepare from primary 0 to reach replica 1 after some delay
	// effectivelly delivering pre-prepare instead of checkpoint
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == 0 && msg.dst == 1 {
				c := msg.msg.GetPreprepare()
				if c != nil && c.Seq.View == 0 {
					preprep = msg   //memorizing pre-prepare
					return e, false // but dropping it
				}
				d := msg.msg.GetCheckpoint()
				if d != nil {
					msg.msg = &Msg{&Msg_Preprepare{preprep.msg.GetPreprepare()}}
					return e, true //and delivering it
				}
				return e, false //droping other msgs from 0 to 1
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

func TestBacklogReordering(t *testing.T) {
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

	var preprep *testMsgEvent

	// forcing pre-prepare from primary 0 to reach replica 1 after some delay
	// effectivelly delivering pre-prepare instead of checkpoint
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.src == 0 && msg.dst == 1 {
				c := msg.msg.GetPreprepare()
				if c != nil && c.Seq.View == 0 {
					preprep = msg   //memorizing pre-prepare
					return e, false // but dropping it
				}
				d := msg.msg.GetCheckpoint()
				if d != nil {
					msg.msg = &Msg{&Msg_Preprepare{preprep.msg.GetPreprepare()}}
					return e, true //and delivering it
				}
				return e, true //letting prepare and commit from 0 to 1 pass
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

func TestViewChangeWithRetransmission(t *testing.T) {
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
			if c := msg.msg.GetPrepare(); c != nil && c.Seq.View == 0 {
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
			t.Fatalf("expected execution of 1 batch: %v", a.batches)
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed first")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[1].Payloads) {
			t.Error("wrong request executed second")
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
	sys.Run()

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	testLog.Notice("restarting 0")
	repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range adapters {
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
		if len(a.batches) != 2 {
			t.Fatalf("expected execution of 2 batches, %d got %v", a.id, a.batches)
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[1].Payloads) {
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
				testLog.Notice("restarting 0")
				repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range adapters {
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
	sys.Run()

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
				for _, a := range adapters {
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
	sys.Run()

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

func TestRestartAfterCheckpoint(t *testing.T) {
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
				for _, a := range adapters {
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
	sys.Run()

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

func TestErroneousViewChange(t *testing.T) {
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
				for _, a := range adapters {
					if a.id != 0 {
						a.receiver.Connection(0)
						adapters[0].receiver.Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	// iteration order here is essential to trigger the bug
	outer := []uint64{2, 3, 0, 1}
	inner := []uint64{0, 1, 2, 3}
	for _, i := range outer {
		a, ok := sys.adapters[i]
		if !ok {
			continue
		}

		for _, j := range inner {
			b, ok := sys.adapters[j]
			if !ok {
				continue
			}
			if a.id != b.id {
				a.receiver.Connection(b.id)
			}
		}
	}
	sys.Run()

	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}
	sys.Run()

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

func TestRestartMissedViewChange(t *testing.T) {
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

	disconnect := false

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if disconnect && (msg.src == 0 || msg.dst == 0) {
				return e, false
			}
		}

		return e, true
	}

	connectAll(sys)

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	disconnect = true
	// move to view 1
	for _, r := range repls {
		if r.id != 0 {
			r.sendViewChange()
		}
	}
	sys.Run()

	r2 := []byte{3, 1, 2}
	repls[1].Request(r2)
	sys.Run()

	disconnect = false
	testLog.Notice("restarting 0")
	repls[0], _ = New(0, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range adapters {
		if a.id != 0 {
			a.receiver.Connection(0)
			adapters[0].receiver.Connection(a.id)
		}
	}

	r3 := []byte{3, 5, 2}
	repls[1].Request(r3)
	sys.Run()

	for _, a := range adapters {
		if len(a.batches) == 0 {
			t.Fatalf("expected execution of some batches on %d", a.id)
		}

		if !reflect.DeepEqual([][]byte{r3}, a.batches[len(a.batches)-1].Payloads) {
			t.Errorf("wrong request executed on %d: %v", a.id, a.batches[2])
		}
	}
}

func TestFullBacklog(t *testing.T) {
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

	connectAll(sys)
	sys.enqueue(200*time.Millisecond, &testTimer{id: 999, tf: func() {
		repls[0].sys.Send(&Msg{&Msg_Prepare{&Subject{Seq: &SeqView{Seq: 100}}}}, 1)
	}})
	for i := 0; i < 10; i++ {
		sys.enqueue(time.Duration(i)*100*time.Millisecond, &testTimer{id: 999, tf: func() {
			repls[0].Request(r1)
		}})
	}
	sys.Run()
	if len(repls[1].replicaState[2].backLog) > 4*3 {
		t.Errorf("backlog too long: %d", len(repls[1].replicaState[0].backLog))
	}
	for _, a := range adapters {
		if len(a.batches) == 0 {
			t.Fatalf("expected execution of batches on %d", a.id)
		}
		bh := a.batches[len(a.batches)-1].DecodeHeader()
		if bh.Seq != 10 {
			t.Errorf("wrong request executed on %d: %v", a.id, bh)
		}
	}
}

func TestViewChangeTimer(t *testing.T) {
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

	phase := 1

	// network outage after prepares are received
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.dst == msg.src {
				return e, true
			} else if msg.src == 1 && phase == 2 {
				return e, false
			}
			testLog.Debugf("passing msg from %d to %d, phase %d", msg.src, msg.dst, phase)
		}

		return e, true
	}

	connectAll(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)

	repls[3].sendViewChange()

	sys.enqueue(10*time.Minute, &testTimer{id: 999, tf: func() {
		if repls[3].view != 1 {
			t.Fatalf("expected view not to advance past 1, we are in %d", repls[3].view)
		}
	}})

	sys.enqueue(11*time.Minute, &testTimer{id: 999, tf: func() {
		phase = 2
		repls[2].sendViewChange()
	}})

	sys.enqueue(12*time.Minute, &testTimer{id: 999, tf: func() {
		if repls[3].view != 2 {
			t.Fatalf("expected view not to advance past 2, 3 is in %d", repls[3].view)
		}
	}})

	sys.enqueue(20*time.Minute, &testTimer{id: 999, tf: func() {
		for _, r := range repls {
			if r.view > 4 {
				t.Fatalf("expected view not to advance too much, we are in %d", r.view)
			}
		}
	}})

	sys.Run()
	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[2].Request(r2)
	repls[2].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches) != 2 {
			t.Fatalf("%d: expected execution of 2 batches: %v", a.id, a.batches)
		}
		if a.id != 3 {
			if !reflect.DeepEqual([][]byte{r1}, a.batches[0].Payloads) {
				t.Errorf("%d: wrong request executed (1): %v", a.id, a.batches)
			}
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[1].Payloads) {
			t.Errorf("%d: wrong request executed (2): %v", a.id, a.batches)
		}
	}
}

func TestResendViewChange(t *testing.T) {
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

	phase := make(map[uint64]int)

	// prevent first view change from being delivered
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.dst == msg.src {
				return e, true
			} else if phase[msg.src] == 0 && msg.msg.GetViewChange() != nil {
				return e, false
			} else if msg.msg.GetHello() != nil {
				phase[msg.src] = 1
			}
		}

		return e, true
	}

	for _, r := range repls {
		r.sendViewChange()
	}
	sys.Run()

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

func TestTenReplicasBombedWithRequests(t *testing.T) {
	N := uint64(10)
	requestNumber := 11
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, &Config{N: N, F: 3, BatchDurationNsec: 2000000000, BatchSizeBytes: 3, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	connectAll(sys)
	for i := 0; i < requestNumber; i++ {
		r := []byte{byte(i), 2, 3}
		repls[2].Request(r)
	}
	sys.Run()
	for _, a := range adapters {
		i := 0
		for _, b := range a.batches {
			i = i + len(b.Payloads)
		}
		if i != requestNumber {
			t.Fatalf("expected execution of %d requests but: %d", requestNumber, i)
		}
	}
}
