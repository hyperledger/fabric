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

	"math"

	"fmt"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

const chainId = "id"
const lowN uint64 = 4   //keep lowN greater or equal to 4
const highN uint64 = 10 //keep highN greater or equal to 10

var testLog = logging.MustGetLogger("test")

func init() {
	// logging.SetLevel(logging.NOTICE, "")
	// logging.SetLevel(logging.DEBUG, "test")
	logging.SetLevel(logging.DEBUG, "sbft")
}

func skipInShortMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}
}

func connectAll(sys *testSystem, chainIds []string) {
	for _, chainId := range chainIds {
		connectAllForChainId(sys, chainId)
	}
}

func connectAllForDefaultChain(sys *testSystem) {
	connectAllForChainId(sys, chainId)
}

func connectAllForChainId(sys *testSystem, chainId string) {
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
				a.receivers[chainId].Connection(b.id)
			}
		}
	}
	sys.Run()
}

func TestMultiChain(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	M := uint64(5)
	sys := newTestSystem(N)
	chainIds := make([]string, 0, M)
	var repls map[string][]*SBFT = map[string][]*SBFT{}
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		for j := uint64(0); j < M; j++ {
			chainId := fmt.Sprintf("%d", j)
			s, err := New(i, chainId, &Config{N: N, F: 0, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
			if err != nil {
				t.Fatal(err)
			}
			repls[chainId] = append(repls[chainId], s)
			if uint64(len(chainIds)) < M {
				chainIds = append(chainIds, chainId)
			}
		}
		adapters = append(adapters, a)
	}
	connectAll(sys, chainIds)
	r1 := []byte{1, 2, 3}
	for i := uint64(0); i < N; i++ {
		for j := uint64(0); j < M; j++ {
			if j%uint64(2) == 0 {
				chainId := fmt.Sprintf("%d", j)
				repls[chainId][i].Request(r1)
			}
		}
	}
	sys.Run()
	for _, a := range adapters {
		for chainId := range a.batches {
			// we check that if this is a chain where we sent a req then the req
			// was written to the "ledger"
			j, _ := strconv.ParseInt(chainId, 10, 64)
			if j%2 == 0 && len(a.batches[chainId]) != 1 {
				t.Fatalf("expected one batches on chain %s", chainId)
			}
			// in other cases, we should have at most an empty ledger
			if j%2 != 0 && len(a.batches[chainId]) != 0 {
				t.Fatalf("expected one batches on chain %s", chainId)
			}
		}
	}
}

func TestSBFT(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestQuorumSizes(t *testing.T) {
	for N := uint64(1); N < 100; N++ {
		for f := uint64(0); f <= uint64(math.Floor(float64(N-1)/float64(3))); f++ {
			sys := newTestSystem(N)
			a := sys.NewAdapter(0)
			s, err := New(0, chainId, &Config{N: N, F: f, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
			if err != nil {
				t.Fatal(err)
			}
			if uint64(2*s.commonCaseQuorum())-N < f+1 {
				t.Fatal("insufficient intersection of two common case quorums", "N = ", N, " F = ", f)
			}
			if uint64(s.commonCaseQuorum()+s.viewChangeQuorum())-N < f+1 {
				t.Fatal("insufficient intersection of common case and view change quorums", "N = ", N, " F = ", f)
			}
			if f < uint64(math.Floor(float64(N-1)/float64(3))) {
				//end test for unoptimized f
				continue
			}
			//test additionally when f is optimized
			switch int(math.Mod(float64(N), float64(3))) {
			case 1:
				if s.commonCaseQuorum() != int(N-f) || s.viewChangeQuorum() != int(N-f) {
					t.Fatal("quorum sizes are wrong in default case N mod 3 == 1")
				}
			case 2:
				if s.viewChangeQuorum() >= s.commonCaseQuorum() || s.viewChangeQuorum() >= int(N-f) {
					t.Fatal("view change quorums size not optimized when N mod 3 == 2")
				}
			case 3:
				if s.commonCaseQuorum() >= int(N-f) || s.viewChangeQuorum() >= int(N-f) {
					t.Fatal("quorum sizes not optimized when N mod 3 == 3")
				}
			}
		}
	}
}

func TestSBFTDelayed(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	BS := uint64(1)
	sys := newTestSystemWithBatchSize(N, BS)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: BS, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	r2 := []byte{3, 1, 2}
	repls[0].Request(r1)
	repls[1].Request(r2)
	sys.Run()
	for i, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Errorf("expected execution of 2 batches on %d", i)
			continue
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestN1(t *testing.T) {
	skipInShortMode(t)
	N := uint64(1)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 0, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestMonotonicViews(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	repls[0].sendViewChange()
	sys.Run()

	view := repls[0].view
	testLog.Notice("TEST: Replica 0 is in view ", view)
	testLog.Notice("TEST: restarting replica 0")
	repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range adapters {
		if a.id != 0 {
			a.receivers[chainId].Connection(0)
			adapters[0].receivers[chainId].Connection(a.id)
		}
	}
	sys.Run()

	if repls[0].view < view {
		t.Fatalf("Replica 0 must be at least in view %d, but is in view %d", view, repls[0].view)
	}
}

func TestByzPrimaryN4(t *testing.T) {
	skipInShortMode(t)
	N := uint64(4)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed first")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed second")
		}
	}
}

func TestNewPrimaryHandlingViewChange(t *testing.T) {
	skipInShortMode(t)
	N := uint64(7)
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 2, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	r1 := []byte{1, 2, 3}
	r2 := []byte{5, 6, 7}

	// change preprepare to 2-6
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

	connectAllForDefaultChain(sys)
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) < 1 {
			t.Fatal("expected execution of at least one batches")
		}
		if a.batches[chainId][0].Payloads != nil && !reflect.DeepEqual(adapters[2].batches[chainId][0].Payloads, a.batches[chainId][0].Payloads) {
			t.Error("consensus violated on first batches at replica", a.id)
		}
	}
}

func TestByzPrimaryBullyingSingleReplica(t *testing.T) {
	skipInShortMode(t)
	N := highN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	r1 := []byte{1, 2, 3}
	r2 := []byte{5, 6, 7}

	// change preprepare to 1
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if pp := msg.msg.GetPreprepare(); pp != nil && msg.src == 0 && msg.dst == 1 {
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

	connectAllForDefaultChain(sys)
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if a.id != 1 && len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches at all except replica 1")
		}
	}
}

func TestViewChange(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestMsgReordering(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestBacklogReordering(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestViewChangeWithRetransmission(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatal("expected execution of 1 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
	}
}

func TestViewChangeXset(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatalf("expected execution of 1 batches: %v", a.batches[chainId])
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed first")
		}
		if !reflect.DeepEqual([][]byte{r2}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed second")
		}
	}
}

func TestRestart(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	connectAllForDefaultChain(sys)
	// move to view 1
	for _, r := range repls {
		r.sendViewChange()
	}
	sys.Run()

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	testLog.Notice("restarting 0")
	repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range adapters {
		if a.id != 0 {
			a.receivers[chainId].Connection(0)
			adapters[0].receivers[chainId].Connection(a.id)
		}
	}

	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Fatalf("expected execution of 2 batches, %d got %v", a.id, a.batches)
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestAbdicatingPrimary(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	phase := 1
	// Dropping all phase 1 msgs except requests and viewchange to 0
	// (preprepare to primary 0 is automatically delivered)
	sys.filterFn = func(e testElem) (testElem, bool) {
		if phase == 1 {
			if msg, ok := e.ev.(*testMsgEvent); ok {
				if c := msg.msg.GetRequest(); c != nil {
					return e, true
				}
				if c := msg.msg.GetViewChange(); c != nil && msg.dst == 0 {
					return e, true
				}
				return e, false
			}
			return e, true
		}
		return e, true
	}

	connectAllForDefaultChain(sys)

	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	phase = 2

	testLog.Notice("TEST: restarting connections from 0")
	for _, a := range adapters {
		if a.id != 0 {
			a.receivers[chainId].Connection(0)
			adapters[0].receivers[chainId].Connection(a.id)
		}
	}

	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 1 {
			t.Fatalf("expected execution of 1 batches, %d got %v", a.id, a.batches[chainId])
		}
	}
}

func TestRestartAfterPrepare(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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
				repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range adapters {
					if a.id != 0 {
						a.receivers[chainId].Connection(0)
						adapters[0].receivers[chainId].Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAllForDefaultChain(sys)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartAfterCommit(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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
				repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range adapters {
					if a.id != 0 {
						a.receivers[chainId].Connection(0)
						adapters[0].receivers[chainId].Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAllForDefaultChain(sys)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartAfterCheckpoint(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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
				repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range adapters {
					if a.id != 0 {
						a.receivers[chainId].Connection(0)
						adapters[0].receivers[chainId].Connection(a.id)
					}
				}
			}
		}

		return e, true
	}

	connectAllForDefaultChain(sys)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestErroneousViewChange(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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
				repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
				for _, a := range adapters {
					if a.id != 0 {
						a.receivers[chainId].Connection(0)
						adapters[0].receivers[chainId].Connection(a.id)
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
				a.receivers[chainId].Connection(b.id)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestRestartMissedViewChange(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)

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
	repls[0], _ = New(0, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, adapters[0])
	for _, a := range adapters {
		if a.id != 0 {
			a.receivers[chainId].Connection(0)
			adapters[0].receivers[chainId].Connection(a.id)
		}
	}

	r3 := []byte{3, 5, 2}
	repls[1].Request(r3)

	sys.Run()

	for _, a := range adapters {
		if len(a.batches[chainId]) == 0 {
			t.Fatalf("expected execution of some batches on %d", a.id)
		}

		if !reflect.DeepEqual([][]byte{r3}, a.batches[chainId][len(a.batches[chainId])-1].Payloads) {
			t.Errorf("wrong request executed on %d: %v", a.id, a.batches[chainId][2])
		}
	}
}

func TestFullBacklog(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	BS := uint64(1)
	sys := newTestSystemWithBatchSize(N, BS)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: BS, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	r1 := []byte{1, 2, 3}

	connectAllForDefaultChain(sys)
	sys.enqueue(200*time.Millisecond, &testTimer{id: 999, tf: func() {
		repls[0].sys.Send(chainId, &Msg{&Msg_Prepare{&Subject{Seq: &SeqView{Seq: 100}}}}, 1)
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
		if len(a.batches[chainId]) == 0 {
			t.Fatalf("expected execution of batches on %d", a.id)
		}
		bh := a.batches[chainId][len(a.batches[chainId])-1].DecodeHeader()
		if bh.Seq != 10 {
			t.Errorf("wrong request executed on %d: %v", a.id, bh)
		}
	}
}

func TestHelloMsg(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	BS := uint64(1)
	sys := newTestSystemWOTimersWithBatchSize(N, BS)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: uint64(BS), RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	phase := 1

	// We are going to deliver only pre-prepare of the first request to replica 1
	// other messages pertaining to first request, destined to 1 will be dropped
	sys.filterFn = func(e testElem) (testElem, bool) {
		if msg, ok := e.ev.(*testMsgEvent); ok {
			if msg.dst == 1 {
				c := msg.msg.GetPreprepare()
				if c != nil && c.Seq.View == 0 && phase == 1 {
					return e, true // letting the first pre-prepare be delivered to 1
				}
				if phase > 1 {
					return e, true //letting msgs outside phase 1 through
				}
				return e, false //dropping other phase 1 msgs
			}
		}
		return e, true
	}

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()

	phase = 2 //start delivering msgs to replica 1

	testLog.Notice("restarting replica 1")
	repls[1], _ = New(1, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: BS, RequestTimeoutNsec: 20000000000}, adapters[1])
	for _, a := range adapters {
		if a.id != 1 {
			a.receivers[chainId].Connection(1)
			adapters[1].receivers[chainId].Connection(a.id)
		}
	}

	sys.Run()

	phase = 3

	r3 := []byte{3, 5, 2}
	repls[1].Request(r3)
	sys.Run()

	for i, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Fatalf("expected execution of 2 batches but executed %d     %d", len(a.batches[chainId]), i)
		}
	}
}

func TestViewChangeTimer(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
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
		if len(a.batches[chainId]) != 2 {
			t.Fatalf("%d: expected execution of 2 batches: %v", a.id, a.batches[chainId])
		}
		if a.id != 3 {
			if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
				t.Errorf("%d: wrong request executed (1): %v", a.id, a.batches[chainId])
			}
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Errorf("%d: wrong request executed (2): %v", a.id, a.batches[chainId])
		}
	}
}

func TestResendViewChange(t *testing.T) {
	skipInShortMode(t)
	N := lowN
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 10, RequestTimeoutNsec: 20000000000}, a)
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

	connectAllForDefaultChain(sys)
	r1 := []byte{1, 2, 3}
	repls[0].Request(r1)
	sys.Run()
	r2 := []byte{3, 1, 2}
	r3 := []byte{3, 5, 2}
	repls[1].Request(r2)
	repls[1].Request(r3)
	sys.Run()
	for _, a := range adapters {
		if len(a.batches[chainId]) != 2 {
			t.Fatal("expected execution of 2 batches")
		}
		if !reflect.DeepEqual([][]byte{r1}, a.batches[chainId][0].Payloads) {
			t.Error("wrong request executed (1)")
		}
		if !reflect.DeepEqual([][]byte{r2, r3}, a.batches[chainId][1].Payloads) {
			t.Error("wrong request executed (2)")
		}
	}
}

func TestTenReplicasBombedWithRequests(t *testing.T) {
	skipInShortMode(t)
	N := highN
	requestNumber := 11
	sys := newTestSystem(N)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 3, BatchDurationNsec: 2000000000, BatchSizeBytes: 3, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			t.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}

	connectAllForDefaultChain(sys)
	for i := 0; i < requestNumber; i++ {
		r := []byte{byte(i), 2, 3}
		repls[2].Request(r)
	}
	sys.Run()
	for _, a := range adapters {
		i := 0
		for _, b := range a.batches[chainId] {
			i = i + len(b.Payloads)
		}
		if i != requestNumber {
			t.Fatalf("expected execution of %d requests but: %d", requestNumber, i)
		}
	}
}
