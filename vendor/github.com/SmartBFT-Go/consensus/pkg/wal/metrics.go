package wal

import metrics "github.com/SmartBFT-Go/consensus/pkg/api"

var countOfFilesOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
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
func NewMetrics(p *metrics.CustomerProvider) *Metrics {
	countOfFilesOptsTmp := p.NewGaugeOpts(countOfFilesOpts)
	return &Metrics{
		CountOfFiles: p.NewGauge(countOfFilesOptsTmp).With(p.LabelsForWith()...),
	}
}
