package api

import "strconv"

const (
	NameBlackListNodeID = "blackid"
	NameReasonFailAdd   = "reason"

	ReasonRequestMaxBytes      = "MAX_BYTES"
	ReasonSemaphoreAcquireFail = "SEMAPHORE_ACQUIRE_FAIL"
)

type Metrics struct {
	MetricsRequestPool *MetricsRequestPool
	MetricsBlacklist   *MetricsBlacklist
	MetricsConsensus   *MetricsConsensus
	MetricsView        *MetricsView
	MetricsViewChange  *MetricsViewChange
}

func NewMetrics(p Provider, labelNames ...string) *Metrics {
	return &Metrics{
		MetricsRequestPool: NewMetricsRequestPool(p, labelNames...),
		MetricsBlacklist:   NewMetricsBlacklist(p, labelNames...),
		MetricsConsensus:   NewMetricsConsensus(p, labelNames...),
		MetricsView:        NewMetricsView(p, labelNames...),
		MetricsViewChange:  NewMetricsViewChange(p, labelNames...),
	}
}

func (m *Metrics) With(labelValues ...string) *Metrics {
	return &Metrics{
		MetricsRequestPool: m.MetricsRequestPool.With(labelValues...),
		MetricsBlacklist:   m.MetricsBlacklist.With(labelValues...),
		MetricsConsensus:   m.MetricsConsensus.With(labelValues...),
		MetricsView:        m.MetricsView.With(labelValues...),
		MetricsViewChange:  m.MetricsViewChange.With(labelValues...),
	}
}

func (m *Metrics) Initialize(nodes []uint64) {
	m.MetricsRequestPool.Initialize()
	m.MetricsBlacklist.Initialize(nodes)
	m.MetricsConsensus.Initialize()
	m.MetricsView.Initialize()
	m.MetricsViewChange.Initialize()
}

var countOfRequestPoolOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_elements",
	Help:         "Number of elements in the consensus request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfFailAddRequestToPoolOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_fail_add_request",
	Help:         "Number of requests pool insertion failure.",
	LabelNames:   []string{NameReasonFailAdd},
	StatsdFormat: "%{#fqname}.%{" + NameReasonFailAdd + "}",
}

// ForwardTimeout
var countOfLeaderForwardRequestOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_leader_forward_request",
	Help:         "Number of requests forwarded to the leader.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTimeoutTwoStepOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_timeout_two_step",
	Help:         "Number of times requests reached second timeout.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfDeleteRequestPoolOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_delete_request",
	Help:         "Number of elements removed from the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfRequestPoolAllOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "pool_count_of_elements_all",
	Help:         "Total amount of elements in the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyOfRequestPoolOpts = HistogramOpts{
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
	CountOfRequestPool          Gauge
	CountOfFailAddRequestToPool Counter
	CountOfLeaderForwardRequest Counter
	CountTimeoutTwoStep         Counter
	CountOfDeleteRequestPool    Counter
	CountOfRequestPoolAll       Counter
	LatencyOfRequestPool        Histogram

	labels []string
}

// NewMetricsRequestPool create new request pool metrics
func NewMetricsRequestPool(p Provider, labelNames ...string) *MetricsRequestPool {
	countOfRequestPoolOptsTmp := NewGaugeOpts(countOfRequestPoolOpts, labelNames)
	countOfFailAddRequestToPoolOptsTmp := NewCounterOpts(countOfFailAddRequestToPoolOpts, labelNames)
	countOfLeaderForwardRequestOptsTmp := NewCounterOpts(countOfLeaderForwardRequestOpts, labelNames)
	countTimeoutTwoStepOptsTmp := NewCounterOpts(countTimeoutTwoStepOpts, labelNames)
	countOfDeleteRequestPoolOptsTmp := NewCounterOpts(countOfDeleteRequestPoolOpts, labelNames)
	countOfRequestPoolAllOptsTmp := NewCounterOpts(countOfRequestPoolAllOpts, labelNames)
	latencyOfRequestPoolOptsTmp := NewHistogramOpts(latencyOfRequestPoolOpts, labelNames)
	return &MetricsRequestPool{
		CountOfRequestPool:          p.NewGauge(countOfRequestPoolOptsTmp),
		CountOfFailAddRequestToPool: p.NewCounter(countOfFailAddRequestToPoolOptsTmp),
		CountOfLeaderForwardRequest: p.NewCounter(countOfLeaderForwardRequestOptsTmp),
		CountTimeoutTwoStep:         p.NewCounter(countTimeoutTwoStepOptsTmp),
		CountOfDeleteRequestPool:    p.NewCounter(countOfDeleteRequestPoolOptsTmp),
		CountOfRequestPoolAll:       p.NewCounter(countOfRequestPoolAllOptsTmp),
		LatencyOfRequestPool:        p.NewHistogram(latencyOfRequestPoolOptsTmp),
	}
}

func (m *MetricsRequestPool) With(labelValues ...string) *MetricsRequestPool {
	return &MetricsRequestPool{
		CountOfRequestPool:          m.CountOfRequestPool.With(labelValues...),
		CountOfFailAddRequestToPool: m.CountOfFailAddRequestToPool,
		CountOfLeaderForwardRequest: m.CountOfLeaderForwardRequest.With(labelValues...),
		CountTimeoutTwoStep:         m.CountTimeoutTwoStep.With(labelValues...),
		CountOfDeleteRequestPool:    m.CountOfDeleteRequestPool.With(labelValues...),
		CountOfRequestPoolAll:       m.CountOfRequestPoolAll.With(labelValues...),
		LatencyOfRequestPool:        m.LatencyOfRequestPool.With(labelValues...),
		labels:                      labelValues,
	}
}

func (m *MetricsRequestPool) Initialize() {
	m.CountOfRequestPool.Add(0)
	m.CountOfFailAddRequestToPool.With(
		m.LabelsForWith(NameReasonFailAdd, ReasonRequestMaxBytes)...,
	).Add(0)
	m.CountOfFailAddRequestToPool.With(
		m.LabelsForWith(NameReasonFailAdd, ReasonSemaphoreAcquireFail)...,
	).Add(0)
	m.CountOfLeaderForwardRequest.Add(0)
	m.CountTimeoutTwoStep.Add(0)
	m.CountOfDeleteRequestPool.Add(0)
	m.CountOfRequestPoolAll.Add(0)
	m.LatencyOfRequestPool.Observe(0)
}

func (m *MetricsRequestPool) LabelsForWith(labelValues ...string) []string {
	result := make([]string, 0, len(m.labels)+len(labelValues))
	result = append(result, labelValues...)
	result = append(result, m.labels...)
	return result
}

var countBlackListOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "blacklist_count",
	Help:         "Count of nodes in blacklist on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nodesInBlackListOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "node_id_in_blacklist",
	Help:         "Node ID in blacklist on this channel.",
	LabelNames:   []string{NameBlackListNodeID},
	StatsdFormat: "%{#fqname}.%{" + NameBlackListNodeID + "}",
}

// MetricsBlacklist encapsulates blacklist metrics
type MetricsBlacklist struct {
	CountBlackList   Gauge
	NodesInBlackList Gauge

	labels []string
}

// NewMetricsBlacklist create new blacklist metrics
func NewMetricsBlacklist(p Provider, labelNames ...string) *MetricsBlacklist {
	countBlackListOptsTmp := NewGaugeOpts(countBlackListOpts, labelNames)
	nodesInBlackListOptsTmp := NewGaugeOpts(nodesInBlackListOpts, labelNames)
	return &MetricsBlacklist{
		CountBlackList:   p.NewGauge(countBlackListOptsTmp),
		NodesInBlackList: p.NewGauge(nodesInBlackListOptsTmp),
	}
}

func (m *MetricsBlacklist) With(labelValues ...string) *MetricsBlacklist {
	return &MetricsBlacklist{
		CountBlackList:   m.CountBlackList.With(labelValues...),
		NodesInBlackList: m.NodesInBlackList,
		labels:           labelValues,
	}
}

func (m *MetricsBlacklist) Initialize(nodes []uint64) {
	m.CountBlackList.Add(0)
	for _, n := range nodes {
		m.NodesInBlackList.With(
			m.LabelsForWith(NameBlackListNodeID, strconv.FormatUint(n, 10))...,
		).Set(0)
	}
}

func (m *MetricsBlacklist) LabelsForWith(labelValues ...string) []string {
	result := make([]string, 0, len(m.labels)+len(labelValues))
	result = append(result, labelValues...)
	result = append(result, m.labels...)
	return result
}

var consensusReconfigOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "consensus_reconfig",
	Help:         "Number of reconfiguration requests.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencySyncOpts = HistogramOpts{
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
	CountConsensusReconfig Counter
	LatencySync            Histogram
}

// NewMetricsConsensus create new consensus metrics
func NewMetricsConsensus(p Provider, labelNames ...string) *MetricsConsensus {
	consensusReconfigOptsTmp := NewCounterOpts(consensusReconfigOpts, labelNames)
	latencySyncOptsTmp := NewHistogramOpts(latencySyncOpts, labelNames)
	return &MetricsConsensus{
		CountConsensusReconfig: p.NewCounter(consensusReconfigOptsTmp),
		LatencySync:            p.NewHistogram(latencySyncOptsTmp),
	}
}

func (m *MetricsConsensus) With(labelValues ...string) *MetricsConsensus {
	return &MetricsConsensus{
		CountConsensusReconfig: m.CountConsensusReconfig.With(labelValues...),
		LatencySync:            m.LatencySync.With(labelValues...),
	}
}

func (m *MetricsConsensus) Initialize() {
	m.CountConsensusReconfig.Add(0)
	m.LatencySync.Observe(0)
}

var viewNumberOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_number",
	Help:         "The View number value.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var leaderIDOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_leader_id",
	Help:         "The leader id.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var proposalSequenceOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_proposal_sequence",
	Help:         "The sequence number within current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var decisionsInViewOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_decisions",
	Help:         "The number of decisions in the current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var phaseOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_phase",
	Help:         "Current consensus phase.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsInBatchOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_txs_in_batch",
	Help:         "The number of transactions per batch.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countBatchAllOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_batch_all",
	Help:         "Amount of batched processed.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsAllOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_count_txs_all",
	Help:         "Total amount of transactions.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var sizeOfBatchOpts = CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_size_batch",
	Help:         "An average batch size.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchProcessingOpts = HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "view_latency_batch_processing",
	Help:         "Amount of time it take to process batch.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchSaveOpts = HistogramOpts{
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
	ViewNumber             Gauge
	LeaderID               Gauge
	ProposalSequence       Gauge
	DecisionsInView        Gauge
	Phase                  Gauge
	CountTxsInBatch        Gauge
	CountBatchAll          Counter
	CountTxsAll            Counter
	SizeOfBatch            Counter
	LatencyBatchProcessing Histogram
	LatencyBatchSave       Histogram
}

// NewMetricsView create new view metrics
func NewMetricsView(p Provider, labelNames ...string) *MetricsView {
	viewNumberOptsTmp := NewGaugeOpts(viewNumberOpts, labelNames)
	leaderIDOptsTmp := NewGaugeOpts(leaderIDOpts, labelNames)
	proposalSequenceOptsTmp := NewGaugeOpts(proposalSequenceOpts, labelNames)
	decisionsInViewOptsTmp := NewGaugeOpts(decisionsInViewOpts, labelNames)
	phaseOptsTmp := NewGaugeOpts(phaseOpts, labelNames)
	countTxsInBatchOptsTmp := NewGaugeOpts(countTxsInBatchOpts, labelNames)
	countBatchAllOptsTmp := NewCounterOpts(countBatchAllOpts, labelNames)
	countTxsAllOptsTmp := NewCounterOpts(countTxsAllOpts, labelNames)
	sizeOfBatchOptsTmp := NewCounterOpts(sizeOfBatchOpts, labelNames)
	latencyBatchProcessingOptsTmp := NewHistogramOpts(latencyBatchProcessingOpts, labelNames)
	latencyBatchSaveOptsTmp := NewHistogramOpts(latencyBatchSaveOpts, labelNames)
	return &MetricsView{
		ViewNumber:             p.NewGauge(viewNumberOptsTmp),
		LeaderID:               p.NewGauge(leaderIDOptsTmp),
		ProposalSequence:       p.NewGauge(proposalSequenceOptsTmp),
		DecisionsInView:        p.NewGauge(decisionsInViewOptsTmp),
		Phase:                  p.NewGauge(phaseOptsTmp),
		CountTxsInBatch:        p.NewGauge(countTxsInBatchOptsTmp),
		CountBatchAll:          p.NewCounter(countBatchAllOptsTmp),
		CountTxsAll:            p.NewCounter(countTxsAllOptsTmp),
		SizeOfBatch:            p.NewCounter(sizeOfBatchOptsTmp),
		LatencyBatchProcessing: p.NewHistogram(latencyBatchProcessingOptsTmp),
		LatencyBatchSave:       p.NewHistogram(latencyBatchSaveOptsTmp),
	}
}

func (m *MetricsView) With(labelValues ...string) *MetricsView {
	return &MetricsView{
		ViewNumber:             m.ViewNumber.With(labelValues...),
		LeaderID:               m.LeaderID.With(labelValues...),
		ProposalSequence:       m.ProposalSequence.With(labelValues...),
		DecisionsInView:        m.DecisionsInView.With(labelValues...),
		Phase:                  m.Phase.With(labelValues...),
		CountTxsInBatch:        m.CountTxsInBatch.With(labelValues...),
		CountBatchAll:          m.CountBatchAll.With(labelValues...),
		CountTxsAll:            m.CountTxsAll.With(labelValues...),
		SizeOfBatch:            m.SizeOfBatch.With(labelValues...),
		LatencyBatchProcessing: m.LatencyBatchProcessing.With(labelValues...),
		LatencyBatchSave:       m.LatencyBatchSave.With(labelValues...),
	}
}

func (m *MetricsView) Initialize() {
	m.ViewNumber.Add(0)
	m.LeaderID.Add(0)
	m.ProposalSequence.Add(0)
	m.DecisionsInView.Add(0)
	m.Phase.Add(0)
	m.CountTxsInBatch.Add(0)
	m.CountBatchAll.Add(0)
	m.CountTxsAll.Add(0)
	m.SizeOfBatch.Add(0)
	m.LatencyBatchProcessing.Observe(0)
	m.LatencyBatchSave.Observe(0)
}

var currentViewOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_current_view",
	Help:         "current view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nextViewOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_next_view",
	Help:         "next view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var realViewOpts = GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "bft",
	Name:         "viewchange_real_view",
	Help:         "real view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

// MetricsViewChange encapsulates view change metrics
type MetricsViewChange struct {
	CurrentView Gauge
	NextView    Gauge
	RealView    Gauge
}

// NewMetricsViewChange create new view change metrics
func NewMetricsViewChange(p Provider, labelNames ...string) *MetricsViewChange {
	currentViewOptsTmp := NewGaugeOpts(currentViewOpts, labelNames)
	nextViewOptsTmp := NewGaugeOpts(nextViewOpts, labelNames)
	realViewOptsTmp := NewGaugeOpts(realViewOpts, labelNames)
	return &MetricsViewChange{
		CurrentView: p.NewGauge(currentViewOptsTmp),
		NextView:    p.NewGauge(nextViewOptsTmp),
		RealView:    p.NewGauge(realViewOptsTmp),
	}
}

func (m *MetricsViewChange) With(labelValues ...string) *MetricsViewChange {
	return &MetricsViewChange{
		CurrentView: m.CurrentView.With(labelValues...),
		NextView:    m.NextView.With(labelValues...),
		RealView:    m.RealView.With(labelValues...),
	}
}

func (m *MetricsViewChange) Initialize() {
	m.CurrentView.Add(0)
	m.NextView.Add(0)
	m.RealView.Add(0)
}
