/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/transientstore"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestStopDrainsStateChannelsWithoutPanic fills both state channels with
// charged messages, then stops: Stop must dispose the queues exactly once
// without panicking, refuse later handoffs, and never send on a closed
// channel.
func TestStopDrainsStateChannelsWithoutPanic(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 2),
		stateResponseCh: make(chan protoext.ReceivedMessage, 2),
		stopCh:          make(chan struct{}),
		privateDataSem:  make(chan struct{}, 4),
		ledger:          &fakeLedger{},
	}

	var released atomic.Int32
	for i := range 2 {
		s.stateRequestCh <- chargedStateTestMessage(t, uint64(i), false, "testchannel", &released)
		s.stateResponseCh <- chargedStateTestMessage(t, uint64(i), true, "testchannel", &released)
	}

	require.NotPanics(t, func() { s.Stop() })
	require.Empty(t, s.stateRequestCh)
	require.Empty(t, s.stateResponseCh)
	require.EqualValues(t, 4, released.Load(), "queued messages must be disposed exactly once")
	require.True(t, s.ledger.(*fakeLedger).closed.Load())

	// Handoffs after Stop are refused, and the channels are never closed.
	require.False(t, s.directMessage(stateTestMessage(t, 3, false, "testchannel")))
	require.NotPanics(t, func() { s.directMessage(stateTestMessage(t, 4, true, "testchannel")) })
}

// TestTransferExitDrainsResponses proves requestBlocksInRange disposes every
// charged response still queued when the transfer round ends, exactly once.
func TestTransferExitDrainsResponses(t *testing.T) {
	g := &mocks.GossipMock{}
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember(nil))

	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		mediator:        &ServicesMediator{GossipAdapter: g},
		stateRequestCh:  make(chan protoext.ReceivedMessage, 2),
		stateResponseCh: make(chan protoext.ReceivedMessage, 2),
		stopCh:          make(chan struct{}),
		config:          &StateConfig{StateBatchSize: 1},
	}

	var released atomic.Int32
	for i := range 2 {
		s.stateResponseCh <- chargedStateTestMessage(t, uint64(i), true, "testchannel", &released)
	}

	// No peers to request from: the transfer ends immediately and the exit
	// drain must dispose the queued responses.
	s.requestBlocksInRange(1, 2)
	require.Empty(t, s.stateResponseCh)
	require.EqualValues(t, 2, released.Load(), "queued responses must be disposed exactly once")
}

// TestPrivateDataBoundedConcurrency proves the semaphore bounds the number
// of concurrent private-data handlers and drops the rest.
func TestPrivateDataBoundedConcurrency(t *testing.T) {
	ledger := &fakeLedger{storePvtDataGate: make(chan struct{})}
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 4),
		stateResponseCh: make(chan protoext.ReceivedMessage, 4),
		stopCh:          make(chan struct{}),
		privateDataSem:  make(chan struct{}, 2),
		ledger:          ledger,
	}

	inCh := make(chan protoext.ReceivedMessage, 20)
	go s.receiveAndDispatchDirectMessages(inCh)

	for i := range 20 {
		inCh <- privateDataTestMessage(t, uint64(i), "testchannel")
	}
	close(inCh)

	// Two handlers park on the gated ledger; the other eighteen messages
	// are dropped at the semaphore.
	require.Eventually(t, func() bool {
		return ledger.storePvtDataCalls.Load() == 2
	}, 30*time.Second, time.Millisecond, "expected two admitted handlers, got %d", ledger.storePvtDataCalls.Load())
	require.Eventually(t, func() bool {
		return s.droppedDirect.Load() == 18
	}, 30*time.Second, time.Millisecond, "expected 18 dropped messages, got %d", s.droppedDirect.Load())

	close(ledger.storePvtDataGate)
	require.Eventually(t, func() bool {
		return len(s.privateDataSem) == 0
	}, 30*time.Second, time.Millisecond, "parked handlers did not complete after the ledger unblocked")
}

// TestPrivateDataDoesNotStarveStateMessages saturates the private-data
// handlers with a blocked ledger, then delivers a state response: the
// handoff is synchronous and must succeed without waiting for a handler
// slot, so anti-entropy is not starved by private-data work.
func TestPrivateDataDoesNotStarveStateMessages(t *testing.T) {
	ledger := &fakeLedger{storePvtDataGate: make(chan struct{})}
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 1),
		stateResponseCh: make(chan protoext.ReceivedMessage, 1),
		stopCh:          make(chan struct{}),
		privateDataSem:  make(chan struct{}, 2),
		ledger:          ledger,
	}
	atomic.StoreInt32(&s.stateTransferActive, 1)

	inCh := make(chan protoext.ReceivedMessage)
	loopDone := make(chan struct{})
	go func() {
		s.receiveAndDispatchDirectMessages(inCh)
		close(loopDone)
	}()

	// Occupy every handler slot with ledger-blocked private-data work.
	for i := range 2 {
		inCh <- privateDataTestMessage(t, uint64(i), "testchannel")
	}
	require.Eventually(t, func() bool {
		return ledger.storePvtDataCalls.Load() == 2
	}, 30*time.Second, time.Millisecond, "handlers did not occupy every slot")

	// A state response must be handed off synchronously despite the
	// saturated semaphore, and its charge must stay with the queue.
	var released atomic.Int32
	response := chargedStateTestMessage(t, 3, true, "testchannel", &released)
	inCh <- response
	require.Eventually(t, func() bool {
		return len(s.stateResponseCh) == 1
	}, 30*time.Second, time.Millisecond, "state response starved behind saturated private-data handlers")
	require.Zero(t, released.Load(), "handed-off response must keep its charge")

	// The queue consumer owns the release.
	comm.DisposeMessage(<-s.stateResponseCh)
	require.EqualValues(t, 1, released.Load())

	close(ledger.storePvtDataGate)
	close(inCh)
	<-loopDone
}

// TestStopDisposesMessageAwaitingDispatch forces the stop interleaving: a
// charged message the receive loop has already taken must be disposed when
// stopCh fires before its dispatch, never abandoned with its charge held.
func TestStopDisposesMessageAwaitingDispatch(t *testing.T) {
	ledger := &fakeLedger{storePvtDataGate: make(chan struct{})}
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 1),
		stateResponseCh: make(chan protoext.ReceivedMessage, 1),
		stopCh:          make(chan struct{}),
		privateDataSem:  make(chan struct{}, 1),
		ledger:          ledger,
	}

	inCh := make(chan protoext.ReceivedMessage)
	loopDone := make(chan struct{})
	go func() {
		s.receiveAndDispatchDirectMessages(inCh)
		close(loopDone)
	}()

	// Occupy the only handler slot with ledger-blocked work, then stop.
	inCh <- privateDataTestMessage(t, 1, "testchannel")
	require.Eventually(t, func() bool {
		return ledger.storePvtDataCalls.Load() == 1
	}, 30*time.Second, time.Millisecond, "handler did not occupy the slot")
	close(s.stopCh)

	var released atomic.Int32
	inCh <- chargedStateTestMessage(t, 2, false, "testchannel", &released)

	require.Eventually(t, func() bool {
		return released.Load() == 1
	}, 30*time.Second, time.Millisecond, "message taken before stop must be disposed exactly once")
	<-loopDone

	close(ledger.storePvtDataGate)
	close(inCh)
}

// TestZeroChannelSizeStillHandlesPrivateData pins the floor: an explicit
// zero state channel size must not drop every private-data message, because
// the handler bound does not derive from the state queue capacity alone.
func TestZeroChannelSizeStillHandlesPrivateData(t *testing.T) {
	provider := gmetricsmocks.TestUtilConstructMetricProvider()
	stateMetrics := metrics.NewGossipMetrics(provider.FakeProvider).StateMetrics

	dataChan := make(chan *proto.GossipMessage)
	defer close(dataChan)
	commChan := make(chan protoext.ReceivedMessage, 1)
	defer close(commChan)
	g := &mocks.GossipMock{}
	g.On("Accept", mock.Anything, false).Return((<-chan *proto.GossipMessage)(dataChan), nil)
	g.On("Accept", mock.Anything, true).Return(nil, commChan)

	ledger := &fakeLedger{}
	config := &StateConfig{
		StateCheckInterval:   DefStateCheckInterval,
		StateResponseTimeout: DefStateResponseTimeout,
		StateBatchSize:       DefStateBatchSize,
		StateMaxRetries:      DefStateMaxRetries,
		StateBlockBufferSize: DefStateBlockBufferSize,
		StateChannelSize:     0,
		StateEnabled:         false,
	}
	sp := NewGossipStateProvider(flogging.MustGetLogger("test"), "testchannel",
		&ServicesMediator{GossipAdapter: g}, ledger, stateMetrics, false, config)
	require.NotNil(t, sp)
	defer sp.Stop()
	s := sp.(*GossipStateProviderImpl)
	require.Positive(t, cap(s.privateDataSem), "private-data handler bound must stay positive")

	commChan <- privateDataTestMessage(t, 1, "testchannel")
	require.Eventually(t, func() bool {
		return ledger.storePvtDataCalls.Load() == 1
	}, 30*time.Second, time.Millisecond, "private data message was dropped with a zero state channel size")
}

// TestLateStateResponseAfterDrain forces the interleaving where a handler
// reads the active flag before the transfer drain and takes the mutex after
// it: the late response must be refused and never stay charged in the queue.
func TestLateStateResponseAfterDrain(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 1),
		stateResponseCh: make(chan protoext.ReceivedMessage, 1),
		stopCh:          make(chan struct{}),
	}
	atomic.StoreInt32(&s.stateTransferActive, 1)

	// Emulate requestBlocksInRange teardown: the drain runs while a handler
	// blocks on dispatchMu.
	s.dispatchMu.Lock()
	drainStarted := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		close(drainStarted)
		s.drainStateResponses()
		close(drainDone)
	}()
	<-drainStarted

	lateResponse := stateTestMessage(t, 1, true, "testchannel")
	handlerDone := make(chan struct{})
	go func() {
		s.directMessage(lateResponse)
		close(handlerDone)
	}()

	// Let the handler read stateTransferActive == 1 and block on dispatchMu.
	time.Sleep(50 * time.Millisecond)
	atomic.StoreInt32(&s.stateTransferActive, 0)
	s.dispatchMu.Unlock()

	<-drainDone
	<-handlerDone

	require.Empty(t, s.stateResponseCh, "a late response was queued after the transfer drain and stays charged")
}

// stateTestMessage builds a NoopSign'ed state request or response message
// for the given channel.
func stateTestMessage(t *testing.T, nonce uint64, response bool, channel string) protoext.ReceivedMessage {
	msg := &proto.GossipMessage{
		Nonce:   nonce,
		Channel: []byte(channel),
	}
	if response {
		msg.Content = &proto.GossipMessage_StateResponse{StateResponse: &proto.RemoteStateResponse{}}
	} else {
		msg.Content = &proto.GossipMessage_StateRequest{StateRequest: &proto.RemoteStateRequest{}}
	}
	sMsg, err := protoext.NoopSign(msg)
	require.NoError(t, err)
	return &comm.ReceivedMessageImpl{SignedGossipMessage: sMsg}
}

// chargedStateTestMessage builds a state message wired the way the comm
// layer delivers it: one tracked delivery whose disposal fires released.
func chargedStateTestMessage(t *testing.T, nonce uint64, response bool, channel string, released *atomic.Int32) protoext.ReceivedMessage {
	rmi := stateTestMessage(t, nonce, response, channel).(*comm.ReceivedMessageImpl)
	rmi.WireConsumption(func() { released.Add(1) })
	demux := comm.NewChannelDemultiplexer()
	sub := demux.AddChannel(func(any) bool { return true })
	demux.DeMultiplexAbortable(rmi, nil)
	<-sub // the tracked delivery; ownership now rides with the message
	demux.Close()
	return rmi
}

// noopAckMessage detaches the reply paths so ledger-gated private-data
// handlers can acknowledge without a connection.
type noopAckMessage struct {
	protoext.ReceivedMessage
}

func (noopAckMessage) Ack(error)                    {}
func (noopAckMessage) Respond(*proto.GossipMessage) {}

// privateDataTestMessage builds a well-formed private-data message for the
// given channel.
func privateDataTestMessage(t *testing.T, nonce uint64, channel string) protoext.ReceivedMessage {
	msg := &proto.GossipMessage{
		Nonce:   nonce,
		Channel: []byte(channel),
		Content: &proto.GossipMessage_PrivateData{
			PrivateData: &proto.PrivateDataMessage{
				Payload: &proto.PrivatePayload{
					CollectionName: "col",
					Namespace:      "ns",
					TxId:           "txid",
					PrivateRwset:   []byte("rwset"),
				},
			},
		},
	}
	sMsg, err := protoext.NoopSign(msg)
	require.NoError(t, err)
	return noopAckMessage{&comm.ReceivedMessageImpl{SignedGossipMessage: sMsg}}
}

type fakeLedger struct {
	closed            atomic.Bool
	storePvtDataGate  chan struct{}
	storePvtDataCalls atomic.Int32
}

func (l *fakeLedger) StoreBlock(*common.Block, util.PvtDataCollections) error { return nil }

func (l *fakeLedger) StorePvtData(string, *transientstore.TxPvtReadWriteSetWithConfigInfo, uint64) error {
	l.storePvtDataCalls.Add(1)
	if l.storePvtDataGate != nil {
		<-l.storePvtDataGate
	}
	return nil
}

func (l *fakeLedger) GetPvtDataAndBlockByNum(uint64, protoutil.SignedData) (*common.Block, util.PvtDataCollections, error) {
	return nil, nil, nil
}

func (l *fakeLedger) LedgerHeight() (uint64, error) { return 1, nil }

func (l *fakeLedger) Close() { l.closed.Store(true) }
