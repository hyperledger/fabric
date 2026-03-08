package wal

import (
	"github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/metrics"
)

var countOfFilesOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "wal_count_of_files",
	Help:         "Count of wal-files.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// Metrics encapsulates wal metrics
type Metrics struct {
	CountOfFiles metrics.Gauge
}

// NewMetrics create new wal metrics
func NewMetrics(p metrics.Provider, labelNames ...string) *Metrics {
	countOfFilesOptsTmp := api.NewGaugeOpts(countOfFilesOpts, labelNames)
	return &Metrics{
		CountOfFiles: p.NewGauge(countOfFilesOptsTmp),
	}
}

func (m *Metrics) With(labelValues ...string) *Metrics {
	return &Metrics{
		CountOfFiles: m.CountOfFiles.With(labelValues...),
	}
}

func (m *Metrics) Initialize() {
	m.CountOfFiles.Add(0)
}
