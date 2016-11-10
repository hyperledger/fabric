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

package solo

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

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

func getFiltersAndConfig() (*broadcastfilter.RuleSet, *mockConfigManager) {
	cm := &mockConfigManager{}
	filters := broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		&mockConfigFilter{cm},
		broadcastfilter.AcceptRule,
	})
	return filters, cm

}

var genesisBlock *cb.Block

var configTx []byte

func init() {
	bootstrapper := static.New()
	var err error
	genesisBlock, err = bootstrapper.GenesisBlock()
	if err != nil {
		panic("Error intializing static bootstrap genesis block")
	}

	configTx, err = proto.Marshal(&cb.ConfigurationEnvelope{})
	if err != nil {
		panic("Error marshaling empty config tx")
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

func TestQueueOverflow(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	bs := newPlainBroadcastServer(2, 1, time.Second, nil, filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	m := newMockB()
	b := newBroadcaster(bs)
	go b.queueEnvelopes(m)
	defer close(m.recvChan)

	bs.halt()

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
	filters, cm := getFiltersAndConfig()
	bs := newPlainBroadcastServer(2, 1, time.Second, nil, filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	// m := newMockB()
	ms := []*mockB{newMockB(), newMockB(), newMockB()}

	for _, m := range ms {
		b := newBroadcaster(bs)
		go b.queueEnvelopes(m)
		defer close(m.recvChan)
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
	filters, cm := getFiltersAndConfig()
	bs := newPlainBroadcastServer(2, 1, time.Second, nil, filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	m := newMockB()
	defer close(m.recvChan)
	go bs.handleBroadcast(m)

	m.recvChan <- &cb.Envelope{}
	reply := <-m.sendChan
	if reply.Status != cb.Status_BAD_REQUEST {
		t.Fatalf("Should have rejected the null message")
	}

}

func TestEmptyBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	bs := newPlainBroadcastServer(2, 1, time.Millisecond, ramledger.New(10, genesisBlock), filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	if bs.rl.(rawledger.Reader).Height() != 1 {
		t.Fatalf("Expected no new blocks created")
	}
}

func TestBatchTimer(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	rl := ramledger.New(10, genesisBlock)
	bs := newBroadcastServer(0, batchSize, time.Millisecond, rl, filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	defer bs.halt()
	it, _ := rl.Iterator(ab.SeekInfo_SPECIFIED, 1)

	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}

	select {
	case <-it.ReadyChan():
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because of batch timer expiration but did not")
	}
}

func TestFilledBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	messages := 10
	bs := newPlainBroadcastServer(0, batchSize, time.Hour, ramledger.New(10, genesisBlock), filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()
	for i := 0; i < messages; i++ {
		bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	}
	bs.halt()
	<-done
	expected := uint64(1 + messages/batchSize)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}
}

func TestReconfigureGoodPath(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	bs := newPlainBroadcastServer(0, batchSize, time.Hour, ramledger.New(10, genesisBlock), filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()

	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg1")}
	bs.sendChan <- &cb.Envelope{Payload: configTx}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg2")}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg3")}

	bs.halt()
	<-done
	expected := uint64(4)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if !cm.applied {
		t.Errorf("ConfigTx should have been applied after processing")
	}
}

func TestReconfigureFailToValidate(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	cm.validateErr = fmt.Errorf("Fail to validate")
	batchSize := 2
	bs := newPlainBroadcastServer(0, batchSize, time.Hour, ramledger.New(10, genesisBlock), filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()

	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg1")}
	bs.sendChan <- &cb.Envelope{Payload: configTx}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg2")}

	bs.halt()
	<-done
	expected := uint64(2)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if cm.applied {
		t.Errorf("ConfigTx should not have been applied")
	}
}

func TestReconfigureFailToApply(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	cm.applyErr = fmt.Errorf("Fail to apply")
	batchSize := 2
	bs := newPlainBroadcastServer(0, batchSize, time.Hour, ramledger.New(10, genesisBlock), filters, cm) // queueSize, batchSize (unused), batchTimeout (unused), ramLedger (unused), filters, configManager
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()

	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg1")}
	bs.sendChan <- &cb.Envelope{Payload: configTx}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg2")}

	bs.halt()
	<-done
	expected := uint64(2)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if !cm.applied {
		t.Errorf("ConfigTx should tried to apply")
	}
}
