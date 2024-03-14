package api

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/hyperledger-labs/SmartBFT/pkg/metrics"
)

const (
	ReasonRequestMaxBytes      = "MAX_BYTES"
	ReasonSemaphoreAcquireFail = "SEMAPHORE_ACQUIRE_FAIL"
)

func NewGaugeOpts(old metrics.GaugeOpts, labelNames []string) metrics.GaugeOpts {
	return metrics.GaugeOpts{
		Namespace:    old.Namespace,
		Subsystem:    old.Subsystem,
		Name:         old.Name,
		Help:         old.Help,
		LabelNames:   makeLabelNames(labelNames, old.LabelNames...),
		LabelHelp:    old.LabelHelp,
		StatsdFormat: makeStatsdFormat(labelNames, old.StatsdFormat),
	}
}

func NewCounterOpts(old metrics.CounterOpts, labelNames []string) metrics.CounterOpts {
	return metrics.CounterOpts{
		Namespace:    old.Namespace,
		Subsystem:    old.Subsystem,
		Name:         old.Name,
		Help:         old.Help,
		LabelNames:   makeLabelNames(labelNames, old.LabelNames...),
		LabelHelp:    old.LabelHelp,
		StatsdFormat: makeStatsdFormat(labelNames, old.StatsdFormat),
	}
}

func NewHistogramOpts(old metrics.HistogramOpts, labelNames []string) metrics.HistogramOpts {
	return metrics.HistogramOpts{
		Namespace:    old.Namespace,
		Subsystem:    old.Subsystem,
		Name:         old.Name,
		Help:         old.Help,
		Buckets:      old.Buckets,
		LabelNames:   makeLabelNames(labelNames, old.LabelNames...),
		LabelHelp:    old.LabelHelp,
		StatsdFormat: makeStatsdFormat(labelNames, old.StatsdFormat),
	}
}

func makeStatsdFormat(labelNames []string, str string) string {
	sort.Strings(labelNames)
	for _, s := range labelNames {
		str += fmt.Sprintf(".%%{%s}", s)
	}

	return str
}

func makeLabelNames(labelNames []string, names ...string) []string {
	ln := make([]string, 0, len(names)+len(labelNames))
	ln = append(ln, names...)
	sort.Strings(labelNames)
	ln = append(ln, labelNames...)
	return ln
}

type Metrics struct {
	MetricsRequestPool *MetricsRequestPool
	MetricsBlacklist   *MetricsBlacklist
	MetricsConsensus   *MetricsConsensus
	MetricsView        *MetricsView
	MetricsViewChange  *MetricsViewChange
}

func NewMetrics(p metrics.Provider, labelNames ...string) *Metrics {
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

var countOfRequestPoolOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_of_elements",
	Help:         "Number of elements in the consensus request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfFailAddRequestToPoolOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_of_fail_add_request",
	Help:         "Number of requests pool insertion failure.",
	LabelNames:   []string{"reason"},
	StatsdFormat: "%{#fqname}.%{reason}",
}

// ForwardTimeout
var countOfLeaderForwardRequestOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_leader_forward_request",
	Help:         "Number of requests forwarded to the leader.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTimeoutTwoStepOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_timeout_two_step",
	Help:         "Number of times requests reached second timeout.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfDeleteRequestPoolOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_of_delete_request",
	Help:         "Number of elements removed from the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countOfRequestPoolAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "pool_count_of_elements_all",
	Help:         "Total amount of elements in the request pool.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyOfRequestPoolOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
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
func NewMetricsRequestPool(p metrics.Provider, labelNames ...string) *MetricsRequestPool {
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
		m.LabelsForWith("reason", ReasonRequestMaxBytes)...,
	).Add(0)
	m.CountOfFailAddRequestToPool.With(
		m.LabelsForWith("reason", ReasonSemaphoreAcquireFail)...,
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

var countBlackListOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "blacklist_count",
	Help:         "Count of nodes in blacklist on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nodesInBlackListOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "node_id_in_blacklist",
	Help:         "Node ID in blacklist on this channel.",
	LabelNames:   []string{"blackid"},
	StatsdFormat: "%{#fqname}.%{blackid}",
}

// MetricsBlacklist encapsulates blacklist metrics
type MetricsBlacklist struct {
	CountBlackList   metrics.Gauge
	NodesInBlackList metrics.Gauge

	labels []string
}

// NewMetricsBlacklist create new blacklist metrics
func NewMetricsBlacklist(p metrics.Provider, labelNames ...string) *MetricsBlacklist {
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
			m.LabelsForWith("blackid", strconv.FormatUint(n, 10))...,
		).Set(0)
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
	Subsystem:    "smartbft",
	Name:         "consensus_reconfig",
	Help:         "Number of reconfiguration requests.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencySyncOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
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
func NewMetricsConsensus(p metrics.Provider, labelNames ...string) *MetricsConsensus {
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

var viewNumberOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_number",
	Help:         "The View number value.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var leaderIDOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_leader_id",
	Help:         "The leader id.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var proposalSequenceOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_proposal_sequence",
	Help:         "The sequence number within current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var decisionsInViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_decisions",
	Help:         "The number of decisions in the current view.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var phaseOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_phase",
	Help:         "Current consensus phase.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsInBatchOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_count_txs_in_batch",
	Help:         "The number of transactions per batch.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countBatchAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_count_batch_all",
	Help:         "Amount of batched processed.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var countTxsAllOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_count_txs_all",
	Help:         "Total amount of transactions.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var sizeOfBatchOpts = metrics.CounterOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_size_batch",
	Help:         "An average batch size.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchProcessingOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "view_latency_batch_processing",
	Help:         "Amount of time it take to process batch.",
	Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var latencyBatchSaveOpts = metrics.HistogramOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
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
func NewMetricsView(p metrics.Provider, labelNames ...string) *MetricsView {
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

var currentViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "viewchange_current_view",
	Help:         "current view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var nextViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
	Name:         "viewchange_next_view",
	Help:         "next view of viewchange on this channel.",
	LabelNames:   []string{},
	StatsdFormat: "%{#fqname}",
}

var realViewOpts = metrics.GaugeOpts{
	Namespace:    "consensus",
	Subsystem:    "smartbft",
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
func NewMetricsViewChange(p metrics.Provider, labelNames ...string) *MetricsViewChange {
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
