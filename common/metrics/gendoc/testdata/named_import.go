/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testdata

import (
	goo "github.com/hyperledger/fabric/common/metrics"
)

// These variables should be discovered as valid metric options
// even though a named import was used.

var (
	NamedCounter = goo.CounterOpts{
		Namespace:    "namespace",
		Subsystem:    "counter",
		Name:         "name",
		Help:         "This is some help text",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}

	NamedGauge = goo.GaugeOpts{
		Namespace:    "namespace",
		Subsystem:    "gauge",
		Name:         "name",
		Help:         "This is some help text",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}

	NamedHistogram = goo.HistogramOpts{
		Namespace:    "namespace",
		Subsystem:    "histogram",
		Name:         "name",
		Help:         "This is some help text",
		LabelNames:   []string{"label_one", "label_two"},
		StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
	}
)
