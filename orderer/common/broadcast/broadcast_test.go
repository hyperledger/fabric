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
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

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
		return msg, io.EOF
	}
	return msg, nil
}

type erroneousRecvMockB struct {
	grpc.ServerStream
}

func (m *erroneousRecvMockB) Send(br *ab.BroadcastResponse) error {
	return nil
}

func (m *erroneousRecvMockB) Recv() (*cb.Envelope, error) {
	// The point here is to simulate an error other than EOF.
	// We don't bother to create a new custom error type.
	return nil, io.ErrUnexpectedEOF
}

type erroneousSendMockB struct {
	grpc.ServerStream
	recvVal *cb.Envelope
}

func (m *erroneousSendMockB) Send(br *ab.BroadcastResponse) error {
	// The point here is to simulate an error other than EOF.
	// We don't bother to create a new custom error type.
	return io.ErrUnexpectedEOF
}

func (m *erroneousSendMockB) Recv() (*cb.Envelope, error) {
	return m.recvVal, nil
}

var RejectRule = filter.Rule(rejectRule{})

type rejectRule struct{}

func (r rejectRule) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	return filter.Reject, nil
}

type mockSupportManager struct {
	chains     map[string]*mockSupport
	ProcessVal *cb.Envelope
}

func (mm *mockSupportManager) GetChain(chainID string) (Support, bool) {
	chain, ok := mm.chains[chainID]
	return chain, ok
}

func (mm *mockSupportManager) Process(configTx *cb.Envelope) (*cb.Envelope, error) {
	if mm.ProcessVal == nil {
		return nil, fmt.Errorf("Nil result implies error")
	}
	return mm.ProcessVal, nil
}

type mockSupport struct {
	filters       *filter.RuleSet
	rejectEnqueue bool
}

func (ms *mockSupport) Filters() *filter.RuleSet {
	return ms.filters
}

// Enqueue sends a message for ordering
func (ms *mockSupport) Enqueue(env *cb.Envelope) bool {
	return !ms.rejectEnqueue
}

func makeConfigMessage(chainID string) *cb.Envelope {
	payload := &cb.Payload{
		Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{}),
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: chainID,
				Type:      int32(cb.HeaderType_CONFIG_UPDATE),
			}),
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
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: chainID,
			}),
		},
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}

func getMockSupportManager() (*mockSupportManager, *mockSupport) {
	filters := filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		filter.AcceptRule,
	})
	mm := &mockSupportManager{
		chains: make(map[string]*mockSupport),
	}
	mSysChain := &mockSupport{
		filters: filters,
	}
	mm.chains[string(systemChain)] = mSysChain
	return mm, mSysChain
}

func TestEnqueueFailure(t *testing.T) {
	mm, mSysChain := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
		reply := <-m.sendChan
		if reply.Status != cb.Status_SUCCESS {
			t.Fatalf("Should have successfully queued the message")
		}
	}

	mSysChain.rejectEnqueue = true
	m.recvChan <- makeMessage(systemChain, []byte("Some bytes"))
	reply := <-m.sendChan
	if reply.Status != cb.Status_SERVICE_UNAVAILABLE {
		t.Fatalf("Should not have successfully queued the message")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Should have terminated the stream")
	}
}

func TestEmptyEnvelope(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	m.recvChan <- &cb.Envelope{}
	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected the null message")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Should have terminated the stream")
	}
}

func TestBadChannelId(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	m.recvChan <- makeMessage("Wrong chain", []byte("Some bytes"))
	reply := <-m.sendChan
	if reply.Status != cb.Status_NOT_FOUND {
		t.Fatalf("Should have rejected message to a chain which does not exist")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Should have terminated the stream")
	}
}

func TestGoodConfigUpdate(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{ChannelId: systemChain})}})}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)
	newChannelId := "New Chain"

	m.recvChan <- makeConfigMessage(newChannelId)
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_SUCCESS, reply.Status, "Should have allowed a good CONFIG_UPDATE")
}

func TestBadConfigUpdate(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage(systemChain)
	reply := <-m.sendChan
	assert.NotEqual(t, cb.Status_SUCCESS, reply.Status, "Should have rejected CONFIG_UPDATE")
}

func TestGracefulShutdown(t *testing.T) {
	bh := NewHandlerImpl(nil)
	m := newMockB()
	close(m.recvChan)
	assert.NoError(t, bh.Handle(m), "Should exit normally upon EOF")
}

func TestRejected(t *testing.T) {
	filters := filter.NewRuleSet([]filter.Rule{RejectRule})
	mm := &mockSupportManager{
		chains: map[string]*mockSupport{string(systemChain): {filters: filters}},
	}
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{ChannelId: systemChain})}})}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	newChannelId := "New Chain"

	m.recvChan <- makeConfigMessage(newChannelId)
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_BAD_REQUEST, reply.Status, "Should have rejected CONFIG_UPDATE")
}

func TestBadStreamRecv(t *testing.T) {
	bh := NewHandlerImpl(nil)
	assert.Error(t, bh.Handle(&erroneousRecvMockB{}), "Should catch unexpected stream error")
}

func TestBadStreamSend(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{ChannelId: systemChain})}})}
	bh := NewHandlerImpl(mm)
	m := &erroneousSendMockB{recvVal: makeConfigMessage("New Chain")}
	assert.Error(t, bh.Handle(m), "Should catch unexpected stream error")
}

func TestMalformedEnvelope(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: []byte("foo")}
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_BAD_REQUEST, reply.Status, "Should have rejected the malformed message")
}

func TestMissingHeader(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{})}
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_BAD_REQUEST, reply.Status, "Should have rejected the payload without header")
}

func TestBadChannelHeader(t *testing.T) {
	mm, _ := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: []byte("foo")}})}
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_BAD_REQUEST, reply.Status, "Should have rejected bad header")
}

func TestBadPayloadAfterProcessing(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: []byte("foo")}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage("New Chain")
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_INTERNAL_SERVER_ERROR, reply.Status, "Should respond with internal server error")
}

func TestNilHeaderAfterProcessing(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{})}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage("New Chain")
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_INTERNAL_SERVER_ERROR, reply.Status, "Should respond with internal server error")
}

func TestBadChannelHeaderAfterProcessing(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: []byte("foo")}})}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage("New Chain")
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_INTERNAL_SERVER_ERROR, reply.Status, "Should respond with internal server error")
}

func TestEmptyChannelIDAfterProcessing(t *testing.T) {
	mm, _ := getMockSupportManager()
	mm.ProcessVal = &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{})}})}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- makeConfigMessage("New Chain")
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_INTERNAL_SERVER_ERROR, reply.Status, "Should respond with internal server error")
}
