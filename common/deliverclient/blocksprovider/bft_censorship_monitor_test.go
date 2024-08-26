/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNewBFTCensorshipMonitor_New(t *testing.T) {
	flogging.ActivateSpec("debug")

	s := newMonitorTestSetup(t, 5)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, 0, blocksprovider.TimeoutConfig{})
	require.NotNil(t, mon)
}

// Scenario:
// - start the monitor, with a single source
// - stop the monitor without reading from error channel
func TestBFTCensorshipMonitor_Stop(t *testing.T) {
	flogging.ActivateSpec("debug")

	s := newMonitorTestSetup(t, 1)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, 0, blocksprovider.TimeoutConfig{})
	require.NotNil(t, mon)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	mon.Stop()
	wg.Wait()
}

// Scenario:
// - start the monitor, with a single source
// - stop the monitor, ensure error channel contains an error
func TestBFTCensorshipMonitor_StopWithErrors(t *testing.T) {
	flogging.ActivateSpec("debug")

	s := newMonitorTestSetup(t, 1)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, 0, blocksprovider.TimeoutConfig{})
	require.NotNil(t, mon)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "received a stop signal")
}

// Scenario:
// - start the monitor, with no sources
// - monitor exits, ensure error channel contains an error
func TestBFTCensorshipMonitor_NoSources(t *testing.T) {
	flogging.ActivateSpec("debug")

	s := newMonitorTestSetup(t, 0)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, 0, blocksprovider.TimeoutConfig{})
	require.NotNil(t, mon)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "no endpoints")
}

// Scenario:
// - start the monitor, with 4 sources
// - the monitor connects to all but the block source
// - no blocks, no headers, block progress is called repeatedly, returns zero value
// - headers are seeked from 0
func TestBFTCensorshipMonitor_NoHeadersNoBlocks(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 4
	blockSource := 1
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       1 * time.Millisecond,
		MaxRetryInterval:       20 * time.Millisecond,
		BlockCensorshipTimeout: time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			client.RecvCalls(func() (*orderer.DeliverResponse, error) {
				resp := <-s.sourceStream[index]
				if resp == nil {
					return nil, errors.New("test-closing")
				}
				return resp, nil
			})

			client.CloseSendCalls(func() error {
				close(s.sourceStream[index])
				return nil
			})

			return client, func() {}, nil
		})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() >= 9 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(0), n)
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep)
		require.Equal(t, fakeEnv, env)
	}

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "received a stop signal")
}

// Scenario:
// - start the monitor, with 4 sources
// - the monitor connects to all but the block source
// - block progress returns {7, now}
// - one header returns {8}
// - suspicion raised, after timeout, censorship detected
func TestBFTCensorshipMonitor_CensorshipDetected(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 4
	blockSource := 0
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       1 * time.Millisecond,
		MaxRetryInterval:       20 * time.Millisecond,
		BlockCensorshipTimeout: time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			client.RecvCalls(func() (*orderer.DeliverResponse, error) {
				resp := <-s.sourceStream[index]
				if resp == nil {
					return nil, errors.New("test-closing")
				}
				return resp, nil
			})

			client.CloseSendCalls(func() error {
				close(s.sourceStream[index])
				return nil
			})

			return client, func() {}, nil
		})
	b7time := time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(7), b7time)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() > 9 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(8), n, "should seek from block 8")
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep, "should not connect to block source")
		require.Equal(t, fakeEnv, env, "should connect with expected envelope")
	}

	// one header is ahead
	s.sourceStream[1] <- makeDeliverResponseBlock(8)

	require.Eventually(t,
		func() bool {
			susp, num := mon.GetSuspicion()
			return susp && num == uint64(8)
		},
		5*time.Second, 1*time.Millisecond, "suspicion should be raised on block number 8")

	wg.Wait()
	require.EqualError(t, err, "block censorship detected, endpoint: Address: orderer-address-0, CertHash: 08D6C05A21512A79A1DFEB9D2A8F262F")
}

// Scenario:
// - start the monitor, with 4 sources
// - the monitor connects to all but the block source
// - block progress returns {7, now}
// - header receivers return {8, 9, 10}
// - suspicion raised, block advances to 8, then 9,
// - after timeout, censorship detected on 10
func TestBFTCensorshipMonitor_SuspicionsRemovedCensorshipDetected(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 4
	blockSource := 0
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       1 * time.Millisecond,
		MaxRetryInterval:       20 * time.Millisecond,
		BlockCensorshipTimeout: time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			client.RecvCalls(func() (*orderer.DeliverResponse, error) {
				resp := <-s.sourceStream[index]
				if resp == nil {
					return nil, errors.New("test-closing")
				}
				return resp, nil
			})

			client.CloseSendCalls(func() error {
				close(s.sourceStream[index])
				return nil
			})

			return client, func() {}, nil
		})
	blockTime := time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(7), blockTime)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() > 9 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(8), n, "should seek from block 8")
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep, "should not connect to block source")
		require.Equal(t, fakeEnv, env, "should connect with expected envelope")
	}

	// 3 headers are ahead
	s.sourceStream[1] <- makeDeliverResponseBlock(8)
	time.Sleep(5 * time.Millisecond)
	s.sourceStream[2] <- makeDeliverResponseBlock(9)
	time.Sleep(5 * time.Millisecond)
	s.sourceStream[3] <- makeDeliverResponseBlock(10)

	require.Eventually(t,
		func() bool {
			susp, num := mon.GetSuspicion()
			return susp && num == uint64(8)
		},
		5*time.Second, 1*time.Millisecond, "suspicion should be raised on block number 8")

	blockTime = time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(8), blockTime)

	require.Eventually(t,
		func() bool {
			susp, num := mon.GetSuspicion()
			return susp && num == uint64(9)
		},
		5*time.Second, 1*time.Millisecond, "suspicion should be raised on block number 9")

	blockTime = time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(9), blockTime)

	require.Eventually(t,
		func() bool {
			susp, num := mon.GetSuspicion()
			return susp && num == uint64(10)
		},
		5*time.Second, 1*time.Millisecond, "suspicion should be raised on block number 10")

	wg.Wait()
	require.EqualError(t, err, "block censorship detected, endpoint: Address: orderer-address-0, CertHash: 08D6C05A21512A79A1DFEB9D2A8F262F")
}

// Scenario:
// - start the monitor, with 4 sources
// - the monitor connects to all but the block source
// - block progress returns {n, now}
// - one header returns {n+1}, suspicion raised,
// - before timeout, new block (n+1) arrives, suspicion removed
//   - repeat the above x7, each time the header is coming from a different source
func TestBFTCensorshipMonitor_SuspicionRemoved(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 4
	blockSource := 0
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       1 * time.Millisecond,
		MaxRetryInterval:       20 * time.Millisecond,
		BlockCensorshipTimeout: 5 * time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			client.RecvCalls(func() (*orderer.DeliverResponse, error) {
				resp := <-s.sourceStream[index]
				if resp == nil {
					return nil, errors.New("test-closing")
				}
				return resp, nil
			})

			client.CloseSendCalls(func() error {
				close(s.sourceStream[index])
				return nil
			})

			return client, func() {}, nil
		})
	blockTime := time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(7), blockTime)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() == 3 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() > 9 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(8), n, "should seek from block 8")
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep, "should not connect to block source")
		require.Equal(t, fakeEnv, env, "should connect with expected envelope")
	}

	for n := uint64(8); n < uint64(15); n++ {
		// one header is ahead, coming from a different source every time
		headerSourceIndex := n%3 + 1
		s.sourceStream[headerSourceIndex] <- makeDeliverResponseBlock(n)

		require.Eventually(t,
			func() bool {
				susp, num := mon.GetSuspicion()
				return susp && num == n
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be raised")

		blockTime = time.Now()
		s.fakeProgressReporter.BlockProgressReturns(n, blockTime)

		require.Eventually(t,
			func() bool {
				susp, _ := mon.GetSuspicion()
				return !susp
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be removed")
	}

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "received a stop signal")
}

// Scenario:
// - start the monitor, with 7 sources
// - the monitor tries to connect to all but the block source (index=0),
//   - one orderer is faulty (index=1), Recv returns errors
//   - one orderer is down (index=2), cannot connect
//
// - block progress returns {n, now}
// - one header returns {n+1}, suspicion raised,
// - before timeout, new block (n+1) arrives, suspicion removed
//   - repeat the above x7, each time the header is coming from a different source
func TestBFTCensorshipMonitor_FaultySourceIgnored(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 7
	blockSource := 0
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       5 * time.Millisecond,
		MaxRetryInterval:       20 * time.Millisecond,
		BlockCensorshipTimeout: 5 * time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			switch index {
			case 1:
				client.RecvReturns(nil, errors.New("test: faulty source"))
				client.CloseSendCalls(func() error {
					return nil
				})
			case 2:
				return nil, nil, errors.New("test: cannot connect")
			default:
				client.RecvCalls(func() (*orderer.DeliverResponse, error) {
					resp := <-s.sourceStream[index]
					if resp == nil {
						return nil, errors.New("test-closing")
					}
					return resp, nil
				})
				client.CloseSendCalls(func() error {
					close(s.sourceStream[index])
					return nil
				})
			}

			return client, func() {}, nil
		})
	blockTime := time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(7), blockTime)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() >= 6 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() >= 6 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() >= 12 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(8), n, "should seek from block 8")
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep, "should not connect to block source")
		require.Equal(t, fakeEnv, env, "should connect with expected envelope")
	}

	for n := uint64(8); n < uint64(15); n++ {
		// one header is ahead, coming from a different honest source every time
		headerSourceIndex := n%4 + 3
		s.sourceStream[headerSourceIndex] <- makeDeliverResponseBlock(n)

		require.Eventually(t,
			func() bool {
				susp, num := mon.GetSuspicion()
				return susp && num == n
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be raised")

		blockTime = time.Now()
		s.fakeProgressReporter.BlockProgressReturns(n, blockTime)

		require.Eventually(t,
			func() bool {
				susp, _ := mon.GetSuspicion()
				return !susp
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be removed")
	}

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "received a stop signal")
}

// Scenario:
// - start the monitor, with 4 sources
// - the monitor tries to connect to all but the block source (index=0),
//   - for the first 10 calls:
//   - one orderer is faulty (index=1), Recv returns errors
//   - one orderer is down (index=2), cannot connect
//   - then these sources recover
//
// - one orderer remains down (index=3)
//
// - block progress returns {n, now}
// - the recovered sources returns {n+1}, suspicion raised,
// - before timeout, new block (n+1) arrives, suspicion removed
//   - repeat the above x7, each time the header is coming from a different recovered source
func TestBFTCensorshipMonitor_FaultySourceRecovery(t *testing.T) {
	flogging.ActivateSpec("debug")

	numOrderers := 4
	blockSource := 0
	tConfig := blocksprovider.TimeoutConfig{
		MinRetryInterval:       5 * time.Millisecond,
		MaxRetryInterval:       80 * time.Millisecond,
		BlockCensorshipTimeout: 1 * time.Second,
	}
	s := newMonitorTestSetup(t, numOrderers)
	mon := blocksprovider.NewBFTCensorshipMonitor(s.channelID, s.fakeUpdatableBlockVerifier, s.fakeRequester, s.fakeProgressReporter, s.sources, blockSource, tConfig)
	require.NotNil(t, mon)

	fakeEnv := &common.Envelope{Payload: []byte("bogus"), Signature: []byte("bogus")}
	s.fakeRequester.SeekInfoHeadersFromReturns(fakeEnv, nil)
	// Connect returns a client that blocks on Recv()
	callNum1 := 0
	callNum2 := 0
	s.fakeRequester.ConnectCalls(
		func(envelope *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error) {
			index := address2index(endpoint.Address)
			client := &fake.DeliverClient{}

			switch index {
			case 1:
				callNum1++
				if callNum1 <= 10 {
					client.RecvReturns(nil, errors.New("test: faulty source"))
					client.CloseSendCalls(func() error {
						return nil
					})
					return client, func() {}, nil
				}
			case 2:
				callNum2++
				if callNum2 <= 10 {
					return nil, nil, errors.New("test: cannot connect")
				}
			case 3:
				return nil, nil, errors.New("test: cannot connect")
			}

			client.RecvCalls(func() (*orderer.DeliverResponse, error) {
				resp := <-s.sourceStream[index]
				if resp == nil {
					return nil, errors.New("test-closing")
				}
				return resp, nil
			})
			client.CloseSendCalls(func() error {
				close(s.sourceStream[index])
				return nil
			})

			return client, func() {}, nil
		})
	blockTime := time.Now()
	s.fakeProgressReporter.BlockProgressReturns(uint64(7), blockTime)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		mon.Monitor()
		wg.Done()
	}()

	var err error
	go func() {
		err = <-mon.ErrorsChannel()
		wg.Done()
	}()

	require.Eventually(t, func() bool { return s.fakeRequester.SeekInfoHeadersFromCallCount() >= 30 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeRequester.ConnectCallCount() >= 30 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s.fakeProgressReporter.BlockProgressCallCount() >= 60 }, 5*time.Second, 10*time.Millisecond)
	for i := 0; i < s.fakeRequester.ConnectCallCount(); i++ {
		n := s.fakeRequester.SeekInfoHeadersFromArgsForCall(i)
		require.Equal(t, uint64(8), n, "should seek from block 8")
		env, ep := s.fakeRequester.ConnectArgsForCall(i)
		require.NotEqual(t, s.sources[s.sourceIndex].Address, ep, "should not connect to block source")
		require.Equal(t, fakeEnv, env, "should connect with expected envelope")
	}

	for n := uint64(8); n < uint64(15); n++ {
		// one header is ahead, coming from a different recovered source (1,2) every time
		headerSourceIndex := n%2 + 1
		s.sourceStream[headerSourceIndex] <- makeDeliverResponseBlock(n)

		require.Eventually(t,
			func() bool {
				susp, num := mon.GetSuspicion()
				return susp && num == n
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be raised")

		blockTime = time.Now()
		s.fakeProgressReporter.BlockProgressReturns(n, blockTime)

		require.Eventually(t,
			func() bool {
				susp, _ := mon.GetSuspicion()
				return !susp
			},
			5*time.Second, 10*time.Millisecond, "suspicion should be removed")
	}

	mon.Stop()
	wg.Wait()
	require.EqualError(t, err, "received a stop signal")
}

type monitorTestSetup struct {
	channelID                  string
	fakeUpdatableBlockVerifier *fake.UpdatableBlockVerifier
	fakeProgressReporter       *fake.BlockProgressReporter
	fakeRequester              *fake.DeliverClientRequester
	sources                    []*orderers.Endpoint
	sourceIndex                int
	sourceStream               []chan *orderer.DeliverResponse
}

func newMonitorTestSetup(t *testing.T, numSources int) *monitorTestSetup {
	s := &monitorTestSetup{
		channelID:                  "testchannel",
		fakeUpdatableBlockVerifier: &fake.UpdatableBlockVerifier{},
		fakeProgressReporter:       &fake.BlockProgressReporter{},
		fakeRequester:              &fake.DeliverClientRequester{},
		sources:                    nil,
		sourceIndex:                0,
	}
	s.fakeUpdatableBlockVerifier.CloneReturns(&fake.UpdatableBlockVerifier{})

	for i := 0; i < numSources; i++ {
		s.sources = append(s.sources, &orderers.Endpoint{
			Address:   fmt.Sprintf("orderer-address-%d", i),
			RootCerts: [][]byte{{1, 2, 3, 4}},
			Refreshed: make(chan struct{}),
		})
		s.sourceStream = append(s.sourceStream, make(chan *orderer.DeliverResponse))
	}

	return s
}

func address2index(addr string) int {
	tokens := strings.Split(addr, "-")
	i, _ := strconv.Atoi(tokens[2])
	return i
}

func makeDeliverResponseBlock(n uint64) *orderer.DeliverResponse {
	return &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{
			Block: &common.Block{
				Header: &common.BlockHeader{
					Number: n,
				},
			},
		},
	}
}
