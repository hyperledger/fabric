/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package blocksprovider

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
)

// Used to generate a simple test case to initialize delivery
// from given block sequence number.
func makeTestCase(ledgerHeight uint64) func(*testing.T) {
	return func(t *testing.T) {
		gossipServiceAdapter := &mocks.MockGossipServiceAdapter{}
		deliverer := &mocks.MockBlocksDeliverer{Pos: ledgerHeight}
		deliverer.MockRecv = mocks.MockRecv

		provider := &blocksProviderImpl{
			chainID: "***TEST_CHAINID***",
			gossip:  gossipServiceAdapter,
			client:  deliverer,
		}

		provider.RequestBlocks(&mocks.MockLedgerInfo{ledgerHeight})

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
				// Check that all blocks received eventually get gossiped and locally committed
				assert.True(t, deliverer.RecvCnt == gossipServiceAdapter.AddPayloadsCnt)
				assert.True(t, deliverer.RecvCnt == gossipServiceAdapter.GossipCallsCnt)
				return
			}
		case <-time.After(time.Duration(1) * time.Second):
			{
				t.Fatal("Test hasn't finished in timely manner, failing.")
			}
		}
	}
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromTheOldest(t *testing.T) {
	makeTestCase(uint64(0))(t)
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromSpecified(t *testing.T) {
	makeTestCase(uint64(101))(t)
}

func TestBlocksProvider_CheckTerminationDeliveryResponseStatus(t *testing.T) {

	tmp := struct{ mocks.MockBlocksDeliverer }{}

	// Making mocked Recv() function to return DeliverResponse_Status to force block
	// provider to fail and exit, cheking that in that case to block was actually
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

	provider.RequestBlocks(&mocks.MockLedgerInfo{0})

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
			assert.Equal(t, int32(1), tmp.RecvCnt)
			// No payload should commit locally
			assert.Equal(t, int32(0), gossipServiceAdapter.AddPayloadsCnt)
			// No payload should be transfered to other peers
			assert.Equal(t, int32(0), gossipServiceAdapter.GossipCallsCnt)
			return
		}
	case <-time.After(time.Duration(1) * time.Second):
		{
			t.Fatal("Test hasn't finished in timely manner, failing.")
		}
	}
}
