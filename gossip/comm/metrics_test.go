/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/gossip/identity"
	gmetrics "github.com/hyperledger/fabric/gossip/metrics"
	"github.com/stretchr/testify/assert"
)

type testMetricProvider struct {
	fakeProvider         *metricsfakes.Provider
	fakeSentMessages     *metricsfakes.Counter
	fakeBufferOverflow   *metricsfakes.Counter
	fakeReceivedMessages *metricsfakes.Counter
}

func testUtilConstructMetricProvider() *testMetricProvider {
	fakeProvider := &metricsfakes.Provider{}
	fakeSentMessages := testUtilConstructCounter()
	fakeBufferOverflow := testUtilConstructCounter()
	fakeReceivedMessages := testUtilConstructCounter()

	fakeProvider.NewCounterStub = func(opts metrics.CounterOpts) metrics.Counter {
		switch opts.Name {
		case gmetrics.BufferOverflowOpts.Name:
			return fakeBufferOverflow
		case gmetrics.SentMessagesOpts.Name:
			return fakeSentMessages
		case gmetrics.ReceivedMessagesOpts.Name:
			return fakeReceivedMessages
		}
		return nil
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		return nil
	}
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		return nil
	}

	return &testMetricProvider{
		fakeProvider,
		fakeSentMessages,
		fakeBufferOverflow,
		fakeReceivedMessages,
	}
}

func testUtilConstructCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithReturns(fakeCounter)
	return fakeCounter
}

func newCommInstanceMetrics(port int, sec *naiveSecProvider, commMetrics *gmetrics.CommMetrics) (Comm, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)
	id := []byte(endpoint)
	inst, err := NewCommInstanceWithServer(port, identity.NewIdentityMapper(sec, id, noopPurgeIdentity, sec),
		id, nil, sec, commMetrics)
	return inst, err
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	testMetricProvider := testUtilConstructMetricProvider()

	var overflown uint32
	testMetricProvider.fakeBufferOverflow.AddStub = func(delta float64) {
		atomic.StoreUint32(&overflown, uint32(1))
	}

	fakeCommMetrics := gmetrics.NewGossipMetrics(testMetricProvider.fakeProvider).CommMetrics

	comm1, _ := newCommInstanceMetrics(6870, naiveSec, fakeCommMetrics)
	comm2, _ := newCommInstanceMetrics(6871, naiveSec, fakeCommMetrics)
	defer comm1.Stop()
	defer comm2.Stop()

	// Establish preliminary connection
	fromComm1 := comm2.Accept(acceptAll)
	comm1.Send(createGossipMsg(), remotePeer(6871))
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
		comm1.Send(createGossipMsg(), remotePeer(6871))
		if atomic.LoadUint32(&overflown) == uint32(1) {
			t.Log("Buffer overflow detected")
			break
		}
	}

	assert.EqualValues(t,
		1,
		testMetricProvider.fakeSentMessages.AddArgsForCall(0),
	)

	assert.EqualValues(t,
		1,
		testMetricProvider.fakeReceivedMessages.AddArgsForCall(0),
	)

	assert.EqualValues(t,
		1,
		testMetricProvider.fakeBufferOverflow.AddArgsForCall(0),
	)

	assert.Equal(t, uint32(1), atomic.LoadUint32(&overflown))

}
