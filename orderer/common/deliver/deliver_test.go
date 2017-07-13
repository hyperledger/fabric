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

package deliver

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/ledger"
	ramledger "github.com/hyperledger/fabric/orderer/ledger/ram"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var genesisBlock = cb.NewBlock(0, nil)

var systemChainID = "systemChain"

const ledgerSize = 10

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type mockD struct {
	grpc.ServerStream
	recvChan chan *cb.Envelope
	sendChan chan *ab.DeliverResponse
}

func newMockD() *mockD {
	return &mockD{
		recvChan: make(chan *cb.Envelope),
		sendChan: make(chan *ab.DeliverResponse),
	}
}

func (m *mockD) Send(br *ab.DeliverResponse) error {
	m.sendChan <- br
	return nil
}

func (m *mockD) Recv() (*cb.Envelope, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return msg, io.EOF
	}
	return msg, nil
}

type erroneousRecvMockD struct {
	grpc.ServerStream
}

func (m *erroneousRecvMockD) Send(br *ab.DeliverResponse) error {
	return nil
}

func (m *erroneousRecvMockD) Recv() (*cb.Envelope, error) {
	// The point here is to simulate an error other than EOF.
	// We don't bother to create a new custom error type.
	return nil, io.ErrUnexpectedEOF
}

type erroneousSendMockD struct {
	grpc.ServerStream
	recvVal *cb.Envelope
}

func (m *erroneousSendMockD) Send(br *ab.DeliverResponse) error {
	// The point here is to simulate an error other than EOF.
	// We don't bother to create a new custom error type.
	return io.ErrUnexpectedEOF
}

func (m *erroneousSendMockD) Recv() (*cb.Envelope, error) {
	return m.recvVal, nil
}

type mockSupportManager struct {
	chains map[string]*mockSupport
}

func (mm *mockSupportManager) GetChain(chainID string) (Support, bool) {
	cs, ok := mm.chains[chainID]
	return cs, ok
}

type mockSupport struct {
	ledger        ledger.ReadWriter
	policyManager *mockpolicies.Manager
	erroredChan   chan struct{}
	configSeq     uint64
}

func (mcs *mockSupport) Errored() <-chan struct{} {
	return mcs.erroredChan
}

func (mcs *mockSupport) Sequence() uint64 {
	return mcs.configSeq
}

func (mcs *mockSupport) PolicyManager() policies.Manager {
	return mcs.policyManager
}

func (mcs *mockSupport) Reader() ledger.Reader {
	return mcs.ledger
}

func NewRAMLedger() ledger.ReadWriter {
	rlf := ramledger.New(ledgerSize + 1)
	rl, _ := rlf.GetOrCreate(provisional.TestChainID)
	rl.Append(genesisBlock)
	return rl
}

func initializeDeliverHandler() Handler {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		l := mm.chains[systemChainID].ledger
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	return NewHandlerImpl(mm)
}

func newMockMultichainManager() *mockSupportManager {
	rl := NewRAMLedger()
	mm := &mockSupportManager{
		chains: make(map[string]*mockSupport),
	}
	mm.chains[systemChainID] = &mockSupport{
		ledger:        rl,
		policyManager: &mockpolicies.Manager{Policy: &mockpolicies.Policy{}},
		erroredChan:   make(chan struct{}),
	}
	return mm
}

var seekOldest = &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}}
var seekNewest = &ab.SeekPosition{Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}}}

func seekSpecified(number uint64) *ab.SeekPosition {
	return &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: number}}}
}

func makeSeek(chainID string, seekInfo *ab.SeekInfo) *cb.Envelope {
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: chainID,
				}),
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
			Data: utils.MarshalOrPanic(seekInfo),
		}),
	}
}

func TestWholeChainSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekOldest, Stop: seekNewest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	count := uint64(0)
	for {
		select {
		case deliverReply := <-m.sendChan:
			if deliverReply.GetBlock() == nil {
				if deliverReply.GetStatus() != cb.Status_SUCCESS {
					t.Fatalf("Received an error on the reply channel")
				}
				if count != ledgerSize {
					t.Fatalf("Expected %d blocks but got %d", ledgerSize, count)
				}
				return
			}
			if deliverReply.GetBlock().Header.Number != count {
				t.Fatalf("Expected block %d but got block %d", count, deliverReply.GetBlock().Header.Number)
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
	}
}

func TestNewestSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekNewest, Stop: seekNewest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetBlock() == nil {
			t.Fatalf("Received an error on the reply channel")
		}
		if deliverReply.GetBlock().Header.Number != uint64(ledgerSize-1) {
			t.Fatalf("Expected only the most recent block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestSpecificSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	specifiedStart := uint64(3)
	specifiedStop := uint64(7)
	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(specifiedStart), Stop: seekSpecified(specifiedStop), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	count := uint64(0)
	for {
		select {
		case deliverReply := <-m.sendChan:
			if deliverReply.GetBlock() == nil {
				if deliverReply.GetStatus() != cb.Status_SUCCESS {
					t.Fatalf("Received an error on the reply channel")
				}
				return
			}
			if expected := specifiedStart + count; deliverReply.GetBlock().Header.Number != expected {
				t.Fatalf("Expected block %d but got block %d", expected, deliverReply.GetBlock().Header.Number)
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
	}
}

func TestUnauthorizedSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		l := mm.chains[systemChainID].ledger
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}
	mm.chains[systemChainID].policyManager.Policy.Err = fmt.Errorf("Fail to evaluate policy")

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(0)), Stop: seekSpecified(uint64(0)), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetStatus() != cb.Status_FORBIDDEN {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestRevokedAuthorizationSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		l := mm.chains[systemChainID].ledger
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(ledgerSize - 1)), Stop: seekSpecified(ledgerSize), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		assert.NotNil(t, deliverReply.GetBlock(), "First should succeed")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}

	mm.chains[systemChainID].policyManager.Policy.Err = fmt.Errorf("Fail to evaluate policy")
	mm.chains[systemChainID].configSeq++
	l := mm.chains[systemChainID].ledger
	l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", ledgerSize+1))}}))

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_FORBIDDEN, deliverReply.GetStatus(), "Second should been forbidden ")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}

}

func TestOutOfBoundSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(3 * ledgerSize)), Stop: seekSpecified(uint64(3 * ledgerSize)), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetStatus() != cb.Status_NOT_FOUND {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestFailFastSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(ledgerSize - 1)), Stop: seekSpecified(ledgerSize), Behavior: ab.SeekInfo_FAIL_IF_NOT_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetBlock() == nil {
			t.Fatalf("Expected to receive first block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetStatus() != cb.Status_NOT_FOUND {
			t.Fatalf("Expected to receive failure for second block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBlockingSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		l := mm.chains[systemChainID].ledger
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(ledgerSize - 1)), Stop: seekSpecified(ledgerSize), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetBlock() == nil {
			t.Fatalf("Expected to receive first block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get first block")
	}

	select {
	case <-m.sendChan:
		t.Fatalf("Should not have delivered an error or second block")
	case <-time.After(50 * time.Millisecond):
	}

	l := mm.chains[systemChainID].ledger
	l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", ledgerSize+1))}}))

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetBlock() == nil {
			t.Fatalf("Expected to receive new block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get new block")
	}

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetStatus() != cb.Status_SUCCESS {
			t.Fatalf("Expected delivery to complete")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestErroredSeek(t *testing.T) {
	mm := newMockMultichainManager()
	ms := mm.chains[systemChainID]
	l := ms.ledger
	close(ms.erroredChan)
	for i := 1; i < ledgerSize; i++ {
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(ledgerSize - 1)), Stop: seekSpecified(ledgerSize), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_SERVICE_UNAVAILABLE, deliverReply.GetStatus(), "Mock support errored")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting for error response")
	}
}

func TestErroredBlockingSeek(t *testing.T) {
	mm := newMockMultichainManager()
	ms := mm.chains[systemChainID]
	l := ms.ledger
	for i := 1; i < ledgerSize; i++ {
		l.Append(ledger.CreateNextBlock(l, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(uint64(ledgerSize - 1)), Stop: seekSpecified(ledgerSize), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		assert.NotNil(t, deliverReply.GetBlock(), "Expected first block")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get first block")
	}

	close(ms.erroredChan)

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_SERVICE_UNAVAILABLE, deliverReply.GetStatus(), "Mock support errored")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting for error response")
	}
}

func TestSGracefulShutdown(t *testing.T) {
	m := newMockD()
	ds := NewHandlerImpl(nil)

	close(m.recvChan)
	assert.NoError(t, ds.Handle(m), "Expected no error for hangup")
}

func TestReversedSeqSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	specifiedStart := uint64(7)
	specifiedStop := uint64(3)
	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekSpecified(specifiedStart), Stop: seekSpecified(specifiedStop), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetStatus() != cb.Status_BAD_REQUEST {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadStreamRecv(t *testing.T) {
	bh := NewHandlerImpl(nil)
	assert.Error(t, bh.Handle(&erroneousRecvMockD{}), "Should catch unexpected stream error")
}

func TestBadStreamSend(t *testing.T) {
	m := &erroneousSendMockD{recvVal: makeSeek(systemChainID, &ab.SeekInfo{Start: seekNewest, Stop: seekNewest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})}
	ds := initializeDeliverHandler()
	assert.Error(t, ds.Handle(m), "Should catch unexpected stream error")
}

func TestOldestSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekOldest, Stop: seekOldest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		assert.NotEqual(t, nil, deliverReply.GetBlock(), "Received an error on the reply channel")
		assert.Equal(t, uint64(0), deliverReply.GetBlock().Header.Number, "Expected only the most recent block")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestNoPayloadSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: []byte("Foo")}

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_BAD_REQUEST, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestNilPayloadHeaderSeek(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{})}

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_BAD_REQUEST, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadChannelHeader(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{
		Header: &cb.Header{ChannelHeader: []byte("Foo")},
	})}

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_BAD_REQUEST, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestChainNotFound(t *testing.T) {
	mm := &mockSupportManager{
		chains: make(map[string]*mockSupport),
	}

	m := newMockD()
	defer close(m.recvChan)

	ds := NewHandlerImpl(mm)
	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekNewest, Stop: seekNewest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_NOT_FOUND, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadSeekInfoPayload(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: systemChainID,
				}),
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
			Data: []byte("Foo"),
		}),
	}

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_BAD_REQUEST, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestMissingSeekPosition(t *testing.T) {
	m := newMockD()
	defer close(m.recvChan)

	ds := initializeDeliverHandler()
	go ds.Handle(m)

	m.recvChan <- &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: systemChainID,
				}),
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
			Data: nil,
		}),
	}

	select {
	case deliverReply := <-m.sendChan:
		assert.Equal(t, cb.Status_BAD_REQUEST, deliverReply.GetStatus(), "Received wrong error on the reply channel")
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}
