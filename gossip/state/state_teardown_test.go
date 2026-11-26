/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/transientstore"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestStopDrainsStateChannelsWithoutPanic fills both state channels, then
// stops: Stop must dispose the queues without panicking, refuse later
// handoffs, and never send on a closed channel.
func TestStopDrainsStateChannelsWithoutPanic(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 2),
		stateResponseCh: make(chan protoext.ReceivedMessage, 2),
		stopCh:          make(chan struct{}),
		directSem:       make(chan struct{}, 4),
		ledger:          &fakeLedger{},
	}

	for range 2 {
		s.stateRequestCh <- stateTestMessage(t, 1, false, "testchannel")
		s.stateResponseCh <- stateTestMessage(t, 2, true, "testchannel")
	}

	require.NotPanics(t, func() { s.Stop() })
	require.Empty(t, s.stateRequestCh)
	require.Empty(t, s.stateResponseCh)
	require.True(t, s.ledger.(*fakeLedger).closed.Load())

	// Handoffs after Stop are refused, and the channels are never closed.
	require.False(t, s.directMessage(stateTestMessage(t, 3, false, "testchannel")))
	require.NotPanics(t, func() { s.directMessage(stateTestMessage(t, 4, true, "testchannel")) })
}

// TestDrainStateResponsesDisposesQueue proves the exit drain empties the
// response queue so stranded responses never stay charged.
func TestDrainStateResponsesDisposesQueue(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateResponseCh: make(chan protoext.ReceivedMessage, 3),
		stopCh:          make(chan struct{}),
	}
	for i := range 3 {
		s.stateResponseCh <- stateTestMessage(t, uint64(i), true, "testchannel")
	}
	s.drainStateResponses()
	require.Empty(t, s.stateResponseCh)
}

// TestTransferExitDrainsResponses proves requestBlocksInRange disposes every
// response still queued when the transfer round ends.
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
	for i := range 2 {
		s.stateResponseCh <- stateTestMessage(t, uint64(i), true, "testchannel")
	}

	// No peers to request from: the transfer ends immediately and the exit
	// drain must dispose the queued responses.
	s.requestBlocksInRange(1, 2)
	require.Empty(t, s.stateResponseCh)
}

// TestDispatchBoundedConcurrency proves the dispatch semaphore bounds the
// number of concurrent direct-message handlers and drops the rest.
func TestDispatchBoundedConcurrency(t *testing.T) {
	s := &GossipStateProviderImpl{
		logger:          flogging.MustGetLogger("test"),
		chainID:         "testchannel",
		stateRequestCh:  make(chan protoext.ReceivedMessage, 4),
		stateResponseCh: make(chan protoext.ReceivedMessage, 4),
		stopCh:          make(chan struct{}),
		directSem:       make(chan struct{}, 2),
		ledger:          &fakeLedger{},
	}

	// Hold the handoff mutex so both handler slots stay occupied.
	s.dispatchMu.Lock()

	inCh := make(chan protoext.ReceivedMessage, 20)
	go s.receiveAndDispatchDirectMessages(inCh)

	baseline := runtime.NumGoroutine()
	for i := range 20 {
		inCh <- stateTestMessage(t, uint64(i), false, "testchannel")
	}
	close(inCh)

	// Two handlers park on dispatchMu; the other eighteen messages are dropped.
	require.Eventually(t, func() bool {
		return s.droppedDirect.Load() == 18
	}, 30*time.Second, time.Millisecond, "expected 18 dropped messages, got %d", s.droppedDirect.Load())
	require.LessOrEqual(t, runtime.NumGoroutine(), baseline+4, "goroutines must stay bounded by the semaphore")

	s.dispatchMu.Unlock()
	require.Eventually(t, func() bool {
		return len(s.stateRequestCh) == 2
	}, 30*time.Second, time.Millisecond, "parked handlers did not complete after unlock")
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

type fakeLedger struct {
	closed atomic.Bool
}

func (l *fakeLedger) StoreBlock(*common.Block, util.PvtDataCollections) error { return nil }

func (l *fakeLedger) StorePvtData(string, *transientstore.TxPvtReadWriteSetWithConfigInfo, uint64) error {
	return nil
}

func (l *fakeLedger) GetPvtDataAndBlockByNum(uint64, protoutil.SignedData) (*common.Block, util.PvtDataCollections, error) {
	return nil, nil, nil
}

func (l *fakeLedger) LedgerHeight() (uint64, error) { return 1, nil }

func (l *fakeLedger) Close() { l.closed.Store(true) }
