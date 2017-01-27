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

package deliverclient

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/pkg/testutil/assert"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/spf13/viper"
)

type mockBlocksDelivererFactory struct {
	mockCreate func() (blocksprovider.BlocksDeliverer, error)
}

func (mock *mockBlocksDelivererFactory) Create() (blocksprovider.BlocksDeliverer, error) {
	return mock.mockCreate()
}

func TestNewDeliverService(t *testing.T) {
	viper.Set("peer.gossip.orgLeader", true)

	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{}
	factory := &struct{ mockBlocksDelivererFactory }{}

	blocksDeliverer := &mocks.MockBlocksDeliverer{}
	blocksDeliverer.MockRecv = mocks.MockRecv

	factory.mockCreate = func() (blocksprovider.BlocksDeliverer, error) {
		return blocksDeliverer, nil
	}

	service := NewFactoryDeliverService(gossipServiceAdapter, factory, nil)
	service.JoinChain("TEST_CHAINID", &mocks.MockLedgerInfo{0})

	// Let it try to simulate a few recv -> gossip rounds
	time.Sleep(time.Duration(10) * time.Millisecond)
	service.Stop()

	// Make sure to stop all blocks providers
	time.Sleep(time.Duration(500) * time.Millisecond)

	assert.Equal(t, atomic.LoadInt32(&blocksDeliverer.RecvCnt), atomic.LoadInt32(&gossipServiceAdapter.AddPayloadsCnt))
	assert.Equal(t, atomic.LoadInt32(&blocksDeliverer.RecvCnt), atomic.LoadInt32(&gossipServiceAdapter.GossipCallsCnt))

}
