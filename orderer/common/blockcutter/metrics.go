/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockcutter

import "github.com/hyperledger/fabric/common/metrics"

var blockFillDuration = metrics.HistogramOpts{
	Namespace:    "blockcutter",
	Name:         "block_fill_duration",
	Help:         "The time from first transaction enqueing to the block being cut in seconds.",
	LabelNames:   []string{"channel"},
	StatsdFormat: "%{#fqname}.%{channel}",
}

type Metrics struct {
	BlockFillDuration metrics.Histogram
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		BlockFillDuration: p.NewHistogram(blockFillDuration),
	}
}
