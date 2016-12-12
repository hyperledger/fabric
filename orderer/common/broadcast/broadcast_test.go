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
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"google.golang.org/grpc"
)

var systemChain = "systemChain"

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

type mockSupportManager struct {
	chains map[string]*mockSupport
}

func (mm *mockSupportManager) GetChain(chainID string) (Support, bool) {
	chain, ok := mm.chains[chainID]
	return chain, ok
}

func (mm *mockSupportManager) ProposeChain(configTx *cb.Envelope) cb.Status {
	payload := utils.ExtractPayloadOrPanic(configTx)

	mm.chains[string(payload.Header.ChainHeader.ChainID)] = &mockSupport{
		filters: filter.NewRuleSet([]filter.Rule{
			filter.EmptyRejectRule,
			filter.AcceptRule,
		}),
		queue: make(chan *cb.Envelope),
	}
	return cb.Status_SUCCESS
}

func (mm *mockSupportManager) halt() {
	for _, chain := range mm.chains {
		chain.halt()
	}
}

type mockSupport struct {
	filters *filter.RuleSet
	queue   chan *cb.Envelope
	done    bool
}

func (ms *mockSupport) Filters() *filter.RuleSet {
	return ms.filters
}

// Enqueue sends a message for ordering
func (ms *mockSupport) Enqueue(env *cb.Envelope) bool {
	ms.queue <- env
	return !ms.done
}

func (ms *mockSupport) halt() {
	ms.done = true
	select {
	case <-ms.queue:
	default:
	}
}

func makeConfigMessage(chainID string) *cb.Envelope {
	payload := &cb.Payload{
		Data: utils.MarshalOrPanic(&cb.ConfigurationEnvelope{}),
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				ChainID: chainID,
				Type:    int32(cb.HeaderType_CONFIGURATION_TRANSACTION),
			},
		},
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}

func makeMessage(chainID string, data []byte) *cb.Envelope {
	payload := &cb.Payload{
		Data: data,
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				ChainID: chainID,
			},
		},
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}

func getMultichainManager() *mockSupportManager {
	filters := filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		filter.AcceptRule,
	})
	mm := &mockSupportManager{
		chains: make(map[string]*mockSupport),
	}
	mm.chains[string(systemChain)] = &mockSupport{
		filters: filters,
		queue:   make(chan *cb.Envelope),
	}
	return mm
}

func TestQueueOverflow(t *testing.T) {
	mm := getMultichainManager()
	defer mm.halt()
	bh := NewHandlerImpl(mm, 2)
	m := newMockB()
	defer close(m.recvChan)
	b := newBroadcaster(bh.(*handlerImpl))
	go b.queueEnvelopes(m)

	for i := 0; i < 2; i++ {
		m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
		reply := <-m.sendChan
		if reply.Status != cb.Status_SUCCESS {
			t.Fatalf("Should have successfully queued the message")
		}
	}

	m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
	reply := <-m.sendChan
	if reply.Status != cb.Status_SERVICE_UNAVAILABLE {
		t.Fatalf("Should not have successfully queued the message")
	}

}

func TestMultiQueueOverflow(t *testing.T) {
	mm := getMultichainManager()
	defer mm.halt()
	bh := NewHandlerImpl(mm, 2)
	ms := []*mockB{newMockB(), newMockB(), newMockB()}

	for _, m := range ms {
		defer close(m.recvChan)
		b := newBroadcaster(bh.(*handlerImpl))
		go b.queueEnvelopes(m)
	}

	for _, m := range ms {
		for i := 0; i < 2; i++ {
			m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
			reply := <-m.sendChan
			if reply.Status != cb.Status_SUCCESS {
				t.Fatalf("Should have successfully queued the message")
			}
		}
	}

	for _, m := range ms {
		m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
		reply := <-m.sendChan
		if reply.Status != cb.Status_SERVICE_UNAVAILABLE {
			t.Fatalf("Should not have successfully queued the message")
		}
	}
}

func TestEmptyEnvelope(t *testing.T) {
	mm := getMultichainManager()
	defer mm.halt()
	bh := NewHandlerImpl(mm, 2)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{}
	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected the null message")
	}

}

func TestBadChainID(t *testing.T) {
	mm := getMultichainManager()
	defer mm.halt()
	bh := NewHandlerImpl(mm, 2)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeMessage("Wrong chain", []byte("Some bytes"))
	reply := <-m.sendChan
	if reply.Status != cb.Status_NOT_FOUND {
		t.Fatalf("Should have rejected message to a chain which does not exist")
	}

}

func TestNewChainID(t *testing.T) {
	mm := getMultichainManager()
	defer mm.halt()
	bh := NewHandlerImpl(mm, 2)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage("New chain")
	reply := <-m.sendChan
	if reply.Status != cb.Status_SUCCESS {
		t.Fatalf("Should have created a new chain, got %d", reply.Status)
	}

	if len(mm.chains) != 2 {
		t.Fatalf("Should have created a new chain")
	}

	m.recvChan <- makeMessage("New chain", []byte("Some bytes"))
	reply = <-m.sendChan
	if reply.Status != cb.Status_SUCCESS {
		t.Fatalf("Should have successfully sent message to new chain")
	}
}
