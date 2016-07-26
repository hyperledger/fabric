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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/consensus/util/events"
	pb "github.com/hyperledger/fabric/protos"
)

type pbftEndpoint struct {
	*testEndpoint
	pbft    *pbftCore
	sc      *simpleConsumer
	manager events.Manager
}

func (pe *pbftEndpoint) deliver(msgPayload []byte, senderHandle *pb.PeerID) {
	senderID, _ := getValidatorID(senderHandle)
	msg := &Message{}
	err := proto.Unmarshal(msgPayload, msg)
	if err != nil {
		panic("Asked to deliver something which did not unmarshal")
	}

	pe.manager.Queue() <- &pbftMessage{msg: msg, sender: senderID}
}

func (pe *pbftEndpoint) stop() {
	pe.pbft.close()
}

func (pe *pbftEndpoint) isBusy() bool {
	if pe.pbft.timerActive || pe.pbft.currentExec != nil {
		pe.net.debugMsg("TEST: Returning as busy because timer active (%v) or current exec (%v)\n", pe.pbft.timerActive, pe.pbft.currentExec)
		return true
	}

	// TODO, this looks racey, but seems fine, because the message send is on an unbuffered
	// channel, the send blocks until the thread has picked up the new work, still
	// this will be removed pending the transition to an externally driven state machine
	select {
	case pe.manager.Queue() <- nil:
	default:
		pe.net.debugMsg("TEST: Returning as busy no reply on idleChan\n")
		return true
	}

	return false
}

type pbftNetwork struct {
	*testnet
	pbftEndpoints []*pbftEndpoint
}

type simpleConsumer struct {
	pe            *pbftEndpoint
	pbftNet       *pbftNetwork
	executions    uint64
	lastSeqNo     uint64
	skipOccurred  bool
	lastExecution string
	mockPersist
}

func (sc *simpleConsumer) broadcast(msgPayload []byte) {
	sc.pe.Broadcast(&pb.Message{Payload: msgPayload}, pb.PeerEndpoint_VALIDATOR)
}
func (sc *simpleConsumer) unicast(msgPayload []byte, receiverID uint64) error {
	handle, err := getValidatorHandle(receiverID)
	if nil != err {
		return err
	}
	sc.pe.Unicast(&pb.Message{Payload: msgPayload}, handle)
	return nil
}

func (sc *simpleConsumer) Close() {
	// No-op
}

func (sc *simpleConsumer) sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (sc *simpleConsumer) verify(senderID uint64, signature []byte, message []byte) error {
	return nil
}

func (sc *simpleConsumer) viewChange(curView uint64) {
}

func (sc *simpleConsumer) invalidateState() {}
func (sc *simpleConsumer) validateState()   {}

func (sc *simpleConsumer) skipTo(seqNo uint64, id []byte, replicas []uint64) {
	sc.skipOccurred = true
	sc.executions = seqNo
	go func() {
		sc.pe.manager.Queue() <- stateUpdatedEvent{
			chkpt: &checkpointMessage{
				seqNo: seqNo,
				id:    id,
			},
			target: &pb.BlockchainInfo{},
		}
	}()
	sc.pbftNet.debugMsg("TEST: skipping to %d\n", seqNo)
}

func (sc *simpleConsumer) execute(seqNo uint64, reqBatch *RequestBatch) {
	for _, req := range reqBatch.GetBatch() {
		sc.pbftNet.debugMsg("TEST: executing request\n")
		sc.lastExecution = hash(req)
		sc.executions++
		sc.lastSeqNo = seqNo
		go func() { sc.pe.manager.Queue() <- execDoneEvent{} }()
	}
}

func (sc *simpleConsumer) getState() []byte {
	return []byte(fmt.Sprintf("%d", sc.executions))
}

func (sc *simpleConsumer) getLastSeqNo() (uint64, error) {
	if sc.executions < 1 {
		return 0, fmt.Errorf("no execution yet")
	}
	return sc.lastSeqNo, nil
}

func makePBFTNetwork(N int, config *viper.Viper) *pbftNetwork {
	if config == nil {
		config = loadConfig()
	}

	config.Set("general.N", N)
	config.Set("general.f", (N-1)/3)
	endpointFunc := func(id uint64, net *testnet) endpoint {
		tep := makeTestEndpoint(id, net)
		pe := &pbftEndpoint{
			testEndpoint: tep,
			manager:      events.NewManagerImpl(),
		}

		pe.sc = &simpleConsumer{
			pe: pe,
		}

		pe.pbft = newPbftCore(id, config, pe.sc, events.NewTimerFactoryImpl(pe.manager))
		pe.manager.SetReceiver(pe.pbft)

		pe.manager.Start()

		return pe

	}

	pn := &pbftNetwork{testnet: makeTestnet(N, endpointFunc)}
	pn.pbftEndpoints = make([]*pbftEndpoint, len(pn.endpoints))
	for i, ep := range pn.endpoints {
		pn.pbftEndpoints[i] = ep.(*pbftEndpoint)
		pn.pbftEndpoints[i].sc.pbftNet = pn
	}
	return pn
}
