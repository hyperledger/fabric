/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

type mockStream struct {
	grpc.ServerStream
}

func (mockStream) Context() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{})
}

type mockB struct {
	mockStream
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

func (m *erroneousRecvMockB) Context() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{})
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
	mockStream
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

type mockSupportManager struct {
	MsgProcessorIsConfig bool
	MsgProcessorVal      *mockSupport
	MsgProcessorErr      error
	ChdrVal              *cb.ChannelHeader
}

func (mm *mockSupportManager) BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, ChannelSupport, error) {
	return mm.ChdrVal, mm.MsgProcessorIsConfig, mm.MsgProcessorVal, mm.MsgProcessorErr
}

type mockSupport struct {
	ProcessConfigEnv *cb.Envelope
	ProcessConfigSeq uint64
	ProcessErr       error
	rejectEnqueue    bool
}

func (ms *mockSupport) WaitReady() error {
	return nil
}

// Order sends a message for ordering
func (ms *mockSupport) Order(env *cb.Envelope, configSeq uint64) error {
	if ms.rejectEnqueue {
		return fmt.Errorf("Reject")
	}
	return nil
}

// Configure sends a reconfiguration message for ordering
func (ms *mockSupport) Configure(config *cb.Envelope, configSeq uint64) error {
	return ms.Order(config, configSeq)
}

func (ms *mockSupport) ClassifyMsg(chdr *cb.ChannelHeader) msgprocessor.Classification {
	panic("UNIMPLMENTED")
}

func (ms *mockSupport) ProcessNormalMsg(msg *cb.Envelope) (uint64, error) {
	return ms.ProcessConfigSeq, ms.ProcessErr
}

func (ms *mockSupport) ProcessConfigUpdateMsg(msg *cb.Envelope) (*cb.Envelope, uint64, error) {
	return ms.ProcessConfigEnv, ms.ProcessConfigSeq, ms.ProcessErr
}

func (ms *mockSupport) ProcessConfigMsg(msg *cb.Envelope) (*cb.Envelope, uint64, error) {
	return ms.ProcessConfigEnv, ms.ProcessConfigSeq, ms.ProcessErr
}

func getMockSupportManager() *mockSupportManager {
	return &mockSupportManager{
		MsgProcessorVal: &mockSupport{},
		ChdrVal:         &cb.ChannelHeader{},
	}
}

func TestEnqueueFailure(t *testing.T) {
	mm := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		m.recvChan <- nil
		reply := <-m.sendChan
		if reply.Status != cb.Status_SUCCESS {
			t.Fatalf("Should have successfully queued the message")
		}
	}

	mm.MsgProcessorVal.rejectEnqueue = true
	m.recvChan <- nil
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

func TestClassifyError(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		assert.Equal(t, cb.Status_NOT_FOUND, ClassifyError(msgprocessor.ErrChannelDoesNotExist))
	})
	t.Run("Forbidden", func(t *testing.T) {
		assert.Equal(t, cb.Status_FORBIDDEN, ClassifyError(msgprocessor.ErrPermissionDenied))
	})
	t.Run("WrappedErr", func(t *testing.T) {
		assert.Equal(t, cb.Status_NOT_FOUND, ClassifyError(errors.Wrap(msgprocessor.ErrChannelDoesNotExist, "A wrapped error")))
	})
	t.Run("DefaultBadReq", func(t *testing.T) {
		assert.Equal(t, cb.Status_BAD_REQUEST, ClassifyError(fmt.Errorf("Foo")))
	})
}

func TestBadChannelId(t *testing.T) {
	mm := getMockSupportManager()
	mm.MsgProcessorVal = &mockSupport{ProcessErr: msgprocessor.ErrChannelDoesNotExist}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	m.recvChan <- nil
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
	mm := getMockSupportManager()
	mm.MsgProcessorIsConfig = true
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- nil
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_SUCCESS, reply.Status, "Should have allowed a good CONFIG_UPDATE")
}

func TestBadConfigUpdate(t *testing.T) {
	mm := getMockSupportManager()
	mm.MsgProcessorIsConfig = true
	mm.MsgProcessorVal.ProcessErr = fmt.Errorf("Error")
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- nil
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
	mm := &mockSupportManager{
		MsgProcessorVal: &mockSupport{ProcessErr: fmt.Errorf("Reject")},
		ChdrVal:         &cb.ChannelHeader{},
	}
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	go bh.Handle(m)

	m.recvChan <- nil
	reply := <-m.sendChan
	assert.Equal(t, cb.Status_BAD_REQUEST, reply.Status, "Should have rejected CONFIG_UPDATE")
	assert.Equal(t, mm.MsgProcessorVal.ProcessErr.Error(), reply.Info, "Should have rejected CONFIG_UPDATE")
}

func TestBadStreamRecv(t *testing.T) {
	bh := NewHandlerImpl(nil)
	assert.Error(t, bh.Handle(&erroneousRecvMockB{}), "Should catch unexpected stream error")
}

func TestBadStreamSend(t *testing.T) {
	mm := getMockSupportManager()
	bh := NewHandlerImpl(mm)
	m := &erroneousSendMockB{recvVal: nil}
	assert.Error(t, bh.Handle(m), "Should catch unexpected stream error")
}

func TestMalformedHeader(t *testing.T) {
	mm := getMockSupportManager()
	mm.ChdrVal = nil
	mm.MsgProcessorErr = errors.New("Mocked Error")
	bh := NewHandlerImpl(mm)
	m := newMockB()
	defer close(m.recvChan)
	done := make(chan struct{})
	go func() {
		bh.Handle(m)
		close(done)
	}()

	m.recvChan <- nil
	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected message for malformed header")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Should have terminated the stream")
	}
}
