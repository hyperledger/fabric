/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"testing"

	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {

	provider := &metricsfakes.Provider{}

	counter := &metricsfakes.Counter{}
	gauge := &metricsfakes.Gauge{}
	histogram := &metricsfakes.Histogram{}

	provider.NewCounterReturns(counter)
	provider.NewGaugeReturns(gauge)
	provider.NewHistogramReturns(histogram)

	gossipMetrics := NewGossipMetrics(provider)

	// make sure all metrics were created
	assert.NotNil(t, gossipMetrics)

	assert.NotNil(t, gossipMetrics.StateMetrics)
	assert.NotNil(t, gossipMetrics.StateMetrics.Height)
	assert.NotNil(t, gossipMetrics.StateMetrics.CommitDuration)
	assert.NotNil(t, gossipMetrics.StateMetrics.PayloadBufferSize)

	assert.NotNil(t, gossipMetrics.ElectionMetrics)
	assert.NotNil(t, gossipMetrics.ElectionMetrics.Declaration)

}
