// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package consensus

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	algorithm "github.com/SmartBFT-Go/consensus/internal/bft"
	bft "github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/metrics/disabled"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Consensus submits requests to be total ordered,
// and delivers to the application proposals by invoking Deliver() on it.
// The proposals contain batches of requests assembled together by the Assembler.
type Consensus struct {
	Config             types.Configuration
	Application        bft.Application
	Assembler          bft.Assembler
	WAL                bft.WriteAheadLog
	WALInitialContent  [][]byte
	Comm               bft.Comm
	Signer             bft.Signer
	Verifier           bft.Verifier
	MembershipNotifier bft.MembershipNotifier
	RequestInspector   bft.RequestInspector
	Synchronizer       bft.Synchronizer
	Logger             bft.Logger
	MetricsProvider    *bft.CustomerProvider
	Metadata           *protos.ViewMetadata
	LastProposal       types.Proposal
	LastSignatures     []types.Signature
	Scheduler          <-chan time.Time
	ViewChangerTicker  <-chan time.Time

	submittedChan chan struct{}
	inFlight      *algorithm.InFlightData
	checkpoint    *types.Checkpoint
	Pool          *algorithm.Pool
	viewChanger   *algorithm.ViewChanger
	controller    *algorithm.Controller
	collector     *algorithm.StateCollector
	state         *algorithm.PersistedState
	numberOfNodes uint64
	nodes         []uint64
	nodeMap       sync.Map

	consensusDone sync.WaitGroup
	stopOnce      sync.Once
	stopChan      chan struct{}

	consensusLock sync.RWMutex

	reconfigChan     chan types.Reconfig
	metricsBlacklist *algorithm.MetricsBlacklist
	metricsConsensus *algorithm.MetricsConsensus
	metricsView      *algorithm.MetricsView

	running uint64
}

func (c *Consensus) Complain(viewNum uint64, stopView bool) {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.viewChanger.StartViewChange(viewNum, stopView)
}

func (c *Consensus) Deliver(proposal types.Proposal, signatures []types.Signature) types.Reconfig {
	reconfig := c.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		c.Logger.Debugf("Detected a reconfig in deliver")
		c.reconfigChan <- reconfig
	}
	return reconfig
}

func (c *Consensus) Sync() types.SyncResponse {
	begin := time.Now()
	syncResponse := c.Synchronizer.Sync()
	c.metricsConsensus.LatencySync.Observe(time.Since(begin).Seconds())
	if syncResponse.Reconfig.InReplicatedDecisions {
		c.Logger.Debugf("Detected a reconfig in sync")
		c.reconfigChan <- types.Reconfig{
			InLatestDecision: true,
			CurrentNodes:     syncResponse.Reconfig.CurrentNodes,
			CurrentConfig:    syncResponse.Reconfig.CurrentConfig,
		}
	}
	return syncResponse
}

// GetLeaderID returns the current leader ID or zero if Consensus is not running
func (c *Consensus) GetLeaderID() uint64 {
	if atomic.LoadUint64(&c.running) == 0 {
		return 0
	}
	return c.controller.GetLeaderID()
}

func (c *Consensus) Start() error {
	if err := c.ValidateConfiguration(c.Comm.Nodes()); err != nil {
		return errors.Wrapf(err, "configuration is invalid")
	}

	if c.MetricsProvider == nil {
		c.MetricsProvider = bft.NewCustomerProvider(&disabled.Provider{})
	}
	c.metricsConsensus = algorithm.NewMetricsConsensus(c.MetricsProvider)
	c.metricsBlacklist = algorithm.NewMetricsBlacklist(c.MetricsProvider)
	c.metricsView = algorithm.NewMetricsView(c.MetricsProvider)

	c.consensusDone.Add(1)
	c.stopOnce = sync.Once{}
	c.stopChan = make(chan struct{})
	c.reconfigChan = make(chan types.Reconfig)
	c.consensusLock.Lock()
	defer c.consensusLock.Unlock()

	c.setNodes(c.Comm.Nodes())

	c.inFlight = &algorithm.InFlightData{}

	c.state = &algorithm.PersistedState{
		InFlightProposal: c.inFlight,
		Entries:          c.WALInitialContent,
		Logger:           c.Logger,
		WAL:              c.WAL,
	}

	c.checkpoint = &types.Checkpoint{}
	c.checkpoint.Set(c.LastProposal, c.LastSignatures)

	c.createComponents()
	opts := algorithm.PoolOptions{
		QueueSize:         int64(c.Config.RequestPoolSize),
		ForwardTimeout:    c.Config.RequestForwardTimeout,
		ComplainTimeout:   c.Config.RequestComplainTimeout,
		AutoRemoveTimeout: c.Config.RequestAutoRemoveTimeout,
		RequestMaxBytes:   c.Config.RequestMaxBytes,
		SubmitTimeout:     c.Config.RequestPoolSubmitTimeout,
		MetricsProvider:   c.MetricsProvider,
	}
	c.submittedChan = make(chan struct{}, 1)
	c.Pool = algorithm.NewPool(c.Logger, c.RequestInspector, c.controller, opts, c.submittedChan)
	c.continueCreateComponents()

	c.Logger.Debugf("Application started with view %d, seq %d, and decisions %d", c.Metadata.ViewId, c.Metadata.LatestSequence, c.Metadata.DecisionsInView)
	view, seq, dec := c.setViewAndSeq(c.Metadata.ViewId, c.Metadata.LatestSequence, c.Metadata.DecisionsInView)

	c.waitForEachOther()

	go c.run()

	c.startComponents(view, seq, dec, true)

	atomic.StoreUint64(&c.running, 1)

	return nil
}

func (c *Consensus) run() {
	defer func() {
		c.Logger.Infof("Exiting")
		atomic.StoreUint64(&c.running, 0)
		c.Stop()
	}()

	defer c.consensusDone.Done()

	for {
		select {
		case reconfig := <-c.reconfigChan:
			c.reconfig(reconfig)
		case <-c.stopChan:
			return
		}
	}
}

func (c *Consensus) reconfig(reconfig types.Reconfig) {
	c.Logger.Debugf("Starting reconfig")
	c.consensusLock.Lock()
	defer c.consensusLock.Unlock()

	// make sure all components are stopped
	c.viewChanger.Stop()
	c.controller.StopWithPoolPause()
	c.collector.Stop()

	var exist bool
	for _, n := range reconfig.CurrentNodes {
		if c.Config.SelfID == n {
			exist = true
			break
		}
	}

	if !exist {
		c.Logger.Infof("Evicted in reconfiguration, shutting down")
		c.close()
		return
	}

	c.Config = reconfig.CurrentConfig
	if err := c.ValidateConfiguration(reconfig.CurrentNodes); err != nil {
		if strings.Contains(err.Error(), "nodes does not contain the SelfID") {
			c.close()
			c.Logger.Infof("Closing consensus since this node is not in the current set of nodes")
			return
		}
		c.Logger.Panicf("Configuration is invalid, error: %v", err)
	}

	c.setNodes(reconfig.CurrentNodes)

	c.createComponents()
	opts := algorithm.PoolOptions{
		ForwardTimeout:    c.Config.RequestForwardTimeout,
		ComplainTimeout:   c.Config.RequestComplainTimeout,
		AutoRemoveTimeout: c.Config.RequestAutoRemoveTimeout,
		RequestMaxBytes:   c.Config.RequestMaxBytes,
		SubmitTimeout:     c.Config.RequestPoolSubmitTimeout,
	}
	c.Pool.ChangeTimeouts(c.controller, opts) // TODO handle reconfiguration of queue size in the pool
	c.continueCreateComponents()

	proposal, _ := c.checkpoint.Get()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(proposal.Metadata, md); err != nil {
		c.Logger.Panicf("Couldn't unmarshal the checkpoint metadata, error: %v", err)
	}
	c.Logger.Debugf("Checkpoint with view %d and seq %d", md.ViewId, md.LatestSequence)

	view, seq, dec := c.setViewAndSeq(md.ViewId, md.LatestSequence, md.DecisionsInView)

	c.waitForEachOther()

	c.startComponents(view, seq, dec, false)

	c.Pool.RestartTimers()

	c.metricsConsensus.CountConsensusReconfig.Add(1)

	c.Logger.Debugf("Reconfig is done")
}

func (c *Consensus) close() {
	c.stopOnce.Do(
		func() {
			select {
			case <-c.stopChan:
				return
			default:
				close(c.stopChan)
			}
		},
	)
}

func (c *Consensus) Stop() {
	c.consensusLock.RLock()
	c.viewChanger.Stop()
	c.controller.Stop()
	c.collector.Stop()
	c.consensusLock.RUnlock()
	c.close()
	c.consensusDone.Wait()
}

func (c *Consensus) HandleMessage(sender uint64, m *protos.Message) {
	if _, exists := c.nodeMap.Load(sender); !exists {
		c.Logger.Warnf("Received message from unexpected node %d", sender)
		return
	}
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.controller.ProcessMessages(sender, m)
}

func (c *Consensus) HandleRequest(sender uint64, req []byte) {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.controller.HandleRequest(sender, req)
}

func (c *Consensus) SubmitRequest(req []byte) error {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	if c.GetLeaderID() == 0 {
		return errors.Errorf("no leader")
	}
	c.Logger.Debugf("Submit Request: %s", c.RequestInspector.RequestID(req))
	return c.controller.SubmitRequest(req)
}

func (c *Consensus) proposalMaker() *algorithm.ProposalMaker {
	return &algorithm.ProposalMaker{
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		Checkpoint:         c.checkpoint,
		State:              c.state,
		Comm:               c.controller,
		Decider:            c.controller,
		Logger:             c.Logger,
		MetricsBlacklist:   c.metricsBlacklist,
		MetricsView:        c.metricsView,
		Signer:             c.Signer,
		MembershipNotifier: c.MembershipNotifier,
		SelfID:             c.Config.SelfID,
		Sync:               c.controller,
		FailureDetector:    c,
		Verifier:           c.Verifier,
		N:                  c.numberOfNodes,
		InMsqQSize:         int(c.Config.IncomingMessageBufferSize),
		ViewSequences:      c.controller.ViewSequences,
	}
}

func (c *Consensus) ValidateConfiguration(nodes []uint64) error {
	if err := c.Config.Validate(); err != nil {
		return errors.Wrap(err, "bad configuration")
	}

	nodeSet := make(map[uint64]bool)
	for _, val := range nodes {
		if val == 0 {
			return errors.Errorf("nodes contains node id 0 which is not permitted, nodes: %v", nodes)
		}
		nodeSet[val] = true
	}

	if !nodeSet[c.Config.SelfID] {
		return errors.Errorf("nodes does not contain the SelfID: %d, nodes: %v", c.Config.SelfID, nodes)
	}

	if len(nodeSet) != len(nodes) {
		return errors.Errorf("nodes contains duplicate IDs, nodes: %v", nodes)
	}

	return nil
}

func (c *Consensus) setNodes(nodes []uint64) {
	for _, n := range c.nodes {
		c.nodeMap.Delete(n)
	}

	c.numberOfNodes = uint64(len(nodes))
	c.nodes = sortNodes(nodes)
	for _, n := range nodes {
		c.nodeMap.Store(n, struct{}{})
	}
}

func sortNodes(nodes []uint64) []uint64 {
	sorted := make([]uint64, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	return sorted
}

func (c *Consensus) createComponents() {
	c.viewChanger = &algorithm.ViewChanger{
		SelfID:             c.Config.SelfID,
		N:                  c.numberOfNodes,
		NodesList:          c.nodes,
		LeaderRotation:     c.Config.LeaderRotation,
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		SpeedUpViewChange:  c.Config.SpeedUpViewChange,
		Logger:             c.Logger,
		Signer:             c.Signer,
		Verifier:           c.Verifier,
		Checkpoint:         c.checkpoint,
		InFlight:           c.inFlight,
		State:              c.state,
		// Controller later
		// RequestsTimer later
		Ticker:            c.ViewChangerTicker,
		ResendTimeout:     c.Config.ViewChangeResendInterval,
		ViewChangeTimeout: c.Config.ViewChangeTimeout,
		InMsqQSize:        int(c.Config.IncomingMessageBufferSize),
		MetricsViewChange: algorithm.NewMetricsViewChange(c.MetricsProvider),
		MetricsBlacklist:  c.metricsBlacklist,
		MetricsView:       c.metricsView,
	}

	c.collector = &algorithm.StateCollector{
		SelfID:         c.Config.SelfID,
		N:              c.numberOfNodes,
		Logger:         c.Logger,
		CollectTimeout: c.Config.CollectTimeout,
	}

	c.controller = &algorithm.Controller{
		Checkpoint:         c.checkpoint,
		WAL:                c.WAL,
		ID:                 c.Config.SelfID,
		N:                  c.numberOfNodes,
		NodesList:          c.nodes,
		LeaderRotation:     c.Config.LeaderRotation,
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		Verifier:           c.Verifier,
		Logger:             c.Logger,
		Assembler:          c.Assembler,
		Application:        c,
		FailureDetector:    c,
		Synchronizer:       c,
		Comm:               c.Comm,
		Signer:             c.Signer,
		RequestInspector:   c.RequestInspector,
		ViewChanger:        c.viewChanger,
		ViewSequences:      &atomic.Value{},
		Collector:          c.collector,
		State:              c.state,
		InFlight:           c.inFlight,
		MetricsView:        c.metricsView,
	}
	c.viewChanger.Application = &algorithm.MutuallyExclusiveDeliver{C: c.controller}
	c.viewChanger.Comm = c.controller
	c.viewChanger.Synchronizer = c.controller

	c.controller.ProposerBuilder = c.proposalMaker()
}

func (c *Consensus) continueCreateComponents() {
	batchBuilder := algorithm.NewBatchBuilder(c.Pool, c.submittedChan, c.Config.RequestBatchMaxCount, c.Config.RequestBatchMaxBytes, c.Config.RequestBatchMaxInterval)
	leaderMonitor := algorithm.NewHeartbeatMonitor(c.Scheduler, c.Logger, c.Config.LeaderHeartbeatTimeout, c.Config.LeaderHeartbeatCount, c.controller, c.numberOfNodes, c.controller, c.controller.ViewSequences, c.Config.NumOfTicksBehindBeforeSyncing)
	c.controller.RequestPool = c.Pool
	c.controller.Batcher = batchBuilder
	c.controller.LeaderMonitor = leaderMonitor

	c.viewChanger.Controller = c.controller
	c.viewChanger.Pruner = c.controller
	c.viewChanger.RequestsTimer = c.Pool
	c.viewChanger.ViewSequences = c.controller.ViewSequences
}

func (c *Consensus) setViewAndSeq(view, seq, dec uint64) (newView, newSeq, newDec uint64) {
	newView = view
	newSeq = seq
	// decisions in view is incremented after delivery,
	// so if we delivered to the application proposal with decisions i,
	// then we are expecting to be proposed a proposal with decisions i+1,
	// unless this is the genesis block, or after a view change
	newDec = dec + 1
	if seq == 0 {
		newDec = 0
	}
	viewChange, err := c.state.LoadViewChangeIfApplicable()
	if err != nil {
		c.Logger.Panicf("Failed loading view change, error: %v", err)
	}
	if viewChange == nil {
		c.Logger.Debugf("No view change to restore")
	} else if viewChange.NextView >= view {
		// Check if the view change has a newer view
		c.Logger.Debugf("Restoring from view change with view %d", viewChange.NextView)
		newView = viewChange.NextView
		restoreChan := make(chan struct{}, 1)
		restoreChan <- struct{}{}
		c.viewChanger.Restore = restoreChan
	}

	viewSeq, err := c.state.LoadNewViewIfApplicable()
	if err != nil {
		c.Logger.Panicf("Failed loading new view, error: %v", err)
	}
	if viewSeq == nil {
		c.Logger.Debugf("No new view to restore")
	} else if viewSeq.Seq >= seq {
		// Check if metadata should be taken from the restored new view
		c.Logger.Debugf("Restoring from new view with view %d and seq %d", viewSeq.View, viewSeq.Seq)
		newView = viewSeq.View
		newSeq = viewSeq.Seq
		newDec = 0
	}
	return newView, newSeq, newDec
}

func (c *Consensus) waitForEachOther() {
	c.viewChanger.ControllerStartedWG = sync.WaitGroup{}
	c.viewChanger.ControllerStartedWG.Add(1)
	c.controller.StartedWG = &c.viewChanger.ControllerStartedWG
}

func (c *Consensus) startComponents(view, seq, dec uint64, configSync bool) {
	// If we delivered to the application proposal with sequence i,
	// then we are expecting to be proposed a proposal with sequence i+1.
	c.collector.Start()
	c.viewChanger.Start(view)
	if configSync {
		c.controller.Start(view, seq+1, dec, c.Config.SyncOnStart)
	} else {
		c.controller.Start(view, seq+1, dec, false)
	}
}
