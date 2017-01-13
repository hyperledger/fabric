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
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	ramledger "github.com/hyperledger/fabric/orderer/ledger/ram"
	"github.com/hyperledger/fabric/orderer/localconfig"
	mockpolicies "github.com/hyperledger/fabric/orderer/mocks/policies"
	mocksharedconfig "github.com/hyperledger/fabric/orderer/mocks/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
	"google.golang.org/grpc"
)

var genesisBlock *cb.Block

var systemChainID = "systemChain"

const ledgerSize = 10

func init() {
	logging.SetLevel(logging.DEBUG, "")
	genesisBlock = provisional.New(config.Load()).GenesisBlock()
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
		return msg, fmt.Errorf("Channel closed")
	}
	return msg, nil
}

type mockSupportManager struct {
	chains map[string]*mockSupport
}

func (mm *mockSupportManager) GetChain(chainID string) (Support, bool) {
	cs, ok := mm.chains[chainID]
	return cs, ok
}

type mockSupport struct {
	ledger        ordererledger.ReadWriter
	sharedConfig  *mocksharedconfig.Manager
	policyManager *mockpolicies.Manager
}

func (mcs *mockSupport) PolicyManager() policies.Manager {
	return mcs.policyManager
}

func (mcs *mockSupport) Reader() ordererledger.Reader {
	return mcs.ledger
}

func NewRAMLedger() ordererledger.ReadWriter {
	rlf := ramledger.New(ledgerSize + 1)
	rl, _ := rlf.GetOrCreate(provisional.TestChainID)
	rl.Append(genesisBlock)
	return rl
}

func (mcs *mockSupport) SharedConfig() sharedconfig.Manager {
	return mcs.sharedConfig
}

func newMockMultichainManager() *mockSupportManager {
	rl := NewRAMLedger()
	mm := &mockSupportManager{
		chains: make(map[string]*mockSupport),
	}
	mm.chains[systemChainID] = &mockSupport{
		ledger:        rl,
		sharedConfig:  &mocksharedconfig.Manager{EgressPolicyNamesVal: []string{"somePolicy"}},
		policyManager: &mockpolicies.Manager{Policy: &mockpolicies.Policy{}},
	}
	return mm
}

var seekOldest = &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{&ab.SeekOldest{}}}
var seekNewest = &ab.SeekPosition{Type: &ab.SeekPosition_Newest{&ab.SeekNewest{}}}

func seekSpecified(number uint64) *ab.SeekPosition {
	return &ab.SeekPosition{Type: &ab.SeekPosition_Specified{&ab.SeekSpecified{Number: number}}}
}

func makeSeek(chainID string, seekInfo *ab.SeekInfo) *cb.Envelope {
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChainHeader: &cb.ChainHeader{
					ChainID: chainID,
				},
				SignatureHeader: &cb.SignatureHeader{},
			},
			Data: utils.MarshalOrPanic(seekInfo),
		}),
	}
}

func TestOldestSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

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
			} else {
				if deliverReply.GetBlock().Header.Number != count {
					t.Fatalf("Expected block %d but got block %d", count, deliverReply.GetBlock().Header.Number)
				}
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
	}
}

func TestNewestSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

	go ds.Handle(m)

	m.recvChan <- makeSeek(systemChainID, &ab.SeekInfo{Start: seekNewest, Stop: seekNewest, Behavior: ab.SeekInfo_BLOCK_UNTIL_READY})

	select {
	case deliverReply := <-m.sendChan:
		if deliverReply.GetBlock() == nil {
			if deliverReply.GetStatus() != cb.Status_SUCCESS {
				t.Fatalf("Received an error on the reply channel")
			}
			return
		} else {
			if deliverReply.GetBlock().Header.Number != uint64(ledgerSize-1) {
				t.Fatalf("Expected only the most recent block")
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestSpecificSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)
	specifiedStart := uint64(3)
	specifiedStop := uint64(7)

	go ds.Handle(m)

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
			} else {
				if expected := specifiedStart + count; deliverReply.GetBlock().Header.Number != expected {
					t.Fatalf("Expected block %d but got block %d", expected, deliverReply.GetBlock().Header.Number)
				}
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
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
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

func TestBadSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

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
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm)

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
		ledger := mm.chains[systemChainID].ledger
		ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}))
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

	ledger := mm.chains[systemChainID].ledger
	ledger.Append(ordererledger.CreateNextBlock(ledger, []*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", ledgerSize+1))}}))

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
