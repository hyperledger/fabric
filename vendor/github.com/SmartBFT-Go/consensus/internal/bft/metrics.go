package bft

import metrics "github.com/SmartBFT-Go/consensus/pkg/api"

const (
	nameBlackListNodeID = "blackid"
	nameReasonFailAdd   = "reason"

	reasonRequestMaxBytes      = "MAX_BYTES"
	reasonSemaphoreAcquireFail = "SEMAPHORE_ACQUIRE_FAIL"
)

var countOfRequestPoolOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_elements",
	Help:         "Number of elements in the consensus request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfFailAddRequestToPoolOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_fail_add_request",
	Help:         "Number of requests pool insertion failure.",
	LabelNames:   []string{nameReasonFailAdd},
	StatsdFormat: "%{#fqname}.%{" + nameReasonFailAdd + "}",
}

// ForwardTimeout
var countOfLeaderForwardRequestOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_leader_forward_request",
	Help:         "Number of requests forwarded to the leader.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTimeoutTwoStepOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_timeout_two_step",
	Help:         "Number of times requests reached second timeout.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfDeleteRequestPoolOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_delete_request",
	Help:         "Number of elements removed from the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfRequestPoolAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_elements_all",
	Help:         "Total amount of elements in the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyOfRequestPoolOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_latency_of_elements",
	Help:         "The average request processing time, time request resides in the pool.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// MetricsRequestPool encapsulates request pool metrics
type MetricsRequestPool struct {
	CountOfRequestPool          metrics.Gauge
	CountOfFailAddRequestToPool metrics.Counter
	CountOfLeaderForwardRequest metrics.Counter
	CountTimeoutTwoStep         metrics.Counter
	CountOfDeleteRequestPool    metrics.Counter
	CountOfRequestPoolAll       metrics.Counter
	LatencyOfRequestPool        metrics.Histogram

	labels []string
}

// NewMetricsRequestPool create new request pool metrics
func NewMetricsRequestPool(p *metrics.CustomerProvider) *MetricsRequestPool {
	countOfRequestPoolOptsTmp := p.NewGaugeOpts(countOfRequestPoolOpts)
	countOfFailAddRequestToPoolOptsTmp := p.NewCounterOpts(countOfFailAddRequestToPoolOpts)
	countOfLeaderForwardRequestOptsTmp := p.NewCounterOpts(countOfLeaderForwardRequestOpts)
	countTimeoutTwoStepOptsTmp := p.NewCounterOpts(countTimeoutTwoStepOpts)
	countOfDeleteRequestPoolOptsTmp := p.NewCounterOpts(countOfDeleteRequestPoolOpts)
	countOfRequestPoolAllOptsTmp := p.NewCounterOpts(countOfRequestPoolAllOpts)
	latencyOfRequestPoolOptsTmp := p.NewHistogramOpts(latencyOfRequestPoolOpts)
	return &MetricsRequestPool{
		CountOfRequestPool:          p.NewGauge(countOfRequestPoolOptsTmp).With(p.LabelsForWith()...),
		CountOfFailAddRequestToPool: p.NewCounter(countOfFailAddRequestToPoolOptsTmp),
		CountOfLeaderForwardRequest: p.NewCounter(countOfLeaderForwardRequestOptsTmp).With(p.LabelsForWith()...),
		CountTimeoutTwoStep:         p.NewCounter(countTimeoutTwoStepOptsTmp).With(p.LabelsForWith()...),
		CountOfDeleteRequestPool:    p.NewCounter(countOfDeleteRequestPoolOptsTmp).With(p.LabelsForWith()...),
		CountOfRequestPoolAll:       p.NewCounter(countOfRequestPoolAllOptsTmp).With(p.LabelsForWith()...),
		LatencyOfRequestPool:        p.NewHistogram(latencyOfRequestPoolOptsTmp).With(p.LabelsForWith()...),
		labels:                      p.LabelsForWith(),
	}
}

func (m *MetricsRequestPool) LabelsForWith(labelValues ...string) []string {
	result := make([]string, 0, len(m.labels)+len(labelValues))
	result = append(result, labelValues...)
	result = append(result, m.labels...)
	return result
}

var countBlackListOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "blacklist_count",
	Help:         "Count of nodes in blacklist on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nodesInBlackListOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "node_id_in_blacklist",
	Help:         "Node ID in blacklist on this channel.",
	LabelNames:   []string{nameBlackListNodeID},
	StatsdFormat: "%{#fqname}.%{" + nameBlackListNodeID + "}",
}

// MetricsBlacklist encapsulates blacklist metrics
type MetricsBlacklist struct {
	CountBlackList   metrics.Gauge
	NodesInBlackList metrics.Gauge

	labels []string
}

// NewMetricsBlacklist create new blacklist metrics
func NewMetricsBlacklist(p *metrics.CustomerProvider) *MetricsBlacklist {
	countBlackListOptsTmp := p.NewGaugeOpts(countBlackListOpts)
	nodesInBlackListOptsTmp := p.NewGaugeOpts(nodesInBlackListOpts)
	return &MetricsBlacklist{
		CountBlackList:   p.NewGauge(countBlackListOptsTmp).With(p.LabelsForWith()...),
		NodesInBlackList: p.NewGauge(nodesInBlackListOptsTmp),
		labels:           p.LabelsForWith(),
	}
}

func (m *MetricsBlacklist) LabelsForWith(labelValues ...string) []string {
	result := make([]string, 0, len(m.labels)+len(labelValues))
	result = append(result, labelValues...)
	result = append(result, m.labels...)
	return result
}

var consensusReconfigOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "consensus_reconfig",
	Help:         "Number of reconfiguration requests.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencySyncOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "consensus_latency_sync",
	Help:         "An average time it takes to sync node.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// MetricsConsensus encapsulates consensus metrics
type MetricsConsensus struct {
	CountConsensusReconfig metrics.Counter
	LatencySync            metrics.Histogram
}

// NewMetricsConsensus create new consensus metrics
func NewMetricsConsensus(p *metrics.CustomerProvider) *MetricsConsensus {
	consensusReconfigOptsTmp := p.NewCounterOpts(consensusReconfigOpts)
	latencySyncOptsTmp := p.NewHistogramOpts(latencySyncOpts)
	return &MetricsConsensus{
		CountConsensusReconfig: p.NewCounter(consensusReconfigOptsTmp).With(p.LabelsForWith()...),
		LatencySync:            p.NewHistogram(latencySyncOptsTmp).With(p.LabelsForWith()...),
	}
}

var viewNumberOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_number",
	Help:         "The View number value.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var leaderIDOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_leader_id",
	Help:         "The leader id.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var proposalSequenceOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_proposal_sequence",
	Help:         "The sequence number within current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var decisionsInViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_decisions",
	Help:         "The number of decisions in the current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var phaseOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_phase",
	Help:         "Current consensus phase.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsInBatchOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_txs_in_batch",
	Help:         "The number of transactions per batch.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countBatchAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_batch_all",
	Help:         "Amount of batched processed.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_txs_all",
	Help:         "Total amount of transactions.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var sizeOfBatchOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_size_batch",
	Help:         "An average batch size.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchProcessingOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_latency_batch_processing",
	Help:         "Amount of time it take to process batch.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchSaveOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_latency_batch_save",
	Help:         "An average time it takes to persist batch.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// MetricsView encapsulates view metrics
type MetricsView struct {
	ViewNumber             metrics.Gauge
	LeaderID               metrics.Gauge
	ProposalSequence       metrics.Gauge
	DecisionsInView        metrics.Gauge
	Phase                  metrics.Gauge
	CountTxsInBatch        metrics.Gauge
	CountBatchAll          metrics.Counter
	CountTxsAll            metrics.Counter
	SizeOfBatch            metrics.Counter
	LatencyBatchProcessing metrics.Histogram
	LatencyBatchSave       metrics.Histogram
}

// NewMetricsView create new view metrics
func NewMetricsView(p *metrics.CustomerProvider) *MetricsView {
	viewNumberOptsTmp := p.NewGaugeOpts(viewNumberOpts)
	leaderIDOptsTmp := p.NewGaugeOpts(leaderIDOpts)
	proposalSequenceOptsTmp := p.NewGaugeOpts(proposalSequenceOpts)
	decisionsInViewOptsTmp := p.NewGaugeOpts(decisionsInViewOpts)
	phaseOptsTmp := p.NewGaugeOpts(phaseOpts)
	countTxsInBatchOptsTmp := p.NewGaugeOpts(countTxsInBatchOpts)
	countBatchAllOptsTmp := p.NewCounterOpts(countBatchAllOpts)
	countTxsAllOptsTmp := p.NewCounterOpts(countTxsAllOpts)
	sizeOfBatchOptsTmp := p.NewCounterOpts(sizeOfBatchOpts)
	latencyBatchProcessingOptsTmp := p.NewHistogramOpts(latencyBatchProcessingOpts)
	latencyBatchSaveOptsTmp := p.NewHistogramOpts(latencyBatchSaveOpts)
	return &MetricsView{
		ViewNumber:             p.NewGauge(viewNumberOptsTmp).With(p.LabelsForWith()...),
		LeaderID:               p.NewGauge(leaderIDOptsTmp).With(p.LabelsForWith()...),
		ProposalSequence:       p.NewGauge(proposalSequenceOptsTmp).With(p.LabelsForWith()...),
		DecisionsInView:        p.NewGauge(decisionsInViewOptsTmp).With(p.LabelsForWith()...),
		Phase:                  p.NewGauge(phaseOptsTmp).With(p.LabelsForWith()...),
		CountTxsInBatch:        p.NewGauge(countTxsInBatchOptsTmp).With(p.LabelsForWith()...),
		CountBatchAll:          p.NewCounter(countBatchAllOptsTmp).With(p.LabelsForWith()...),
		CountTxsAll:            p.NewCounter(countTxsAllOptsTmp).With(p.LabelsForWith()...),
		SizeOfBatch:            p.NewCounter(sizeOfBatchOptsTmp).With(p.LabelsForWith()...),
		LatencyBatchProcessing: p.NewHistogram(latencyBatchProcessingOptsTmp).With(p.LabelsForWith()...),
		LatencyBatchSave:       p.NewHistogram(latencyBatchSaveOptsTmp).With(p.LabelsForWith()...),
	}
}

var currentViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_current_view",
	Help:         "current view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nextViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_next_view",
	Help:         "next view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var realViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_real_view",
	Help:         "real view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// MetricsViewChange encapsulates view change metrics
type MetricsViewChange struct {
	CurrentView metrics.Gauge
	NextView    metrics.Gauge
	RealView    metrics.Gauge
}

// NewMetricsViewChange create new view change metrics
func NewMetricsViewChange(p *metrics.CustomerProvider) *MetricsViewChange {
	currentViewOptsTmp := p.NewGaugeOpts(currentViewOpts)
	nextViewOptsTmp := p.NewGaugeOpts(nextViewOpts)
	realViewOptsTmp := p.NewGaugeOpts(realViewOpts)
	return &MetricsViewChange{
		CurrentView: p.NewGauge(currentViewOptsTmp).With(p.LabelsForWith()...),
		NextView:    p.NewGauge(nextViewOptsTmp).With(p.LabelsForWith()...),
		RealView:    p.NewGauge(realViewOptsTmp).With(p.LabelsForWith()...),
	}
}
