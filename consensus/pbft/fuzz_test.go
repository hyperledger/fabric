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
	"math/rand"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/gofuzz"
	"github.com/op/go-logging"

	"fmt"
)

func newFuzzMock() *omniProto {
	return &omniProto{
		broadcastImpl: func(msgPayload []byte) {
			// No-op
		},
		verifyImpl: func(senderID uint64, signature []byte, message []byte) error {
			return nil
		},
		signImpl: func(msg []byte) ([]byte, error) {
			return msg, nil
		},
		viewChangeImpl: func(curView uint64) {
		},
		validateStateImpl:   func() {},
		invalidateStateImpl: func() {},
	}
}

func TestFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test")
	}

	logging.SetBackend(logging.InitForTesting(logging.ERROR))

	mock := newFuzzMock()
	primary, pmanager := createRunningPbftWithManager(0, loadConfig(), mock)
	defer primary.close()
	defer pmanager.Halt()
	mock = newFuzzMock()
	backup, bmanager := createRunningPbftWithManager(1, loadConfig(), mock)
	defer backup.close()
	defer bmanager.Halt()

	f := fuzz.New()

	for i := 0; i < 30; i++ {
		msg := &Message{}
		f.Fuzz(msg)
		// roundtrip through protobufs to translate
		// nil slices into empty slices
		raw, _ := proto.Marshal(msg)
		proto.Unmarshal(raw, msg)

		var senderID uint64
		if reqBatch := msg.GetRequestBatch(); reqBatch != nil {
			senderID = primary.id // doesn't matter, not checked
		} else if preprep := msg.GetPrePrepare(); preprep != nil {
			senderID = preprep.ReplicaId
		} else if prep := msg.GetPrepare(); prep != nil {
			senderID = prep.ReplicaId
		} else if commit := msg.GetCommit(); commit != nil {
			senderID = commit.ReplicaId
		} else if chkpt := msg.GetCheckpoint(); chkpt != nil {
			senderID = chkpt.ReplicaId
		} else if vc := msg.GetViewChange(); vc != nil {
			senderID = vc.ReplicaId
		} else if nv := msg.GetNewView(); nv != nil {
			senderID = nv.ReplicaId
		}

		pmanager.Queue() <- &pbftMessageEvent{msg: msg, sender: senderID}
		bmanager.Queue() <- &pbftMessageEvent{msg: msg, sender: senderID}
	}

	logging.Reset()
}

func (msg *Message) Fuzz(c fuzz.Continue) {
	switch c.RandUint64() % 7 {
	case 0:
		m := &Message_RequestBatch{}
		c.Fuzz(m)
		msg.Payload = m
	case 1:
		m := &Message_PrePrepare{}
		c.Fuzz(m)
		msg.Payload = m
	case 2:
		m := &Message_Prepare{}
		c.Fuzz(m)
		msg.Payload = m
	case 3:
		m := &Message_Commit{}
		c.Fuzz(m)
		msg.Payload = m
	case 4:
		m := &Message_Checkpoint{}
		c.Fuzz(m)
		msg.Payload = m
	case 5:
		m := &Message_ViewChange{}
		c.Fuzz(m)
		msg.Payload = m
	case 6:
		m := &Message_NewView{}
		c.Fuzz(m)
		msg.Payload = m
	}
}

func TestMinimalFuzz(t *testing.T) {
	var err error
	if testing.Short() {
		t.Skip("Skipping fuzz test")
	}

	validatorCount := 4
	net := makePBFTNetwork(validatorCount, nil)
	defer net.stop()
	fuzzer := &protoFuzzer{r: rand.New(rand.NewSource(0))}
	net.filterFn = fuzzer.fuzzPacket

	noExec := 0
	for reqID := 1; reqID < 30; reqID++ {
		if reqID%3 == 0 {
			fuzzer.fuzzNode = fuzzer.r.Intn(len(net.endpoints))
			fmt.Printf("Fuzzing node %d\n", fuzzer.fuzzNode)
		}

		sender := uint64(generateBroadcaster(validatorCount))
		reqBatchMsg := createPbftReqBatchMsg(int64(reqID), sender)
		for _, ep := range net.endpoints {
			ep.(*pbftEndpoint).manager.Queue() <- &pbftMessageEvent{msg: reqBatchMsg, sender: sender}
		}
		if err != nil {
			t.Fatalf("Request failed: %s", err)
		}

		err = net.process()
		if err != nil {
			t.Fatalf("Processing failed: %s", err)
		}

		quorum := 0
		for _, ep := range net.endpoints {
			if ep.(*pbftEndpoint).sc.executions > 0 {
				quorum++
				ep.(*pbftEndpoint).sc.executions = 0
			}
		}
		if quorum < len(net.endpoints)/3 {
			noExec++
		}
		if noExec > 1 {
			noExec = 0
			for _, ep := range net.endpoints {
				ep.(*pbftEndpoint).pbft.sendViewChange()
			}
			err = net.process()
			if err != nil {
				t.Fatalf("Processing failed: %s", err)
			}
		}
	}
}

type protoFuzzer struct {
	fuzzNode int
	r        *rand.Rand
}

func (f *protoFuzzer) fuzzPacket(src int, dst int, msgOuter []byte) []byte {
	if dst != -1 || src != f.fuzzNode {
		return msgOuter
	}

	// XXX only with some probability
	msg := &Message{}
	if proto.Unmarshal(msgOuter, msg) != nil {
		panic("could not unmarshal")
	}

	fmt.Printf("Will fuzz %v\n", msg)

	if m := msg.GetPrePrepare(); m != nil {
		f.fuzzPayload(m)
	}
	if m := msg.GetPrepare(); m != nil {
		f.fuzzPayload(m)
	}
	if m := msg.GetCommit(); m != nil {
		f.fuzzPayload(m)
	}
	if m := msg.GetCheckpoint(); m != nil {
		f.fuzzPayload(m)
	}
	if m := msg.GetViewChange(); m != nil {
		f.fuzzPayload(m)
	}
	if m := msg.GetNewView(); m != nil {
		f.fuzzPayload(m)
	}

	newMsg, _ := proto.Marshal(msg)
	return newMsg
}

func (f *protoFuzzer) fuzzPayload(s interface{}) {
	v := reflect.ValueOf(s).Elem()
	t := v.Type()

	var elems []reflect.Value
	var fields []string
	for i := 0; i < v.NumField(); i++ {
		if t.Field(i).Name == "ReplicaId" {
			continue
		}
		elems = append(elems, v.Field(i))
		fields = append(fields, t.Field(i).Name)
	}

	idx := f.r.Intn(len(elems))
	elm := elems[idx]
	fld := fields[idx]
	fmt.Printf("Fuzzing %s:%v\n", fld, elm)
	f.Fuzz(elm)
}

func (f *protoFuzzer) Fuzz(v reflect.Value) {
	if !v.CanSet() {
		return
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.FuzzInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.FuzzUint(v)
	case reflect.String:
		str := ""
		for i := 0; i < v.Len(); i++ {
			str = str + string(' '+rune(f.r.Intn(94)))
		}
		v.SetString(str)
		return
	case reflect.Ptr:
		if !v.IsNil() {
			f.Fuzz(v.Elem())
		}
		return
	case reflect.Slice:
		mode := f.r.Intn(3)
		switch {
		case v.Len() > 0 && mode == 0:
			// fuzz entry
			f.Fuzz(v.Index(f.r.Intn(v.Len())))
		case v.Len() > 0 && mode == 1:
			// remove entry
			entry := f.r.Intn(v.Len())
			pre := v.Slice(0, entry)
			post := v.Slice(entry+1, v.Len())
			v.Set(reflect.AppendSlice(pre, post))
		default:
			// add entry
			entry := reflect.MakeSlice(v.Type(), 1, 1)
			f.Fuzz(entry) // XXX fill all fields
			v.Set(reflect.AppendSlice(v, entry))
		}
		return
	case reflect.Struct:
		f.Fuzz(v.Field(f.r.Intn(v.NumField())))
		return
	case reflect.Map:
		// TODO fuzz map
	default:
		panic(fmt.Sprintf("Not fuzzing %v %+v", v.Kind(), v))
	}
}

func (f *protoFuzzer) FuzzInt(v reflect.Value) {
	v.SetInt(v.Int() + f.fuzzyInt())
}

func (f *protoFuzzer) FuzzUint(v reflect.Value) {
	val := v.Uint()
	for {
		delta := f.fuzzyInt()
		if delta > 0 || uint64(-delta) < val {
			v.SetUint(val + uint64(delta))
			return
		}
	}
}

func (f *protoFuzzer) fuzzyInt() int64 {
	i := int64(rand.NewZipf(f.r, 3, 1, 200).Uint64() + 1)
	if rand.Intn(2) == 0 {
		i = -i
	}
	fmt.Printf("Changing int by %d\n", i)
	return i
}

func (f *protoFuzzer) FuzzSlice(v reflect.Value) {
}
