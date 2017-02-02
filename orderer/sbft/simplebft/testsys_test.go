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
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"runtime"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/filter"
)

const defaultMaxReqCount = uint64(5)

var maxReqCount uint64

type testSystemAdapter struct {
	id  uint64
	sys *testSystem

	// chainId to instance mapping
	receivers   map[string]Receiver
	batches     map[string][]*Batch
	persistence map[string][]byte

	arrivals map[uint64]time.Duration
	reqs     []*Request

	key *ecdsa.PrivateKey
}

func (t *testSystemAdapter) AddReceiver(chainId string, recv Receiver) {
	if t.receivers == nil {
		t.receivers = make(map[string]Receiver)
	}
	if t.receivers[chainId] != nil {
		// remove all events for us
		t.sys.queue.filter(func(e testElem) bool {
			switch e := e.ev.(type) {
			case *testTimer:
				if e.id == t.id {
					return false
				}
			case *testMsgEvent:
				if e.dst == t.id {
					return false
				}
			}
			return true
		})
	}

	t.receivers[chainId] = recv
}

func (t *testSystemAdapter) getArrival(dest uint64) time.Duration {
	// XXX for now, define fixed variance per destination
	arr, ok := t.arrivals[dest]
	if !ok {
		inflight := 20 * time.Millisecond
		variance := 1 * time.Millisecond
		if dest == t.id {
			inflight = 0
		}
		variance = time.Duration(t.sys.rand.Int31n(int32(variance)))
		arr = inflight + variance
		t.arrivals[dest] = arr
	}
	return arr
}

func (t *testSystemAdapter) Send(chainId string, msg *Msg, dest uint64) {
	arr := t.getArrival(dest)
	ev := &testMsgEvent{
		inflight: arr,
		src:      t.id,
		dst:      dest,
		msg:      msg,
		chainId:  chainId,
	}
	// simulate time for marshalling (and unmarshalling)
	bytes, _ := proto.Marshal(msg)
	m2 := &Msg{}
	_ = proto.Unmarshal(bytes, m2)
	t.sys.enqueue(arr, ev)
}

type testMsgEvent struct {
	inflight time.Duration
	src, dst uint64
	msg      *Msg
	chainId  string
}

func (ev *testMsgEvent) Exec(t *testSystem) {
	r := t.adapters[ev.dst]
	if r == nil {
		testLog.Errorf("message to non-existing %s", ev)
		return
	}
	r.receivers[ev.chainId].Receive(ev.msg, ev.src)
}

func (ev *testMsgEvent) String() string {
	return fmt.Sprintf("Message<from %d, to %d, inflight %s, %v",
		ev.src, ev.dst, ev.inflight, ev.msg)
}

type testTimer struct {
	id        uint64
	tf        func()
	cancelled bool
}

func (t *testTimer) Cancel() {
	t.cancelled = true
}

func (t *testTimer) Exec(_ *testSystem) {
	if !t.cancelled {
		t.tf()
	}
}

func (t *testTimer) String() string {
	fun := runtime.FuncForPC(reflect.ValueOf(t.tf).Pointer()).Name()
	return fmt.Sprintf("Timer<on %d, cancelled %v, fun %s>", t.id, t.cancelled, fun)
}

func (t *testSystemAdapter) Timer(d time.Duration, tf func()) Canceller {
	tt := &testTimer{id: t.id, tf: tf}
	if !t.sys.disableTimers {
		t.sys.enqueue(d, tt)
	}
	return tt
}

func (t *testSystemAdapter) Deliver(chainId string, batch *Batch, committer []filter.Committer) {
	if t.batches == nil {
		t.batches = make(map[string][]*Batch)
	}
	if t.batches[chainId] == nil {
		t.batches[chainId] = make([]*Batch, 0, 1)
	}
	t.batches[chainId] = append(t.batches[chainId], batch)
}

func (t *testSystemAdapter) Validate(chainID string, req *Request) ([][]*Request, [][]filter.Committer, bool) {
	r := t.reqs
	if t.reqs == nil || uint64(len(t.reqs)) == maxReqCount-uint64(1) {
		t.reqs = make([]*Request, 0, maxReqCount-1)
	}
	if uint64(len(r)) == maxReqCount-uint64(1) {
		c := [][]filter.Committer{{}}
		return [][]*Request{append(r, req)}, c, true
	}
	t.reqs = append(t.reqs, req)
	return nil, nil, true
}

func (t *testSystemAdapter) Cut(chainID string) ([]*Request, []filter.Committer) {
	r := t.reqs
	t.reqs = make([]*Request, 0, maxReqCount)
	return r, []filter.Committer{}
}

func (t *testSystemAdapter) Persist(chainId string, key string, data proto.Message) {
	compk := fmt.Sprintf("chain-%s-%s", chainId, key)
	if data == nil {
		delete(t.persistence, compk)
	} else {
		bytes, err := proto.Marshal(data)
		if err != nil {
			panic(err)
		}
		t.persistence[compk] = bytes
	}
}

func (t *testSystemAdapter) Restore(chainId string, key string, out proto.Message) bool {
	compk := fmt.Sprintf("chain-%s-%s", chainId, key)
	val, ok := t.persistence[compk]
	if !ok {
		return false
	}
	err := proto.Unmarshal(val, out)
	return (err == nil)
}

func (t *testSystemAdapter) LastBatch(chainId string) *Batch {
	if len(t.batches[chainId]) == 0 {
		return t.receivers[chainId].(*SBFT).makeBatch(0, nil, nil)
	}
	return t.batches[chainId][len(t.batches[chainId])-1]
}

func (t *testSystemAdapter) Sign(data []byte) []byte {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(crand.Reader, t.key, hash[:])
	if err != nil {
		panic(err)
	}
	sig, err := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	if err != nil {
		panic(err)
	}
	return sig
}

func (t *testSystemAdapter) CheckSig(data []byte, src uint64, sig []byte) error {
	rs := struct{ R, S *big.Int }{}
	rest, err := asn1.Unmarshal(sig, &rs)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("invalid signature")
	}
	hash := sha256.Sum256(data)
	ok := ecdsa.Verify(&t.sys.adapters[src].key.PublicKey, hash[:], rs.R, rs.S)
	if !ok {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (t *testSystemAdapter) Reconnect(chainId string, replica uint64) {
	testLog.Infof("dropping connection from %d to %d", replica, t.id)
	t.sys.queue.filter(func(e testElem) bool {
		switch e := e.ev.(type) {
		case *testMsgEvent:
			if e.dst == t.id && e.src == replica {
				return false
			}
		}
		return true
	})
	arr := t.sys.adapters[replica].arrivals[t.id] * 10
	t.sys.enqueue(arr, &testTimer{id: t.id, tf: func() {
		testLog.Infof("reconnecting %d to %d", replica, t.id)
		t.sys.adapters[replica].receivers[chainId].Connection(t.id)
	}})
}

// ==============================================

type testEvent interface {
	Exec(t *testSystem)
}

// ==============================================

type testSystem struct {
	rand          *rand.Rand
	now           time.Duration
	queue         *calendarQueue
	adapters      map[uint64]*testSystemAdapter
	filterFn      func(testElem) (testElem, bool)
	disableTimers bool
}

type testElem struct {
	at time.Duration
	ev testEvent
}

func (t testElem) String() string {
	return fmt.Sprintf("Event<%s: %s>", t.at, t.ev)
}

func newTestSystem(n uint64) *testSystem {
	return newTestSystemWithBatchSize(n, defaultMaxReqCount)
}

func newTestSystemWithBatchSize(n uint64, batchSize uint64) *testSystem {
	return newTestSystemWithParams(n, batchSize, false)
}

func newTestSystemWOTimers(n uint64) *testSystem {
	return newTestSystemWithParams(n, defaultMaxReqCount, true)
}

func newTestSystemWOTimersWithBatchSize(n uint64, batchSize uint64) *testSystem {
	return newTestSystemWithParams(n, batchSize, true)
}

func newTestSystemWithParams(n uint64, batchSize uint64, disableTimers bool) *testSystem {
	maxReqCount = batchSize
	return &testSystem{
		rand:          rand.New(rand.NewSource(0)),
		adapters:      make(map[uint64]*testSystemAdapter),
		queue:         newCalendarQueue(time.Millisecond/time.Duration(n*n), int(n*n)),
		disableTimers: disableTimers,
	}
}

func (t *testSystem) NewAdapter(id uint64) *testSystemAdapter {
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		panic(err)
	}
	a := &testSystemAdapter{
		id:          id,
		sys:         t,
		arrivals:    make(map[uint64]time.Duration),
		persistence: make(map[string][]byte),
		key:         key,
	}
	t.adapters[id] = a
	return a
}

func (t *testSystem) enqueue(d time.Duration, ev testEvent) {
	e := testElem{at: t.now + d, ev: ev}
	if t.filterFn != nil {
		var keep bool
		e, keep = t.filterFn(e)
		if !keep {
			return
		}
	}
	testLog.Debugf("enqueuing %s\n", e)
	t.queue.Add(e)
}

func (t *testSystem) Run() {
	for {
		e, ok := t.queue.Pop()
		if !ok {
			break
		}
		t.now = e.at
		testLog.Debugf("executing %s\n", e)
		e.ev.Exec(t)
	}

	testLog.Debugf("max len: %d", t.queue.maxLen)
	t.queue.maxLen = 0
}
