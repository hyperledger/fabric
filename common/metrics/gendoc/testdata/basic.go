/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testdata

import (
	"time"

	"github.com/hyperledger/fabric/common/metrics"
)

// These variables should be discovered as valid metric options.

var (
	Counter = metrics.CounterOpts{
		Namespace:    "fixtures",
		Name:         "counter",
		Help:         "This is some help text that is more than a few words long. It really can be quite long. Really long.",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}

	Gauge = metrics.GaugeOpts{
		Namespace:    "fixtures",
		Name:         "gauge",
		Help:         "This is some help text",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}

	Histogram = metrics.HistogramOpts{
		Namespace:    "fixtures",
		Name:         "histogram",
		Help:         "This is some help text",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}

	ignoredStruct = struct{}{}

	ignoredInt = 0

	ignoredTime = time.Now()
)
