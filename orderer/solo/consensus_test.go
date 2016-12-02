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

	"github.com/hyperledger/fabric/orderer/common/blockcutter"
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

func (mcm *mockConfigManager) ChainID() string {
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

func getEverything(batchSize int) (*mockConfigManager, blockcutter.Receiver, rawledger.ReadWriter) {
	cm := &mockConfigManager{}
	filters := broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		&mockConfigFilter{cm},
		broadcastfilter.AcceptRule,
	})
	cutter := blockcutter.NewReceiverImpl(batchSize, filters, cm)
	_, rl := ramledger.New(10, genesisBlock)
	return cm, cutter, rl

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

func TestEmptyBatch(t *testing.T) {
	cm, cutter, rl := getEverything(1)
	bs := newChain(time.Millisecond, cm, cutter, rl, nil)
	if bs.rl.(rawledger.Reader).Height() != 1 {
		t.Fatalf("Expected no new blocks created")
	}
}

func TestBatchTimer(t *testing.T) {
	batchSize := 2
	cm, cutter, rl := getEverything(batchSize)
	bs := newChain(time.Millisecond, cm, cutter, rl, nil)
	bs.Start()
	defer bs.Halt()
	it, _ := rl.Iterator(ab.SeekInfo_SPECIFIED, 1)

	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}

	select {
	case <-it.ReadyChan():
		it.Next()
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because of batch timer expiration but did not")
	}

	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	select {
	case <-it.ReadyChan():
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the timer was not appopriately reset")
	}
}

func TestBatchTimerHaltOnFilledBatch(t *testing.T) {
	batchSize := 2
	cm, cutter, rl := getEverything(batchSize)
	bs := newChain(time.Millisecond, cm, cutter, rl, nil)
	bs.Start()
	defer bs.Halt()
	it, _ := rl.Iterator(ab.SeekInfo_SPECIFIED, 1)

	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}

	select {
	case <-it.ReadyChan():
		it.Next()
	case <-time.After(time.Second):
		t.Fatalf("Expected a block to be cut because the batch was filled, but did not")
	}

	// Change the batch timeout to be near instant
	bs.batchTimeout = time.Millisecond

	bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	select {
	case <-it.ReadyChan():
	case <-time.After(time.Second):
		t.Fatalf("Did not create the second batch, indicating that the old timer was still running")
	}
}

func TestFilledBatch(t *testing.T) {
	batchSize := 2
	cm, cutter, rl := getEverything(batchSize)
	bs := newChain(time.Hour, cm, cutter, rl, nil)
	messages := 10
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()
	for i := 0; i < messages; i++ {
		bs.sendChan <- &cb.Envelope{Payload: []byte("Some bytes")}
	}
	bs.Halt()
	<-done
	expected := uint64(1 + messages/batchSize)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}
}

func TestReconfigureGoodPath(t *testing.T) {
	batchSize := 2
	cm, cutter, rl := getEverything(batchSize)
	bs := newChain(time.Hour, cm, cutter, rl, nil)
	done := make(chan struct{})
	go func() {
		bs.main()
		close(done)
	}()

	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg1")}
	bs.sendChan <- &cb.Envelope{Payload: configTx}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg2")}
	bs.sendChan <- &cb.Envelope{Payload: []byte("Msg3")}

	bs.Halt()
	<-done
	expected := uint64(4)
	if bs.rl.(rawledger.Reader).Height() != expected {
		t.Fatalf("Expected %d blocks but got %d", expected, bs.rl.(rawledger.Reader).Height())
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}
}
