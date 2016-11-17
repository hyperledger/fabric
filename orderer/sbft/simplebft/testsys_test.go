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
)

type testSystemAdapter struct {
	id       uint64
	sys      *testSystem
	receiver Receiver

	batches     []*Batch
	arrivals    map[uint64]time.Duration
	persistence map[string][]byte

	key *ecdsa.PrivateKey
}

func (t *testSystemAdapter) SetReceiver(recv Receiver) {
	if t.receiver != nil {
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

	t.receiver = recv
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

func (t *testSystemAdapter) Send(msg *Msg, dest uint64) {
	arr := t.getArrival(dest)
	ev := &testMsgEvent{
		inflight: arr,
		src:      t.id,
		dst:      dest,
		msg:      msg,
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
}

func (ev *testMsgEvent) Exec(t *testSystem) {
	r := t.adapters[ev.dst]
	if r == nil {
		testLog.Errorf("message to non-existing %s", ev)
		return
	}
	r.receiver.Receive(ev.msg, ev.src)
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
	t.sys.enqueue(d, tt)
	return tt
}

func (t *testSystemAdapter) Deliver(batch *Batch) {
	t.batches = append(t.batches, batch)
}

func (t *testSystemAdapter) Persist(key string, data proto.Message) {
	if data == nil {
		delete(t.persistence, key)
	} else {
		bytes, err := proto.Marshal(data)
		if err != nil {
			panic(err)
		}
		t.persistence[key] = bytes
	}
}

func (t *testSystemAdapter) Restore(key string, out proto.Message) bool {
	val, ok := t.persistence[key]
	if !ok {
		return false
	}
	err := proto.Unmarshal(val, out)
	return (err == nil)
}

func (t *testSystemAdapter) LastBatch() *Batch {
	if len(t.batches) == 0 {
		return t.receiver.(*SBFT).makeBatch(0, nil, nil)
	} else {
		return t.batches[len(t.batches)-1]
	}
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

func (t *testSystemAdapter) Reconnect(replica uint64) {
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
		t.sys.adapters[replica].receiver.Connection(t.id)
	}})
}

// ==============================================

type testEvent interface {
	Exec(t *testSystem)
}

// ==============================================

type testSystem struct {
	rand     *rand.Rand
	now      time.Duration
	queue    *calendarQueue
	adapters map[uint64]*testSystemAdapter
	filterFn func(testElem) (testElem, bool)
}

type testElem struct {
	at time.Duration
	ev testEvent
}

func (t testElem) String() string {
	return fmt.Sprintf("Event<%s: %s>", t.at, t.ev)
}

func newTestSystem(n uint64) *testSystem {
	return &testSystem{
		rand:     rand.New(rand.NewSource(0)),
		adapters: make(map[uint64]*testSystemAdapter),
		queue:    newCalendarQueue(time.Millisecond/time.Duration(n*n), int(n*n)),
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
