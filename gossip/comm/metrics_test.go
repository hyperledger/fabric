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

	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/stretchr/testify/assert"
)

func newCommInstanceMetrics(port int, sec *naiveSecProvider, commMetrics *metrics.CommMetrics) (Comm, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)
	id := []byte(endpoint)
	inst, err := NewCommInstanceWithServer(port, identity.NewIdentityMapper(sec, id, noopPurgeIdentity, sec),
		id, nil, sec, commMetrics)
	return inst, err
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	testMetricProvider := mocks.TestUtilConstructMetricProvider()

	var overflown uint32
	testMetricProvider.FakeBufferOverflow.AddStub = func(delta float64) {
		atomic.StoreUint32(&overflown, uint32(1))
	}

	fakeCommMetrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).CommMetrics

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
		testMetricProvider.FakeSentMessages.AddArgsForCall(0),
	)

	assert.EqualValues(t,
		1,
		testMetricProvider.FakeReceivedMessages.AddArgsForCall(0),
	)

	assert.EqualValues(t,
		1,
		testMetricProvider.FakeBufferOverflow.AddArgsForCall(0),
	)

	assert.Equal(t, uint32(1), atomic.LoadUint32(&overflown))

}
