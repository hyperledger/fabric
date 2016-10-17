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
	"testing"
	"time"

	"github.com/hyperledger/fabric/consensus"
	"github.com/hyperledger/fabric/consensus/util/events"
	pb "github.com/hyperledger/fabric/protos"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
)

func (op *obcBatch) getPBFTCore() *pbftCore {
	return op.pbft
}

func obcBatchHelper(id uint64, config *viper.Viper, stack consensus.Stack) pbftConsumer {
	// It's not entirely obvious why the compiler likes the parent function, but not newObcBatch directly
	return newObcBatch(id, config, stack)
}

func TestNetworkBatch(t *testing.T) {
	batchSize := 2
	validatorCount := 4
	net := makeConsumerNetwork(validatorCount, obcBatchHelper, func(ce *consumerEndpoint) {
		ce.consumer.(*obcBatch).batchSize = batchSize
	})
	defer net.stop()

	broadcaster := net.endpoints[generateBroadcaster(validatorCount)].getHandle()
	err := net.endpoints[1].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(1), broadcaster)
	if err != nil {
		t.Errorf("External request was not processed by backup: %v", err)
	}
	err = net.endpoints[2].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(2), broadcaster)
	if err != nil {
		t.Fatalf("External request was not processed by backup: %v", err)
	}

	net.process()
	net.process()

	if l := len(net.endpoints[0].(*consumerEndpoint).consumer.(*obcBatch).batchStore); l != 0 {
		t.Errorf("%d messages expected in primary's batchStore, found %v", 0,
			net.endpoints[0].(*consumerEndpoint).consumer.(*obcBatch).batchStore)
	}

	for _, ep := range net.endpoints {
		ce := ep.(*consumerEndpoint)
		block, err := ce.consumer.(*obcBatch).stack.GetBlock(1)
		if nil != err {
			t.Fatalf("Replica %d executed requests, expected a new block on the chain, but could not retrieve it : %s", ce.id, err)
		}
		numTrans := len(block.Transactions)
		if numTrans != batchSize {
			t.Fatalf("Replica %d executed %d requests, expected %d",
				ce.id, numTrans, batchSize)
		}
	}
}

var inertState = &omniProto{
	GetBlockchainInfoImpl: func() *pb.BlockchainInfo {
		return &pb.BlockchainInfo{
			CurrentBlockHash: []byte("GENESIS"),
			Height:           1,
		}
	},
	GetBlockchainInfoBlobImpl: func() []byte {
		b, _ := proto.Marshal(&pb.BlockchainInfo{
			CurrentBlockHash: []byte("GENESIS"),
			Height:           1,
		})
		return b
	},
	InvalidateStateImpl: func() {},
	ValidateStateImpl:   func() {},
	UpdateStateImpl:     func(id interface{}, target *pb.BlockchainInfo, peers []*pb.PeerID) {},
}

func TestClearOutstandingReqsOnStateRecovery(t *testing.T) {
	omni := *inertState
	omni.UnicastImpl = func(msg *pb.Message, receiverHandle *pb.PeerID) error { return nil }
	b := newObcBatch(0, loadConfig(), &omni)
	b.StateUpdated(&checkpointMessage{seqNo: 0, id: inertState.GetBlockchainInfoBlobImpl()}, inertState.GetBlockchainInfoImpl())

	defer b.Close()

	b.reqStore.storeOutstanding(&Request{})

	b.manager.Queue() <- stateUpdatedEvent{
		chkpt: &checkpointMessage{
			seqNo: 10,
		},
	}

	b.manager.Queue() <- nil

	if b.reqStore.outstandingRequests.Len() != 0 {
		t.Fatalf("Should not have any requests outstanding after completing state transfer")
	}
}

func TestOutstandingReqsIngestion(t *testing.T) {
	bs := [3]*obcBatch{}
	for i := range bs {
		omni := *inertState
		omni.UnicastImpl = func(ocMsg *pb.Message, peer *pb.PeerID) error { return nil }
		bs[i] = newObcBatch(uint64(i), loadConfig(), &omni)
		defer bs[i].Close()

		// Have vp1 only deliver messages
		if i == 1 {
			omni.UnicastImpl = func(ocMsg *pb.Message, peer *pb.PeerID) error {
				dest, _ := getValidatorID(peer)
				if dest == 0 || dest == 2 {
					bs[dest].RecvMsg(ocMsg, &pb.PeerID{Name: "vp1"})
				}
				return nil
			}
		}
	}
	for i := range bs {
		bs[i].StateUpdated(&checkpointMessage{seqNo: 0, id: inertState.GetBlockchainInfoBlobImpl()}, inertState.GetBlockchainInfoImpl())
	}

	err := bs[1].RecvMsg(createTxMsg(1), &pb.PeerID{Name: "vp1"})
	if err != nil {
		t.Fatalf("External request was not processed by backup: %v", err)
	}

	for _, b := range bs {
		b.manager.Queue() <- nil
		b.broadcaster.Wait()
		b.manager.Queue() <- nil
	}

	for i, b := range bs {
		b.manager.Queue() <- nil
		count := b.reqStore.outstandingRequests.Len()
		if count != 1 {
			t.Errorf("Batch backup %d should have the request in its store", i)
		}
	}
}

func TestOutstandingReqsResubmission(t *testing.T) {
	config := loadConfig()
	config.Set("general.batchsize", 2)
	omni := *inertState
	b := newObcBatch(0, config, &omni)
	defer b.Close() // The broadcasting threads only cause problems here... but this test stalls without them

	transactionsBroadcast := 0
	omni.ExecuteImpl = func(tag interface{}, txs []*pb.Transaction) {
		transactionsBroadcast += len(txs)
		logger.Debugf("\nExecuting %d transactions (%v)\n", len(txs), txs)
		nextExec := b.pbft.lastExec + 1
		b.pbft.currentExec = &nextExec
		b.manager.Inject(executedEvent{tag: tag})
	}

	omni.CommitImpl = func(tag interface{}, meta []byte) {
		b.manager.Inject(committedEvent{})
	}

	omni.UnicastImpl = func(ocMsg *pb.Message, dest *pb.PeerID) error {
		return nil
	}

	b.StateUpdated(&checkpointMessage{seqNo: 0, id: inertState.GetBlockchainInfoBlobImpl()}, inertState.GetBlockchainInfoImpl())
	b.manager.Queue() <- nil // Make sure the state update finishes first

	reqs := make([]*Request, 8)
	for i := 0; i < len(reqs); i++ {
		reqs[i] = createPbftReq(int64(i), 0)
	}

	// Add four requests, with a batch size of 2
	b.reqStore.storeOutstanding(reqs[0])
	b.reqStore.storeOutstanding(reqs[1])
	b.reqStore.storeOutstanding(reqs[2])
	b.reqStore.storeOutstanding(reqs[3])

	executed := make(map[string]struct{})
	execute := func() {
		for d, reqBatch := range b.pbft.outstandingReqBatches {
			if _, ok := executed[d]; ok {
				continue
			}
			executed[d] = struct{}{}
			b.execute(b.pbft.lastExec+1, reqBatch)
		}
	}

	tmp := uint64(1)
	b.pbft.currentExec = &tmp
	events.SendEvent(b, committedEvent{})
	execute()

	if b.reqStore.outstandingRequests.Len() != 0 {
		t.Fatalf("All request batches should have been executed and deleted after exec")
	}

	// Simulate changing views, with a request in the qSet, and one outstanding which is not
	wreqsBatch := &RequestBatch{Batch: []*Request{reqs[4]}}
	prePrep := &PrePrepare{
		View:           0,
		SequenceNumber: b.pbft.lastExec + 1,
		BatchDigest:    "foo",
		RequestBatch:   wreqsBatch,
	}

	b.pbft.certStore[msgID{v: prePrep.View, n: prePrep.SequenceNumber}] = &msgCert{prePrepare: prePrep}

	// Add the request, which is already pre-prepared, to be outstanding, and one outstanding not pending, not prepared
	b.reqStore.storeOutstanding(reqs[4]) // req 6
	b.reqStore.storeOutstanding(reqs[5])
	b.reqStore.storeOutstanding(reqs[6])
	b.reqStore.storeOutstanding(reqs[7])

	events.SendEvent(b, viewChangedEvent{})
	execute()

	if b.reqStore.hasNonPending() {
		t.Errorf("All requests should have been resubmitted after view change")
	}

	// We should have one request in batch which has not been sent yet
	expected := 6
	if transactionsBroadcast != expected {
		t.Errorf("Expected %d transactions broadcast, got %d", expected, transactionsBroadcast)
	}

	events.SendEvent(b, batchTimerEvent{})
	execute()

	// If the already prepared request were to be resubmitted, we would get count 8 here
	expected = 7
	if transactionsBroadcast != expected {
		t.Errorf("Expected %d transactions broadcast, got %d", expected, transactionsBroadcast)
	}
}

func TestViewChangeOnPrimarySilence(t *testing.T) {
	omni := *inertState
	omni.UnicastImpl = func(ocMsg *pb.Message, peer *pb.PeerID) error { return nil } // For the checkpoint
	omni.SignImpl = func(msg []byte) ([]byte, error) { return msg, nil }
	omni.VerifyImpl = func(peerID *pb.PeerID, signature []byte, message []byte) error { return nil }
	b := newObcBatch(1, loadConfig(), &omni)
	b.StateUpdated(&checkpointMessage{seqNo: 0, id: inertState.GetBlockchainInfoBlobImpl()}, inertState.GetBlockchainInfoImpl())
	b.pbft.requestTimeout = 50 * time.Millisecond
	defer b.Close()

	// Send a request, which will be ignored, triggering view change
	b.manager.Queue() <- batchMessageEvent{createTxMsg(1), &pb.PeerID{Name: "vp0"}}
	time.Sleep(time.Second)
	b.manager.Queue() <- nil

	if b.pbft.activeView {
		t.Fatalf("Should have caused a view change")
	}
}

func obcBatchSizeOneHelper(id uint64, config *viper.Viper, stack consensus.Stack) pbftConsumer {
	// It's not entirely obvious why the compiler likes the parent function, but not newObcClassic directly
	config.Set("general.batchsize", 1)
	return newObcBatch(id, config, stack)
}

func TestClassicStateTransfer(t *testing.T) {
	validatorCount := 4
	net := makeConsumerNetwork(validatorCount, obcBatchSizeOneHelper, func(ce *consumerEndpoint) {
		ce.consumer.(*obcBatch).pbft.K = 2
		ce.consumer.(*obcBatch).pbft.L = 4
	})
	defer net.stop()
	// net.debug = true

	filterMsg := true
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if filterMsg && dst == 3 { // 3 is byz
			return nil
		}
		return msg
	}

	// Advance the network one seqNo past so that Replica 3 will have to do statetransfer
	broadcaster := net.endpoints[generateBroadcaster(validatorCount)].getHandle()
	net.endpoints[1].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(1), broadcaster)
	net.process()

	// Move the seqNo to 9, at seqNo 6, Replica 3 will realize it's behind, transfer to seqNo 8, then execute seqNo 9
	filterMsg = false
	for n := 2; n <= 9; n++ {
		net.endpoints[1].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(int64(n)), broadcaster)
	}

	net.process()

	for _, ep := range net.endpoints {
		ce := ep.(*consumerEndpoint)
		obc := ce.consumer.(*obcBatch)
		_, err := obc.stack.GetBlock(9)
		if nil != err {
			t.Errorf("Replica %d executed requests, expected a new block on the chain, but could not retrieve it : %s", ce.id, err)
		}
		if !obc.pbft.activeView || obc.pbft.view != 0 {
			t.Errorf("Replica %d not active in view 0, is %v %d", ce.id, obc.pbft.activeView, obc.pbft.view)
		}
	}
}

func TestClassicBackToBackStateTransfer(t *testing.T) {
	validatorCount := 4
	net := makeConsumerNetwork(validatorCount, obcBatchSizeOneHelper, func(ce *consumerEndpoint) {
		ce.consumer.(*obcBatch).pbft.K = 2
		ce.consumer.(*obcBatch).pbft.L = 4
		ce.consumer.(*obcBatch).pbft.requestTimeout = time.Hour // We do not want any view changes
	})
	defer net.stop()
	// net.debug = true

	filterMsg := true
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if filterMsg && dst == 3 { // 3 is byz
			return nil
		}
		return msg
	}

	// Get the group to advance past seqNo 1, leaving Replica 3 behind
	broadcaster := net.endpoints[generateBroadcaster(validatorCount)].getHandle()
	net.endpoints[1].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(1), broadcaster)
	net.process()

	// Now start including Replica 3, go to sequence number 10, Replica 3 will trigger state transfer
	// after seeing seqNo 8, then pass another target for seqNo 10 and 12, but transfer to 8, but the network
	// will have already moved on and be past to seqNo 13, outside of Replica 3's watermarks, but
	// Replica 3 will execute through seqNo 12
	filterMsg = false
	for n := 2; n <= 21; n++ {
		net.endpoints[1].(*consumerEndpoint).consumer.RecvMsg(createTxMsg(int64(n)), broadcaster)
	}

	net.process()

	for _, ep := range net.endpoints {
		ce := ep.(*consumerEndpoint)
		obc := ce.consumer.(*obcBatch)
		_, err := obc.stack.GetBlock(21)
		if nil != err {
			t.Errorf("Replica %d executed requests, expected a new block on the chain, but could not retrieve it : %s", ce.id, err)
		}
		if !obc.pbft.activeView || obc.pbft.view != 0 {
			t.Errorf("Replica %d not active in view 0, is %v %d", ce.id, obc.pbft.activeView, obc.pbft.view)
		}
	}
}

func TestClearBatchStoreOnViewChange(t *testing.T) {
	omni := *inertState
	omni.UnicastImpl = func(ocMsg *pb.Message, peer *pb.PeerID) error { return nil } // For the checkpoint
	b := newObcBatch(1, loadConfig(), &omni)
	b.StateUpdated(&checkpointMessage{seqNo: 0, id: inertState.GetBlockchainInfoBlobImpl()}, inertState.GetBlockchainInfoImpl())
	defer b.Close()

	b.batchStore = []*Request{&Request{}}

	// Send a request, which will be ignored, triggering view change
	b.manager.Queue() <- viewChangedEvent{}
	b.manager.Queue() <- nil

	if len(b.batchStore) != 0 {
		t.Fatalf("Should have cleared the batch store on view change")
	}
}
