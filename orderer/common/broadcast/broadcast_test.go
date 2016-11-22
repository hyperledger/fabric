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

package broadcast

import (
	"bytes"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var configTx []byte

func init() {
	var err error
	configTx, err = proto.Marshal(&cb.ConfigurationEnvelope{})
	if err != nil {
		panic("Error marshaling empty config tx")
	}
}

type mockConfigManager struct {
	validated   bool
	applied     bool
	validateErr error
	applyErr    error
}

func (mcm *mockConfigManager) Validate(configtx *cb.ConfigurationEnvelope) error {
	mcm.validated = true
	return mcm.validateErr
}

func (mcm *mockConfigManager) Apply(message *cb.ConfigurationEnvelope) error {
	mcm.applied = true
	return mcm.applyErr
}

func (mcm *mockConfigManager) ChainID() []byte {
	panic("Unimplemented")
}

type mockConfigFilter struct {
	manager configtx.Manager
}

func (mcf *mockConfigFilter) Apply(msg *cb.Envelope) broadcastfilter.Action {
	if bytes.Equal(msg.Payload, configTx) {
		if mcf.manager == nil || mcf.manager.Validate(nil) != nil {
			return broadcastfilter.Reject
		}
		return broadcastfilter.Reconfigure
	}
	return broadcastfilter.Forward
}

type mockTarget struct {
	queue chan *cb.Envelope
	done  bool
}

func (mt *mockTarget) Enqueue(env *cb.Envelope) bool {
	mt.queue <- env
	return !mt.done
}

func (mt *mockTarget) halt() {
	mt.done = true
	select {
	case <-mt.queue:
	default:
	}
}

type mockB struct {
	grpc.ServerStream
	recvChan chan *cb.Envelope
	sendChan chan *ab.BroadcastResponse
}

func newMockB() *mockB {
	return &mockB{
		recvChan: make(chan *cb.Envelope),
		sendChan: make(chan *ab.BroadcastResponse),
	}
}

func (m *mockB) Send(br *ab.BroadcastResponse) error {
	m.sendChan <- br
	return nil
}

func (m *mockB) Recv() (*cb.Envelope, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return msg, fmt.Errorf("Channel closed")
	}
	return msg, nil
}

func getFiltersConfigMockTarget() (*broadcastfilter.RuleSet, *mockConfigManager, *mockTarget) {
	cm := &mockConfigManager{}
	filters := broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		&mockConfigFilter{cm},
		broadcastfilter.AcceptRule,
	})
	mt := &mockTarget{queue: make(chan *cb.Envelope)}
	return filters, cm, mt

}

func TestQueueOverflow(t *testing.T) {
	filters, cm, mt := getFiltersConfigMockTarget()
	defer mt.halt()
	bh := NewHandlerImpl(2, mt, filters, cm)
	m := newMockB()
	defer close(m.recvChan)
	b := newBroadcaster(bh.(*handlerImpl))
	go b.queueEnvelopes(m)

	for i := 0; i < 2; i++ {
		m.recvChan <- &cb.Envelope{Payload: []byte("Some bytes")}
		reply := <-m.sendChan
		if reply.Status != cb.Status_SUCCESS {
			t.Fatalf("Should have successfully queued the message")
		}
	}

	m.recvChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	reply := <-m.sendChan
	if reply.Status != cb.Status_SERVICE_UNAVAILABLE {
		t.Fatalf("Should not have successfully queued the message")
	}

}

func TestMultiQueueOverflow(t *testing.T) {
	filters, cm, mt := getFiltersConfigMockTarget()
	defer mt.halt()
	bh := NewHandlerImpl(2, mt, filters, cm)
	ms := []*mockB{newMockB(), newMockB(), newMockB()}

	for _, m := range ms {
		defer close(m.recvChan)
		b := newBroadcaster(bh.(*handlerImpl))
		go b.queueEnvelopes(m)
	}

	for _, m := range ms {
		for i := 0; i < 2; i++ {
			m.recvChan <- &cb.Envelope{Payload: []byte("Some bytes")}
			reply := <-m.sendChan
			if reply.Status != cb.Status_SUCCESS {
				t.Fatalf("Should have successfully queued the message")
			}
		}
	}

	for _, m := range ms {
		m.recvChan <- &cb.Envelope{Payload: []byte("Some bytes")}
		reply := <-m.sendChan
		if reply.Status != cb.Status_SERVICE_UNAVAILABLE {
			t.Fatalf("Should not have successfully queued the message")
		}
	}
}

func TestEmptyEnvelope(t *testing.T) {
	filters, cm, mt := getFiltersConfigMockTarget()
	defer mt.halt()
	bh := NewHandlerImpl(2, mt, filters, cm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{}
	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected the null message")
	}

}

func TestReconfigureAccept(t *testing.T) {
	filters, cm, mt := getFiltersConfigMockTarget()
	defer mt.halt()
	bh := NewHandlerImpl(2, mt, filters, cm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: configTx}

	reply := <-m.sendChan
	if reply.Status != cb.Status_SUCCESS {
		t.Fatalf("Should have successfully queued the message")
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}
}

func TestReconfigureReject(t *testing.T) {
	filters, cm, mt := getFiltersConfigMockTarget()
	cm.validateErr = fmt.Errorf("Fail to validate")
	defer mt.halt()
	bh := NewHandlerImpl(2, mt, filters, cm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: configTx}

	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have failed to queue the message because it was invalid config")
	}
}
