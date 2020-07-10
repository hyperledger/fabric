/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
)

func newCommInstanceWithMetrics(t *testing.T, sec *naiveSecProvider, metrics *metrics.CommMetrics) (c Comm, port int) {
	port, gRPCServer, certs, secureDialOpts, dialOpts := util.CreateGRPCLayer()
	comm := newCommInstanceOnlyWithMetrics(t, metrics, sec, gRPCServer, certs, secureDialOpts, dialOpts...)
	return comm, port
}

func TestMetrics(t *testing.T) {
	testMetricProvider := mocks.TestUtilConstructMetricProvider()

	var overflown uint32
	testMetricProvider.FakeBufferOverflow.AddStub = func(delta float64) {
		atomic.StoreUint32(&overflown, uint32(1))
	}

	fakeCommMetrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).CommMetrics

	comm1, _ := newCommInstanceWithMetrics(t, naiveSec, fakeCommMetrics)
	comm2, port2 := newCommInstanceWithMetrics(t, naiveSec, fakeCommMetrics)
	defer comm1.Stop()
	defer comm2.Stop()

	// Establish preliminary connection
	fromComm1 := comm2.Accept(acceptAll)
	comm1.Send(createGossipMsg(), remotePeer(port2))
	<-fromComm1

	// Drain messages slowly by comm2
	drainMessages := func() {
		for {
			msg := <-fromComm1
			time.Sleep(time.Millisecond)
			if msg == nil {
				return
			}
		}
	}

	go drainMessages()

	// Send messages until the buffer overflow event emission is detected
	for {
		comm1.Send(createGossipMsg(), remotePeer(port2))
		if atomic.LoadUint32(&overflown) == uint32(1) {
			t.Log("Buffer overflow detected")
			break
		}
	}

	require.EqualValues(t,
		1,
		testMetricProvider.FakeSentMessages.AddArgsForCall(0),
	)

	require.EqualValues(t,
		1,
		testMetricProvider.FakeReceivedMessages.AddArgsForCall(0),
	)

	require.EqualValues(t,
		1,
		testMetricProvider.FakeBufferOverflow.AddArgsForCall(0),
	)

	require.Equal(t, uint32(1), atomic.LoadUint32(&overflown))
}
