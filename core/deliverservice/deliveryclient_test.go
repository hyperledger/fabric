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
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func init() {
	msptesttools.LoadMSPSetupForTesting()
}

const (
	goRoutineTestWaitTimeout = time.Second * 15
)

var (
	lock = sync.Mutex{}
)

type mockBlocksDelivererFactory struct {
	mockCreate func() (blocksprovider.BlocksDeliverer, error)
}

func (mock *mockBlocksDelivererFactory) Create() (blocksprovider.BlocksDeliverer, error) {
	return mock.mockCreate()
}

type mockMCS struct {
}

func (*mockMCS) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType("pkiID")
}

func (*mockMCS) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	return nil
}

func (*mockMCS) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*mockMCS) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

func TestNewDeliverService(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64, 1)}
	factory := &struct{ mockBlocksDelivererFactory }{}

	blocksDeliverer := &mocks.MockBlocksDeliverer{}
	blocksDeliverer.MockRecv = mocks.MockRecv

	factory.mockCreate = func() (blocksprovider.BlocksDeliverer, error) {
		return blocksDeliverer, nil
	}
	abcf := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return &mocks.MockAtomicBroadcastClient{blocksDeliverer}
	}

	connFactory := func(_ string) func(string) (*grpc.ClientConn, error) {
		return func(endpoint string) (*grpc.ClientConn, error) {
			lock.Lock()
			defer lock.Unlock()
			return newConnection(), nil
		}
	}
	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"a"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  abcf,
		ConnFactory: connFactory,
	})
	assert.NoError(t, err)
	assert.NoError(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{0}))

	// Lets start deliver twice
	assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{0}), "can't start delivery")
	// Lets stop deliver that not started
	assert.Error(t, service.StopDeliverForChannel("TEST_CHAINID2"), "can't stop delivery")

	// Let it try to simulate a few recv -> gossip rounds
	time.Sleep(time.Second)
	assert.NoError(t, service.StopDeliverForChannel("TEST_CHAINID"))
	time.Sleep(time.Duration(10) * time.Millisecond)
	// Make sure to stop all blocks providers
	service.Stop()
	time.Sleep(time.Duration(500) * time.Millisecond)
	assert.Equal(t, 0, connNumber)
	assertBlockDissemination(0, gossipServiceAdapter.GossipBlockDisseminations, t)
	assert.Equal(t, atomic.LoadInt32(&blocksDeliverer.RecvCnt), atomic.LoadInt32(&gossipServiceAdapter.AddPayloadsCnt))
	assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{0}), "Delivery service is stopping")
	assert.Error(t, service.StopDeliverForChannel("TEST_CHAINID"), "Delivery service is stopping")
}

func TestDeliverServiceRestart(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// Scenario: bring up ordering service instance, then shut it down, and then resurrect it.
	// Client is expected to reconnect to it, and to ask for a block sequence that is the next block
	// after the last block it got from the previous incarnation of the ordering service.

	os := mocks.NewOrderer(5611, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5611"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)

	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	os.SetNextExpectedSeek(uint64(100))

	err = service.StartDeliverForChannel("TEST_CHAINID", li)
	assert.NoError(t, err, "can't start delivery")
	// Check that delivery client requests blocks in order
	go os.SendBlock(uint64(100))
	assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
	go os.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
	go os.SendBlock(uint64(102))
	assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
	os.Shutdown()
	time.Sleep(time.Second * 3)
	os = mocks.NewOrderer(5611, t)
	li.Height = 103
	os.SetNextExpectedSeek(uint64(103))
	go os.SendBlock(uint64(103))
	assertBlockDissemination(103, gossipServiceAdapter.GossipBlockDisseminations, t)
	service.Stop()
	os.Shutdown()
}

func TestDeliverServiceFailover(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// Scenario: bring up 2 ordering service instances,
	// and shut down the instance that the client has connected to.
	// Client is expected to connect to the other instance, and to ask for a block sequence that is the next block
	// after the last block it got from the ordering service that was shut down.
	// Then, shut down the other node, and bring back the first (that was shut down first).

	os1 := mocks.NewOrderer(5612, t)
	os2 := mocks.NewOrderer(5613, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5612", "localhost:5613"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)
	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	os1.SetNextExpectedSeek(uint64(100))
	os2.SetNextExpectedSeek(uint64(100))

	err = service.StartDeliverForChannel("TEST_CHAINID", li)
	assert.NoError(t, err, "can't start delivery")
	// We need to discover to which instance the client connected to
	go os1.SendBlock(uint64(100))
	instance2fail := os1
	reincarnatedNodePort := 5612
	instance2failSecond := os2
	select {
	case seq := <-gossipServiceAdapter.GossipBlockDisseminations:
		assert.Equal(t, uint64(100), seq)
	case <-time.After(time.Second * 2):
		// Shutdown first instance and replace it, in order to make an instance
		// with an empty sending channel
		os1.Shutdown()
		time.Sleep(time.Second)
		os1 = mocks.NewOrderer(5612, t)
		instance2fail = os2
		instance2failSecond = os1
		reincarnatedNodePort = 5613
		// Ensure we really are connected to the second instance,
		// by making it send a block
		go os2.SendBlock(uint64(100))
		assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
	}

	atomic.StoreUint64(&li.Height, uint64(101))
	os1.SetNextExpectedSeek(uint64(101))
	os2.SetNextExpectedSeek(uint64(101))
	// Fail the orderer node the client is connected to
	instance2fail.Shutdown()
	time.Sleep(time.Second)
	// Ensure the client asks blocks from the other ordering service node
	go instance2failSecond.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
	atomic.StoreUint64(&li.Height, uint64(102))
	// Now shut down the 2nd node
	instance2failSecond.Shutdown()
	time.Sleep(time.Second * 1)
	// Bring up the first one
	os := mocks.NewOrderer(reincarnatedNodePort, t)
	os.SetNextExpectedSeek(102)
	go os.SendBlock(uint64(102))
	assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
	os.Shutdown()
	service.Stop()
}

func TestDeliverServiceServiceUnavailable(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// Scenario: bring up 2 ordering service instances,
	// Make the instance the client connects to fail after a delivery of a block and send SERVICE_UNAVAILABLE
	// whenever subsequent seeks are sent to it.
	// The client is expected to connect to the other instance, and to ask for a block sequence that is the next block
	// after the last block it got from the first ordering service node.

	os1 := mocks.NewOrderer(5615, t)
	os2 := mocks.NewOrderer(5616, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5615", "localhost:5616"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)
	li := &mocks.MockLedgerInfo{Height: 100}
	os1.SetNextExpectedSeek(100)
	os2.SetNextExpectedSeek(100)

	err = service.StartDeliverForChannel("TEST_CHAINID", li)
	assert.NoError(t, err, "can't start delivery")
	// We need to discover to which instance the client connected to
	go os1.SendBlock(100)
	// Is it the first instance?
	instance2fail := os1
	nextBlockSeek := uint64(100)
	select {
	case seq := <-gossipServiceAdapter.GossipBlockDisseminations:
		// Just for sanity check, ensure we got block seq 100
		assert.Equal(t, uint64(100), seq)
		// Connected to the first instance
		// Advance ledger's height by 1
		atomic.StoreUint64(&li.Height, 101)
		// Backup instance should expect a seek of 101 since we got 100
		os2.SetNextExpectedSeek(uint64(101))
		nextBlockSeek = uint64(101)
		// Have backup instance prepare to send a block
		os2.SendBlock(101)
	case <-time.After(time.Second * 5):
		// We didn't get a block on time, so seems like we're connected to the 2nd instance
		// and not to the first.
		instance2fail = os2
	}

	instance2fail.Fail()
	// Ensure the client asks blocks from the other ordering service node
	assertBlockDissemination(nextBlockSeek, gossipServiceAdapter.GossipBlockDisseminations, t)

	// Cleanup
	os1.Shutdown()
	os2.Shutdown()
	service.Stop()
}

func TestDeliverServiceShutdown(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Then, shut down the client, and check that it is no longer fetching blocks.
	os := mocks.NewOrderer(5614, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5614"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)

	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	os.SetNextExpectedSeek(uint64(100))
	err = service.StartDeliverForChannel("TEST_CHAINID", li)
	assert.NoError(t, err, "can't start delivery")

	// Check that delivery service requests blocks in order
	go os.SendBlock(uint64(100))
	assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
	go os.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
	atomic.StoreUint64(&li.Height, uint64(102))
	os.SetNextExpectedSeek(uint64(102))
	// Now stop the delivery service and make sure we don't disseminate a block
	service.Stop()
	go os.SendBlock(uint64(102))
	select {
	case <-gossipServiceAdapter.GossipBlockDisseminations:
		assert.Fail(t, "Disseminated a block after shutting down the delivery service")
	case <-time.After(time.Second * 2):
	}
	os.Shutdown()
	time.Sleep(time.Second)
}

func TestDeliverServiceBadConfig(t *testing.T) {
	// Empty endpoints
	service, err := NewDeliverService(&Config{
		Endpoints:   []string{},
		Gossip:      &mocks.MockGossipServiceAdapter{},
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.Error(t, err)
	assert.Nil(t, service)

	// Nil gossip adapter
	service, err = NewDeliverService(&Config{
		Endpoints:   []string{"a"},
		Gossip:      nil,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.Error(t, err)
	assert.Nil(t, service)

	// Nil crypto service
	service, err = NewDeliverService(&Config{
		Endpoints:   []string{"a"},
		Gossip:      &mocks.MockGossipServiceAdapter{},
		CryptoSvc:   nil,
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.Error(t, err)
	assert.Nil(t, service)

	// Nil ABCFactory
	service, err = NewDeliverService(&Config{
		Endpoints:   []string{"a"},
		Gossip:      &mocks.MockGossipServiceAdapter{},
		CryptoSvc:   &mockMCS{},
		ABCFactory:  nil,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.Error(t, err)
	assert.Nil(t, service)

	// Nil connFactory
	service, err = NewDeliverService(&Config{
		Endpoints:  []string{"a"},
		Gossip:     &mocks.MockGossipServiceAdapter{},
		CryptoSvc:  &mockMCS{},
		ABCFactory: DefaultABCFactory,
	})
	assert.Error(t, err)
	assert.Nil(t, service)
}

func TestRetryPolicyOverflow(t *testing.T) {
	connFactory := func(channelID string) func(endpoint string) (*grpc.ClientConn, error) {
		return func(_ string) (*grpc.ClientConn, error) {
			return nil, errors.New("")
		}
	}
	client := (&deliverServiceImpl{conf: &Config{ConnFactory: connFactory}}).newClient("TEST", &mocks.MockLedgerInfo{Height: uint64(100)})
	assert.NotNil(t, client.shouldRetry)
	for i := 0; i < 100; i++ {
		retryTime, _ := client.shouldRetry(i, time.Second)
		assert.True(t, retryTime <= time.Hour && retryTime > 0)
	}
}

func assertBlockDissemination(expectedSeq uint64, ch chan uint64, t *testing.T) {
	select {
	case seq := <-ch:
		assert.Equal(t, expectedSeq, seq)
	case <-time.After(time.Second * 5):
		assert.Fail(t, "Didn't gossip a new block within a timely manner")
	}
}

func ensureNoGoroutineLeak(t *testing.T) func() {
	goroutineCountAtStart := runtime.NumGoroutine()
	return func() {
		start := time.Now()
		timeLimit := start.Add(goRoutineTestWaitTimeout)
		for time.Now().Before(timeLimit) {
			time.Sleep(time.Millisecond * 500)
			if goroutineCountAtStart >= runtime.NumGoroutine() {
				return
			}
		}
		assert.Fail(t, "Some goroutine(s) didn't finish: %s", getStackTrace())
	}
}

func getStackTrace() string {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	return string(buf)
}
