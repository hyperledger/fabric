/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
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

func (*mockMCS) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
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
		return &mocks.MockAtomicBroadcastClient{
			BD: blocksDeliverer,
		}
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
	assert.NoError(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}))

	// Lets start deliver twice
	assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}), "can't start delivery")
	// Lets stop deliver that not started
	assert.Error(t, service.StopDeliverForChannel("TEST_CHAINID2"), "can't stop delivery")

	// Let it try to simulate a few recv -> gossip rounds
	time.Sleep(time.Second)
	assert.NoError(t, service.StopDeliverForChannel("TEST_CHAINID"))
	time.Sleep(time.Duration(10) * time.Millisecond)
	// Make sure to stop all blocks providers
	service.Stop()
	time.Sleep(time.Duration(500) * time.Millisecond)
	connWG.Wait()

	assertBlockDissemination(0, gossipServiceAdapter.GossipBlockDisseminations, t)
	assert.Equal(t, atomic.LoadInt32(&blocksDeliverer.RecvCnt), atomic.LoadInt32(&gossipServiceAdapter.AddPayloadsCnt))
	assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}), "Delivery service is stopping")
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

	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
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
	atomic.StoreUint64(&li.Height, uint64(103))
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

	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
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

func TestDeliverServiceUpdateEndpoints(t *testing.T) {
	// TODO: Add test case to check the endpoints update
	// Case: Start service with given ordering service endpoint
	// send a block, switch to new endpoint and send a new block
	// Expected: Delivery service should be able to switch to
	// updated endpoint and receive second block within timely manner.
	defer ensureNoGoroutineLeak(t)()

	os1 := mocks.NewOrderer(5612, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5612"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	defer service.Stop()

	assert.NoError(t, err)
	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	os1.SetNextExpectedSeek(uint64(100))

	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
	assert.NoError(t, err, "can't start delivery")

	go os1.SendBlock(uint64(100))
	assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)

	os2 := mocks.NewOrderer(5613, t)
	defer os2.Shutdown()
	os2.SetNextExpectedSeek(uint64(101))

	service.UpdateEndpoints("TEST_CHAINID", []string{"localhost:5613"})
	// Shutdown old ordering service to make sure block will now come from
	// updated ordering service
	os1.Shutdown()

	atomic.StoreUint64(&li.Height, uint64(101))
	go os2.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
}

func TestDeliverServiceServiceUnavailable(t *testing.T) {
	orgEndpointDisableInterval := comm.EndpointDisableInterval
	comm.EndpointDisableInterval = time.Millisecond * 1500
	defer func() { comm.EndpointDisableInterval = orgEndpointDisableInterval }()
	defer ensureNoGoroutineLeak(t)()
	// Scenario: bring up 2 ordering service instances,
	// Make the instance the client connects to fail after a delivery of a block and send SERVICE_UNAVAILABLE
	// whenever subsequent seeks are sent to it.
	// The client is expected to connect to the other instance, and to ask for a block sequence that is the next block
	// after the last block it got from the first ordering service node.
	// Wait endpoint disable interval
	// After that resurrect failed node (first node) and fail instance client currently connect - send SERVICE_UNAVAILABLE
	// The client should reconnect to original instance and ask for next block.

	os1 := mocks.NewOrderer(5615, t)
	os2 := mocks.NewOrderer(5616, t)

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
	os1.SetNextExpectedSeek(li.Height)
	os2.SetNextExpectedSeek(li.Height)

	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
	assert.NoError(t, err, "can't start delivery")

	waitForConnectionToSomeOSN := func() (*mocks.Orderer, *mocks.Orderer) {
		for {
			if os1.ConnCount() > 0 {
				return os1, os2
			}
			if os2.ConnCount() > 0 {
				return os2, os1
			}
			time.Sleep(time.Millisecond * 100)
		}
	}

	activeInstance, backupInstance := waitForConnectionToSomeOSN()
	assert.NotNil(t, activeInstance)
	assert.NotNil(t, backupInstance)
	// Check that delivery client get connected to active
	assert.Equal(t, activeInstance.ConnCount(), 1)
	// and not connected to backup instances
	assert.Equal(t, backupInstance.ConnCount(), 0)

	// Send first block
	go activeInstance.SendBlock(li.Height)

	assertBlockDissemination(li.Height, gossipServiceAdapter.GossipBlockDisseminations, t)
	li.Height++

	// Backup instance should expect a seek of 101 since we got 100
	backupInstance.SetNextExpectedSeek(li.Height)
	// Have backup instance prepare to send a block
	backupInstance.SendBlock(li.Height)

	// Fail instance delivery client connected to
	activeInstance.Fail()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-time.After(time.Millisecond * 100):
				if backupInstance.ConnCount() > 0 {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}(ctx)

	wg.Wait()
	assert.NoError(t, ctx.Err(), "Delivery client has not failed over to alive ordering service")
	// Check that delivery client was indeed connected
	assert.Equal(t, backupInstance.ConnCount(), 1)
	// Ensure the client asks blocks from the other ordering service node
	assertBlockDissemination(li.Height, gossipServiceAdapter.GossipBlockDisseminations, t)

	// Wait until first endpoint enabled again
	time.Sleep(time.Millisecond * 1600)

	li.Height++
	activeInstance.Resurrect()
	backupInstance.Fail()

	resurrectCtx, resCancel := context.WithTimeout(context.Background(), time.Second)
	defer resCancel()

	go func() {
		// Resurrected instance should expect a seek of 102 since we got 101
		activeInstance.SetNextExpectedSeek(li.Height)
		// Have resurrected instance prepare to send a block
		activeInstance.SendBlock(li.Height)

	}()

	reswg := sync.WaitGroup{}
	reswg.Add(1)

	go func() {
		defer reswg.Done()
		for {
			select {
			case <-time.After(time.Millisecond * 100):
				if activeInstance.ConnCount() > 0 {
					return
				}
			case <-resurrectCtx.Done():
				return
			}
		}
	}()

	reswg.Wait()

	assert.NoError(t, resurrectCtx.Err(), "Delivery client has not failed over to alive ordering service")
	// Check that delivery client was indeed connected
	assert.Equal(t, activeInstance.ConnCount(), 1)
	// Ensure the client asks blocks from the other ordering service node
	assertBlockDissemination(li.Height, gossipServiceAdapter.GossipBlockDisseminations, t)

	// Cleanup
	os1.Shutdown()
	os2.Shutdown()
	service.Stop()
}

func TestDeliverServiceAbruptStop(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// Scenario: The deliver service is started and abruptly stopped.
	// The block provider instance is run in a separate goroutine, and thus
	// it might be scheduled after the deliver client is stopped.
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}
	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"a"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)

	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	service.StartDeliverForChannel("mychannel", li, func() {})
	service.StopDeliverForChannel("mychannel")
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
	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
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

func TestDeliverServiceShutdownRespawn(t *testing.T) {
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Then, wait a few seconds, and don't send any blocks.
	// Afterwards - start a new instance and shut down the old instance.
	viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", time.Second)
	defer viper.Reset()
	defer ensureNoGoroutineLeak(t)()

	osn1 := mocks.NewOrderer(5614, t)

	time.Sleep(time.Second)
	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

	service, err := NewDeliverService(&Config{
		Endpoints:   []string{"localhost:5614", "localhost:5615"},
		Gossip:      gossipServiceAdapter,
		CryptoSvc:   &mockMCS{},
		ABCFactory:  DefaultABCFactory,
		ConnFactory: DefaultConnectionFactory,
	})
	assert.NoError(t, err)

	li := &mocks.MockLedgerInfo{Height: uint64(100)}
	osn1.SetNextExpectedSeek(uint64(100))
	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
	assert.NoError(t, err, "can't start delivery")

	// Check that delivery service requests blocks in order
	go osn1.SendBlock(uint64(100))
	assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
	go osn1.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
	atomic.StoreUint64(&li.Height, uint64(102))
	// Now wait for a few seconds
	time.Sleep(time.Second * 2)
	// Now start the new instance
	osn2 := mocks.NewOrderer(5615, t)
	// Now stop the old instance
	osn1.Shutdown()
	// Send a block from osn2
	osn2.SetNextExpectedSeek(uint64(102))
	go osn2.SendBlock(uint64(102))
	// Ensure it is received
	assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
	service.Stop()
	osn2.Shutdown()
}

func TestDeliverServiceDisconnectReconnect(t *testing.T) {
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Stop ordering service, wait for while - simulate disconnect and restart it back.
	// Wait for some time, without sending blocks - simulate recv wait on empty channel.
	// Repeat stop/start sequence multiple times, to make sure total retry time will pass
	// value returned by getReConnectTotalTimeThreshold - in test it set to 2 seconds
	// (0.5s + 1s + 2s + 4s) > 2s.
	// Send new block and check that delivery client got it.
	// So, we can see that waiting on recv in empty channel do reset total time spend in reconnection.
	viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", time.Second*2)
	defer viper.Reset()
	defer ensureNoGoroutineLeak(t)()

	osn := mocks.NewOrderer(5614, t)

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
	osn.SetNextExpectedSeek(uint64(100))
	err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
	assert.NoError(t, err, "can't start delivery")

	// Check that delivery service requests blocks in order
	go osn.SendBlock(uint64(100))
	assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
	go osn.SendBlock(uint64(101))
	assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
	atomic.StoreUint64(&li.Height, uint64(102))

	for i := 0; i < 5; i += 1 {
		// Shutdown orderer, simulate network disconnect
		osn.Shutdown()
		// Now wait for a disconnect to be discovered
		assert.True(t, waitForConnectionCount(osn, 0), "deliverService can't disconnect from orderer")
		// Recreate orderer, simulating network is back
		osn = mocks.NewOrderer(5614, t)
		osn.SetNextExpectedSeek(atomic.LoadUint64(&li.Height))
		// Now wait for a while, to client connect back and simulate empty channel
		assert.True(t, waitForConnectionCount(osn, 1), "deliverService can't reconnect to orderer")
	}

	// Send a block from orderer
	go osn.SendBlock(uint64(102))
	// Ensure it is received
	assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
	service.Stop()
	osn.Shutdown()
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
		assert.FailNow(t, fmt.Sprintf("Didn't gossip a new block with seq num %d within a timely manner", expectedSeq))
		t.Fatal()
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

func waitForConnectionCount(orderer *mocks.Orderer, connCount int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	for {
		select {
		case <-time.After(time.Millisecond * 100):
			if orderer.ConnCount() == connCount {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}
