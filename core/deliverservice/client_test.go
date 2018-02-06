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
	"crypto/sha256"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var connNumber = 0

func newConnection() *grpc.ClientConn {
	connNumber++
	// The balancer is in order to check connection leaks.
	// When grpc.ClientConn.Close() is called, it calls the balancer's Close()
	// method which decrements the connNumber
	cc, _ := grpc.Dial("", grpc.WithInsecure(), grpc.WithBalancer(&balancer{}))
	return cc
}

type balancer struct {
}

func (*balancer) Start(target string, config grpc.BalancerConfig) error {
	return nil
}

func (*balancer) Up(addr grpc.Address) (down func(error)) {
	return func(error) {}
}

func (*balancer) Get(ctx context.Context, opts grpc.BalancerGetOptions) (addr grpc.Address, put func(), err error) {
	return grpc.Address{}, func() {}, errors.New("")
}

func (*balancer) Notify() <-chan []grpc.Address {
	return nil
}

func (*balancer) Close() error {
	connNumber--
	return nil
}

type blocksDelivererConsumer func(blocksprovider.BlocksDeliverer) error

var blockDelivererConsumerWithRecv = func(bd blocksprovider.BlocksDeliverer) error {
	_, err := bd.Recv()
	return err
}

var blockDelivererConsumerWithSend = func(bd blocksprovider.BlocksDeliverer) error {
	return bd.Send(&common.Envelope{})
}

type abc struct {
	shouldFail bool
	grpc.ClientStream
}

func (a *abc) Send(*common.Envelope) error {
	if a.shouldFail {
		return errors.New("Failed sending")
	}
	return nil
}

func (a *abc) Recv() (*orderer.DeliverResponse, error) {
	if a.shouldFail {
		return nil, errors.New("Failed Recv")
	}
	return &orderer.DeliverResponse{}, nil
}

type abclient struct {
	shouldFail bool
	stream     *abc
}

func (ac *abclient) Broadcast(ctx context.Context, opts ...grpc.CallOption) (orderer.AtomicBroadcast_BroadcastClient, error) {
	panic("Not implemented")
}

func (ac *abclient) Deliver(ctx context.Context, opts ...grpc.CallOption) (orderer.AtomicBroadcast_DeliverClient, error) {
	if ac.stream != nil {
		return ac.stream, nil
	}
	if ac.shouldFail {
		return nil, errors.New("Failed creating ABC")
	}
	return &abc{}, nil
}

type connProducer struct {
	shouldFail      bool
	connAttempts    int
	connTime        time.Duration
	ordererEndpoint string
}

func (cp *connProducer) realConnection() (*grpc.ClientConn, string, error) {
	cc, err := grpc.Dial(cp.ordererEndpoint, grpc.WithInsecure())
	if err != nil {
		return nil, "", err
	}
	return cc, cp.ordererEndpoint, nil
}

func (cp *connProducer) NewConnection() (*grpc.ClientConn, string, error) {
	time.Sleep(cp.connTime)
	cp.connAttempts++
	if cp.ordererEndpoint != "" {
		return cp.realConnection()
	}
	if cp.shouldFail {
		return nil, "", errors.New("Failed connecting")
	}
	return newConnection(), "localhost:5611", nil
}

// UpdateEndpoints updates the endpoints of the ConnectionProducer
// to be the given endpoints
func (cp *connProducer) UpdateEndpoints(endpoints []string) {
	panic("Not implemented")
}

func TestOrderingServiceConnFailure(t *testing.T) {
	testOrderingServiceConnFailure(t, blockDelivererConsumerWithRecv)
	testOrderingServiceConnFailure(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServiceConnFailure(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Create a broadcast client and call Recv/Send.
	// Connection to the ordering service should be possible only at second attempt
	cp := &connProducer{shouldFail: true}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return &abclient{}
	}
	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		// When called with the second attempt (iteration number 1),
		// we should be able to connect to the ordering service.
		// Set connection provider mock shouldFail flag to false
		// to enable next connection attempt to succeed
		if attemptNum == 1 {
			cp.shouldFail = false
		}

		return time.Duration(0), attemptNum < 2
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 2, cp.connAttempts)
	assert.Equal(t, 1, setupInvoked)
}

func TestOrderingServiceStreamFailure(t *testing.T) {
	testOrderingServiceStreamFailure(t, blockDelivererConsumerWithRecv)
	testOrderingServiceStreamFailure(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServiceStreamFailure(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Create a broadcast client and call Recv/Send.
	// Connection to the ordering service should be possible at first attempt,
	// but the atomic broadcast client creation fails at first attempt
	abcClient := &abclient{shouldFail: true}
	cp := &connProducer{}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}
	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		// When called with the second attempt (iteration number 1),
		// we should be able to finally call Deliver() by the atomic broadcast client
		if attemptNum == 1 {
			abcClient.shouldFail = false
		}
		return time.Duration(0), attemptNum < 2
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 2, cp.connAttempts)
	assert.Equal(t, 1, setupInvoked)
}

func TestOrderingServiceSetupFailure(t *testing.T) {
	testOrderingServiceSetupFailure(t, blockDelivererConsumerWithRecv)
	testOrderingServiceSetupFailure(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServiceSetupFailure(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Create a broadcast client and call Recv/Send.
	// Connection to the ordering service should be possible,
	// the atomic broadcast client creation succeeds, but invoking the setup function
	// fails at the first attempt and succeeds at the second one.
	cp := &connProducer{}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return &abclient{}
	}
	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		if setupInvoked == 1 {
			return errors.New("Setup failed")
		}
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), true
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 2, cp.connAttempts)
	assert.Equal(t, 2, setupInvoked)
}

func TestOrderingServiceFirstOperationFailure(t *testing.T) {
	testOrderingServiceFirstOperationFailure(t, blockDelivererConsumerWithRecv)
	testOrderingServiceFirstOperationFailure(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServiceFirstOperationFailure(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Creation and connection to the ordering service succeeded
	// but the first Recv/Send failed.
	// The client should reconnect to the ordering service
	cp := &connProducer{}
	abStream := &abc{shouldFail: true}
	abcClient := &abclient{stream: abStream}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		// Fix stream success logic at 2nd attempt
		if setupInvoked == 1 {
			abStream.shouldFail = false
		}
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), true
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 2, setupInvoked)
	assert.Equal(t, cp.connAttempts, 2)
}

func TestOrderingServiceCrashAndRecover(t *testing.T) {
	testOrderingServiceCrashAndRecover(t, blockDelivererConsumerWithRecv)
	testOrderingServiceCrashAndRecover(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServiceCrashAndRecover(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: The ordering service is OK at first usage of Recv/Send,
	// but subsequent calls fails because of connection error.
	// A reconnect is needed and only then Recv/Send should succeed
	cp := &connProducer{}
	abStream := &abc{}
	abcClient := &abclient{stream: abStream}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		// Fix stream success logic at 2nd attempt
		if setupInvoked == 1 {
			abStream.shouldFail = false
		}
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), true
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	// Now fail the subsequent Recv/Send
	abStream.shouldFail = true
	err = bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 2, cp.connAttempts)
	assert.Equal(t, 2, setupInvoked)
}

func TestOrderingServicePermanentCrash(t *testing.T) {
	testOrderingServicePermanentCrash(t, blockDelivererConsumerWithRecv)
	testOrderingServicePermanentCrash(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testOrderingServicePermanentCrash(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: The ordering service is OK at first usage of Recv/Send,
	// but subsequent calls fail because it crashes.
	// The client should give up after 10 attempts in the second reconnect
	cp := &connProducer{}
	abStream := &abc{}
	abcClient := &abclient{stream: abStream}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), attemptNum < 10
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	// Now fail the subsequent Recv/Send
	abStream.shouldFail = true
	cp.shouldFail = true
	err = bdc(bc)
	assert.Error(t, err)
	assert.Equal(t, 10, cp.connAttempts)
	assert.Equal(t, 1, setupInvoked)
}

func TestLimitedConnAttempts(t *testing.T) {
	testLimitedConnAttempts(t, blockDelivererConsumerWithRecv)
	testLimitedConnAttempts(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testLimitedConnAttempts(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: The ordering service isn't available, and the backoff strategy
	// specifies an upper bound of 10 connection attempts
	cp := &connProducer{shouldFail: true}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return &abclient{}
	}
	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), attemptNum < 10
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.Error(t, err)
	assert.Equal(t, 10, cp.connAttempts)
	assert.Equal(t, 0, setupInvoked)
}

func TestLimitedTotalConnTimeRcv(t *testing.T) {
	testLimitedTotalConnTime(t, blockDelivererConsumerWithRecv)
	assert.Equal(t, 0, connNumber)
}

func TestLimitedTotalConnTimeSnd(t *testing.T) {
	testLimitedTotalConnTime(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testLimitedTotalConnTime(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: The ordering service isn't available, and the backoff strategy
	// specifies an upper bound of 1 second
	// The first attempt fails, and the second attempt doesn't take place
	// becuse the creation of connection takes 1.5 seconds.
	cp := &connProducer{shouldFail: true, connTime: 1500 * time.Millisecond}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return &abclient{}
	}
	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Millisecond * 500, elapsedTime.Nanoseconds() < time.Second.Nanoseconds()
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.Error(t, err)
	assert.Equal(t, 3, cp.connAttempts)
	assert.Equal(t, 0, setupInvoked)
}

func TestGreenPath(t *testing.T) {
	testGreenPath(t, blockDelivererConsumerWithRecv)
	testGreenPath(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testGreenPath(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Everything succeeds
	cp := &connProducer{}
	abcClient := &abclient{}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Duration(0), attemptNum < 1
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	defer bc.Close()
	err := bdc(bc)
	assert.NoError(t, err)
	assert.Equal(t, 1, cp.connAttempts)
	assert.Equal(t, 1, setupInvoked)
}

func TestCloseWhileRecv(t *testing.T) {
	// Scenario: Recv is being called and after a while,
	// the connection is closed.
	// The Recv should return immediately in such a case
	fakeOrderer := mocks.NewOrderer(5611, t)
	time.Sleep(time.Second)
	defer fakeOrderer.Shutdown()
	cp := &connProducer{ordererEndpoint: "localhost:5611"}
	clFactory := func(conn *grpc.ClientConn) orderer.AtomicBroadcastClient {
		return orderer.NewAtomicBroadcastClient(conn)
	}

	setup := func(blocksprovider.BlocksDeliverer) error {
		return nil
	}
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return 0, true
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	var flag int32
	go func() {
		for fakeOrderer.ConnCount() == 0 {
			time.Sleep(time.Second)
		}
		atomic.StoreInt32(&flag, int32(1))
		bc.Close()
		bc.Close() // Try to close a second time
	}()
	resp, err := bc.Recv()
	// Ensure we returned because bc.Close() was called and not because some other reason
	assert.Equal(t, int32(1), atomic.LoadInt32(&flag), "Recv returned before bc.Close() was called")
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, "Client is closing", err.Error())
}

func TestCloseWhileSleep(t *testing.T) {
	testCloseWhileSleep(t, blockDelivererConsumerWithRecv)
	testCloseWhileSleep(t, blockDelivererConsumerWithSend)
	assert.Equal(t, 0, connNumber)
}

func testCloseWhileSleep(t *testing.T, bdc blocksDelivererConsumer) {
	// Scenario: Recv/Send is being called while sleeping because
	// of the backoff policy is programmed to sleep 1000000 seconds
	// between retries.
	// The Recv/Send should return pretty soon
	cp := &connProducer{}
	abcClient := &abclient{shouldFail: true}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	setupInvoked := 0
	setup := func(blocksprovider.BlocksDeliverer) error {
		setupInvoked++
		return nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	backoffStrategy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		if attemptNum == 1 {
			go func() {
				time.Sleep(time.Second)
				wg.Done()
			}()
		}
		return time.Second * 1000000, true
	}
	bc := NewBroadcastClient(cp, clFactory, setup, backoffStrategy)
	go func() {
		wg.Wait()
		bc.Close()
		bc.Close() // Try to close a second time
	}()
	err := bdc(bc)
	assert.Error(t, err)
	assert.Equal(t, 1, cp.connAttempts)
	assert.Equal(t, 0, setupInvoked)
}

type signerMock struct {
}

func (s *signerMock) NewSignatureHeader() (*common.SignatureHeader, error) {
	return &common.SignatureHeader{}, nil
}

func (s *signerMock) Sign(message []byte) ([]byte, error) {
	hasher := sha256.New()
	hasher.Write(message)
	return hasher.Sum(nil), nil
}

func TestProductionUsage(t *testing.T) {
	defer ensureNoGoroutineLeak(t)()
	// This test configures the client in a similar fashion as will be
	// in production, and tests against a live gRPC server.
	os := mocks.NewOrderer(5612, t)
	os.SetNextExpectedSeek(5)

	connFact := func(endpoint string) (*grpc.ClientConn, error) {
		return grpc.Dial(endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	prod := comm.NewConnectionProducer(connFact, []string{"localhost:5612"})
	clFact := func(cc *grpc.ClientConn) orderer.AtomicBroadcastClient {
		return orderer.NewAtomicBroadcastClient(cc)
	}
	onConnect := func(bd blocksprovider.BlocksDeliverer) error {
		env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE,
			"TEST",
			&signerMock{}, newTestSeekInfo(), 0, 0)
		assert.NoError(t, err)
		return bd.Send(env)
	}
	retryPol := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Second * 3, attemptNum < 2
	}
	cl := NewBroadcastClient(prod, clFact, onConnect, retryPol)
	go os.SendBlock(5)
	resp, err := cl.Recv()
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, uint64(5), resp.GetBlock().Header.Number)
	os.Shutdown()
	cl.Close()
}

func TestDisconnect(t *testing.T) {
	// Scenario: spawn 2 ordering service instances
	// and a client.
	// Have the client try to Recv() from one of them,
	// and disconnect the client until it tries connecting
	// to the other instance.

	defer ensureNoGoroutineLeak(t)()
	os1 := mocks.NewOrderer(5613, t)
	os1.SetNextExpectedSeek(5)
	os2 := mocks.NewOrderer(5614, t)
	os2.SetNextExpectedSeek(5)

	defer os1.Shutdown()
	defer os2.Shutdown()

	waitForConnectionToSomeOSN := func() {
		for {
			if os1.ConnCount() > 0 || os2.ConnCount() > 0 {
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
	}

	connFact := func(endpoint string) (*grpc.ClientConn, error) {
		return grpc.Dial(endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	prod := comm.NewConnectionProducer(connFact, []string{"localhost:5613", "localhost:5614"})
	clFact := func(cc *grpc.ClientConn) orderer.AtomicBroadcastClient {
		return orderer.NewAtomicBroadcastClient(cc)
	}
	onConnect := func(bd blocksprovider.BlocksDeliverer) error {
		return nil
	}
	retryPol := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		return time.Millisecond * 10, attemptNum < 100
	}

	cl := NewBroadcastClient(prod, clFact, onConnect, retryPol)
	stopChan := make(chan struct{})
	go func() {
		cl.Recv()
		stopChan <- struct{}{}
	}()
	waitForConnectionToSomeOSN()
	cl.Disconnect()

	i := 0
	for (os1.ConnCount() == 0 || os2.ConnCount() == 0) && i < 100 {
		t.Log("Attempt", i, "os1ConnCount()=", os1.ConnCount(), "os2ConnCount()=", os2.ConnCount())
		i++
		if i == 100 {
			assert.Fail(t, "Didn't switch to other instance after many attempts")
		}
		cl.Disconnect()
		time.Sleep(time.Millisecond * 500)
	}
	cl.Close()
	select {
	case <-stopChan:
	case <-time.After(time.Second * 20):
		assert.Fail(t, "Didn't stop within a timely manner")
	}
}

func newTestSeekInfo() *orderer.SeekInfo {
	return &orderer.SeekInfo{Start: &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 5}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}
}
