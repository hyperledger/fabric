/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package blocksprovider

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	common2 "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	maxRetryDelay = time.Second
}

type mockMCS struct {
	mock.Mock
}

func (*mockMCS) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (*mockMCS) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common2.PKIidType {
	return common2.PKIidType("pkiID")
}

func (m *mockMCS) VerifyBlock(chainID common2.ChainID, seqNum uint64, signedBlock []byte) error {
	args := m.Called()
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return nil
}

func (*mockMCS) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*mockMCS) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) VerifyByChannel(chainID common2.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

type rcvFunc func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error)

// Used to generate a simple test case to initialize delivery
// from given block sequence number.
func makeTestCase(ledgerHeight uint64, mcs api.MessageCryptoService, shouldSucceed bool, rcv rcvFunc) func(*testing.T) {
	return func(t *testing.T) {
		gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}
		deliverer := &mocks.MockBlocksDeliverer{Pos: ledgerHeight}
		deliverer.MockRecv = rcv
		provider := NewBlocksProvider("***TEST_CHAINID***", deliverer, gossipServiceAdapter, mcs)

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			provider.DeliverBlocks()
		}()

		for {
			time.Sleep(100 * time.Millisecond)
			if deliverer.RecvCount() > 0 {
				provider.Stop()
				break
			}
		}
		assertDelivery(t, gossipServiceAdapter, deliverer, shouldSucceed)

		wg.Wait()
	}
}

func assertDelivery(t *testing.T, ga *mocks.MockGossipServiceAdapter, deliverer *mocks.MockBlocksDeliverer, shouldSucceed bool) {
	// Check that all blocks received eventually get gossiped and locally committed

	select {
	case <-ga.GossipBlockDisseminations:
		if !shouldSucceed {
			assert.Fail(t, "Should not have succeede")
		}
		assert.Equal(t, deliverer.RecvCount(), ga.AddPayloadCount())
	case <-time.After(time.Second):
		if shouldSucceed {
			assert.Fail(t, "Didn't gossip a block within a timely manner")
		}
	}
}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	timeout := time.Second * 30
	start := time.Now()
	limit := start.UnixNano() + timeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeout / 60)
	}
	assert.Fail(t, "Timeout expired!")
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromTheOldest(t *testing.T) {
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(nil)
	makeTestCase(uint64(0), mcs, true, mocks.MockRecv)(t)
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromSpecified(t *testing.T) {
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(nil)
	makeTestCase(uint64(101), mcs, true, mocks.MockRecv)(t)
}

func TestBlocksProvider_CheckTerminationDeliveryResponseStatus(t *testing.T) {
	tmp := struct{ mocks.MockBlocksDeliverer }{}

	// Making mocked Recv() function to return DeliverResponse_Status to force block
	// provider to fail and exit, checking that in that case to block was actually
	// delivered.
	tmp.MockRecv = func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Status{
				Status: common.Status_SUCCESS,
			},
		}, nil
	}

	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{}
	provider := &blocksProviderImpl{
		chainID: "***TEST_CHAINID***",
		gossip:  gossipServiceAdapter,
		client:  &tmp,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	ready := make(chan struct{})
	go func() {
		provider.DeliverBlocks()
		wg.Done()
		// Send notification
		ready <- struct{}{}
	}()

	time.Sleep(time.Duration(10) * time.Millisecond)
	provider.Stop()

	select {
	case <-ready:
		{
			assert.Equal(t, int32(1), tmp.RecvCount())
			// No payload should commit locally
			assert.Equal(t, int32(0), gossipServiceAdapter.AddPayloadCount())
			// No payload should be transferred to other peers
			select {
			case <-gossipServiceAdapter.GossipBlockDisseminations:
				assert.Fail(t, "Gossiped block but shouldn't have")
			case <-time.After(time.Second):
			}
			return
		}
	case <-time.After(time.Duration(1) * time.Second):
		{
			t.Fatal("Test hasn't finished in timely manner, failing.")
		}
	}
}

func TestBlocksProvider_DeliveryWrongStatus(t *testing.T) {
	orgEndpointDisableInterval := comm.EndpointDisableInterval
	comm.EndpointDisableInterval = 0
	defer func() { comm.EndpointDisableInterval = orgEndpointDisableInterval }()

	sendBlock := func(seqNum uint64) *orderer.DeliverResponse {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{
					Header: &common.BlockHeader{
						Number:       seqNum,
						DataHash:     []byte{},
						PreviousHash: []byte{},
					},
					Data: &common.BlockData{
						Data: [][]byte{},
					},
				}},
		}
	}
	sendStatus := func(status common.Status) *orderer.DeliverResponse {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Status{
				Status: status,
			},
		}
	}

	bd := mocks.MockBlocksDeliverer{DisconnectCalled: make(chan struct{}, 10)}
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(nil)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64, 2)}
	provider := &blocksProviderImpl{
		chainID:              "***TEST_CHAINID***",
		gossip:               gossipServiceAdapter,
		client:               &bd,
		mcs:                  mcs,
		wrongStatusThreshold: wrongStatusThreshold,
	}

	attempts := int32(0)
	bd.MockRecv = func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		atomic.AddInt32(&attempts, 1)
		switch atomic.LoadInt32(&attempts) {
		case int32(1):
			return sendBlock(0), nil
		case int32(2):
			return sendStatus(common.Status_BAD_REQUEST), nil
		case int32(3):
			return sendStatus(common.Status_FORBIDDEN), nil
		case int32(4):
			return sendStatus(common.Status_NOT_FOUND), nil
		case int32(5):
			return sendStatus(common.Status_INTERNAL_SERVER_ERROR), nil
		case int32(6):
			return sendBlock(1), nil
		default:
			provider.Stop()
			return nil, errors.New("Stopping")
		}
	}

	go provider.DeliverBlocks()
	assert.Len(t, bd.DisconnectCalled, 0)
	for i := 0; i < 2; i++ {
		select {
		case seq := <-gossipServiceAdapter.GossipBlockDisseminations:
			assert.Equal(t, uint64(i), seq)
		case <-time.After(time.Second * 10):
			assert.Fail(t, "Didn't receive a block within a timely manner")
		}
	}
	// Make sure disconnect was called in between the deliveries
	assert.Len(t, bd.DisconnectCalled, 4)

}

func TestBlocksProvider_DeliveryWrongStatusClose(t *testing.T) {
	// Test emulates reception of all possible wrong status from the orderer
	// and ensures that after it receives the wrong status (and the threshold is reached),
	// the blocks provider is closed.

	orgEndpointDisableInterval := comm.EndpointDisableInterval
	comm.EndpointDisableInterval = 0
	defer func() { comm.EndpointDisableInterval = orgEndpointDisableInterval }()

	sendStatus := func(status common.Status) *orderer.DeliverResponse {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Status{
				Status: status,
			},
		}
	}

	bd := mocks.MockBlocksDeliverer{
		DisconnectCalled: make(chan struct{}, 100),
		CloseCalled:      make(chan struct{}, 1),
	}
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(nil)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64, 2)}
	provider := &blocksProviderImpl{
		chainID:              "***TEST_CHAINID***",
		gossip:               gossipServiceAdapter,
		client:               &bd,
		mcs:                  mcs,
		wrongStatusThreshold: 1,
	}

	incomingMsgs := make(chan *orderer.DeliverResponse)

	bd.MockRecv = func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		inMsg := <-incomingMsgs
		return inMsg, nil
	}

	go provider.DeliverBlocks()

	incomingMsgs <- sendStatus(common.Status_INTERNAL_SERVER_ERROR)
	incomingMsgs <- sendStatus(common.Status_NOT_FOUND)
	incomingMsgs <- sendStatus(common.Status_BAD_REQUEST)
	incomingMsgs <- sendStatus(common.Status_FORBIDDEN)

	waitUntilOrFail(t, func() bool {
		return len(bd.DisconnectCalled) == 3
	})

	waitUntilOrFail(t, func() bool {
		return len(bd.CloseCalled) == 1
	})
}

func TestBlockFetchFailure(t *testing.T) {
	rcvr := func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		return nil, errors.New("Failed fetching block")
	}
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(nil)
	makeTestCase(uint64(0), mcs, false, rcvr)(t)
}

func TestBlockVerificationFailure(t *testing.T) {
	attempts := int32(0)
	rcvr := func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		if atomic.LoadInt32(&attempts) == int32(1) {
			return &orderer.DeliverResponse{
				Type: &orderer.DeliverResponse_Status{
					Status: common.Status_SUCCESS,
				},
			}, nil
		}
		atomic.AddInt32(&attempts, int32(1))
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{
					Header: &common.BlockHeader{
						Number:       0,
						DataHash:     []byte{},
						PreviousHash: []byte{},
					},
					Data: &common.BlockData{
						Data: [][]byte{},
					},
				}},
		}, nil
	}
	mcs := &mockMCS{}
	mcs.On("VerifyBlock", mock.Anything).Return(errors.New("Invalid signature"))
	makeTestCase(uint64(0), mcs, false, rcvr)(t)
}
