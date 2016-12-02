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

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/policies"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"google.golang.org/grpc"
)

var genesisBlock *cb.Block

var systemChainID = "systemChain"

const ledgerSize = 10

func init() {
	bootstrapper := static.New()
	var err error
	genesisBlock, err = bootstrapper.GenesisBlock()
	if err != nil {
		panic("Error intializing static bootstrap genesis block")
	}
}

// MagicLargestWindow is used as the default max window size for initializing the deliver service
const MagicLargestWindow int = 1000

type mockD struct {
	grpc.ServerStream
	recvChan chan *ab.DeliverUpdate
	sendChan chan *ab.DeliverResponse
}

func newMockD() *mockD {
	return &mockD{
		recvChan: make(chan *ab.DeliverUpdate),
		sendChan: make(chan *ab.DeliverResponse),
	}
}

func (m *mockD) Send(br *ab.DeliverResponse) error {
	m.sendChan <- br
	return nil
}

func (m *mockD) Recv() (*ab.DeliverUpdate, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return msg, fmt.Errorf("Channel closed")
	}
	return msg, nil
}

type mockMultichainManager struct {
	chains map[string]*mockChainSupport
}

func (mm *mockMultichainManager) GetChain(chainID string) (multichain.ChainSupport, bool) {
	cs, ok := mm.chains[chainID]
	return cs, ok
}

type mockChainSupport struct {
	ledger rawledger.ReadWriter
}

func (mcs *mockChainSupport) ConfigManager() configtx.Manager {
	panic("Unimplemented")
}

func (mcs *mockChainSupport) PolicyManager() policies.Manager {
	panic("Unimplemented")
}

func (mcs *mockChainSupport) Filters() *broadcastfilter.RuleSet {
	panic("Unimplemented")
}

func (mcs *mockChainSupport) Reader() rawledger.Reader {
	return mcs.ledger
}

func (mcs *mockChainSupport) Chain() multichain.Chain {
	panic("Unimplemented")
}

func newMockMultichainManager() *mockMultichainManager {
	_, rl := ramledger.New(ledgerSize, genesisBlock)
	mm := &mockMultichainManager{
		chains: make(map[string]*mockChainSupport),
	}
	mm.chains[string(systemChainID)] = &mockChainSupport{
		ledger: rl,
	}
	return mm
}

func TestOldestSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		mm.chains[string(systemChainID)].ledger.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_OLDEST, ChainID: systemChainID}}}

	count := 0
	for {
		select {
		case deliverReply := <-m.sendChan:
			if deliverReply.GetBlock() == nil {
				t.Fatalf("Received an error on the reply channel")
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
		if count == ledgerSize {
			break
		}
	}
}

func TestNewestSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		mm.chains[string(systemChainID)].ledger.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_NEWEST, ChainID: systemChainID}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetBlock() == nil {
			t.Fatalf("Received an error on the reply channel")
		}

		if blockReply.GetBlock().Header.Number != uint64(ledgerSize-1) {
			t.Fatalf("Expected only the most recent block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestSpecificSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < ledgerSize; i++ {
		mm.chains[string(systemChainID)].ledger.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_NEWEST, ChainID: systemChainID}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetBlock() == nil {
			t.Fatalf("Received an error on the reply channel")
		}

		if blockReply.GetBlock().Header.Number != uint64(ledgerSize-1) {
			t.Fatalf("Expected only the most recent block")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadSeek(t *testing.T) {
	mm := newMockMultichainManager()
	for i := 1; i < 2*ledgerSize; i++ {
		mm.chains[string(systemChainID)].ledger.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_SPECIFIED, SpecifiedNumber: uint64(ledgerSize - 1), ChainID: systemChainID}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != cb.Status_NOT_FOUND {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow), Start: ab.SeekInfo_SPECIFIED, SpecifiedNumber: uint64(3 * ledgerSize), ChainID: systemChainID}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != cb.Status_NOT_FOUND {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestBadWindow(t *testing.T) {
	mm := newMockMultichainManager()

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: uint64(MagicLargestWindow) * 2, Start: ab.SeekInfo_OLDEST, ChainID: systemChainID}}}

	select {
	case blockReply := <-m.sendChan:
		if blockReply.GetError() != cb.Status_BAD_REQUEST {
			t.Fatalf("Received wrong error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get all blocks")
	}
}

func TestAck(t *testing.T) {
	mm := newMockMultichainManager()
	windowSize := uint64(2)
	for i := 1; i < ledgerSize; i++ {
		mm.chains[string(systemChainID)].ledger.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte(fmt.Sprintf("%d", i))}}, nil)
	}

	m := newMockD()
	defer close(m.recvChan)
	ds := NewHandlerImpl(mm, MagicLargestWindow)

	go ds.Handle(m)

	m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Seek{Seek: &ab.SeekInfo{WindowSize: windowSize, Start: ab.SeekInfo_OLDEST, ChainID: systemChainID}}}

	count := uint64(0)
	for {
		select {
		case blockReply := <-m.sendChan:
			if blockReply.GetBlock() == nil {
				t.Fatalf("Received an error on the reply channel")
			}
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to get all blocks")
		}
		count++
		if count == windowSize {
			select {
			case <-m.sendChan:
				t.Fatalf("Window size exceeded")
			default:
			}
		}

		if count%windowSize == 0 {
			m.recvChan <- &ab.DeliverUpdate{Type: &ab.DeliverUpdate_Acknowledgement{Acknowledgement: &ab.Acknowledgement{Number: count}}}
		}

		if count == uint64(ledgerSize) {
			break
		}
	}
}
