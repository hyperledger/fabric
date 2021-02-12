/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"testing"

	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, gossipMetrics)

	require.NotNil(t, gossipMetrics.StateMetrics)
	require.NotNil(t, gossipMetrics.StateMetrics.Height)
	require.NotNil(t, gossipMetrics.StateMetrics.CommitDuration)
	require.NotNil(t, gossipMetrics.StateMetrics.PayloadBufferSize)

	require.NotNil(t, gossipMetrics.ElectionMetrics)
	require.NotNil(t, gossipMetrics.ElectionMetrics.Declaration)

	require.NotNil(t, gossipMetrics.CommMetrics)
	require.NotNil(t, gossipMetrics.CommMetrics.SentMessages)
	require.NotNil(t, gossipMetrics.CommMetrics.ReceivedMessages)
	require.NotNil(t, gossipMetrics.CommMetrics.BufferOverflow)

	require.NotNil(t, gossipMetrics.MembershipMetrics)
	require.NotNil(t, gossipMetrics.MembershipMetrics.Total)

	require.NotNil(t, gossipMetrics.PrivdataMetrics)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.CommitPrivateDataDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.FetchDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.ListMissingPrivateDataDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.PurgeDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.ValidationDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.SendDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.ReconciliationDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.PullDuration)
	require.NotNil(t, gossipMetrics.PrivdataMetrics.RetrieveDuration)
}
