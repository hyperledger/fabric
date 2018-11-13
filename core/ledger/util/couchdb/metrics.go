/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"time"

	"github.com/hyperledger/fabric/common/metrics"
)

var (
	apiProcessingTimeOpts = metrics.HistogramOpts{
		Namespace:    "couchdb",
		Subsystem:    "",
		Name:         "processing_time",
		LabelNames:   []string{"database", "result", "function_name"},
		StatsdFormat: "%{#fqname}.%{database}.%{result}.%{function_name}",
	}
)

type stats struct {
	apiProcessingTime metrics.Histogram
}

func newStats(metricsProvider metrics.Provider) *stats {
	return &stats{
		apiProcessingTime: metricsProvider.NewHistogram(apiProcessingTimeOpts),
	}
}

func (s *stats) observeProcessingTime(startTime time.Time, dbName, result, functionName string) {
	s.apiProcessingTime.With(
		"database", dbName,
		"result", result,
		"function_name", functionName,
	).Observe(time.Since(startTime).Seconds())
}
