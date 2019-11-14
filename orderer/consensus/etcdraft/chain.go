/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"context"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"go.etcd.io/etcd/wal"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
)

const (
	// DefaultSnapshotCatchUpEntries is the default number of entries
	// to preserve in memory when a snapshot is taken. This is for
	// slow followers to catch up.
	DefaultSnapshotCatchUpEntries = uint64(20)

	// DefaultSnapshotIntervalSize is the default snapshot interval. It is
	// used if SnapshotIntervalSize is not provided in channel config options.
	// It is needed to enforce snapshot being set.
	DefaultSnapshotIntervalSize = 20 * MEGABYTE // 20 MB

	// DefaultEvictionSuspicion is the threshold that a node will start
	// suspecting its own eviction if it has been leaderless for this
	// period of time.
	DefaultEvictionSuspicion = time.Minute * 10

	// DefaultLeaderlessCheckInterval is the interval that a chain checks
	// its own leadership status.
	DefaultLeaderlessCheckInterval = time.Second * 10
)

//go:generate counterfeiter -o mocks/configurator.go . Configurator

// Configurator is used to configure the communication layer
// when the chain starts.
type Configurator interface {
	Configure(channel string, newNodes []cluster.RemoteNode)
}

//go:generate counterfeiter -o mocks/mock_rpc.go . RPC

// RPC is used to mock the transport layer in tests.
type RPC interface {
	SendConsensus(dest uint64, msg *orderer.ConsensusRequest) error
	SendSubmit(dest uint64, request *orderer.SubmitRequest) error
}

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *common.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

// CreateBlockPuller is a function to create BlockPuller on demand.
// It is passed into chain initializer so that tests could mock this.
type CreateBlockPuller func() (BlockPuller, error)

// Options contains all the configurations relevant to the chain.
type Options struct {
	RaftID uint64

	Clock clock.Clock

	WALDir               string
	SnapDir              string
	SnapshotIntervalSize uint32

	// This is configurable mainly for testing purpose. Users are not
	// expected to alter this. Instead, DefaultSnapshotCatchUpEntries is used.
	SnapshotCatchUpEntries uint64

	MemoryStorage MemoryStorage
	Logger        *flogging.FabricLogger

	TickInterval      time.Duration
	ElectionTick      int
	HeartbeatTick     int
	MaxSizePerMsg     uint64
	MaxInflightBlocks int

	// BlockMetdata and Consenters should only be modified while under lock
	// of raftMetadataLock
	BlockMetadata *etcdraft.BlockMetadata
	Consenters    map[uint64]*etcdraft.Consenter

	// MigrationInit is set when the node starts right after consensus-type migration
	MigrationInit bool

	Metrics *Metrics
	Cert    []byte

	EvictionSuspicion   time.Duration
	LeaderCheckInterval time.Duration
}

type submit struct {
	req    *orderer.SubmitRequest
	leader chan uint64
}

type gc struct {
	index uint64
	state raftpb.ConfState
	data  []byte
}

// Chain implements consensus.Chain interface.
type Chain struct {
	configurator Configurator

	rpc RPC

	raftID    uint64
	channelID string

	lastKnownLeader uint64

	submitC  chan *submit
	applyC   chan apply
	observeC chan<- raft.SoftState // Notifies external observer on leader change (passed in optionally as an argument for tests)
	haltC    chan struct{}         // Signals to goroutines that the chain is halting
	doneC    chan struct{}         // Closes when the chain halts
	startC   chan struct{}         // Closes when the node is started
	snapC    chan *raftpb.Snapshot // Signal to catch up with snapshot
	gcC      chan *gc              // Signal to take snapshot

	errorCLock sync.RWMutex
	errorC     chan struct{} // returned by Errored()

	raftMetadataLock     sync.RWMutex
	confChangeInProgress *raftpb.ConfChange
	justElected          bool // this is true when node has just been elected
	configInflight       bool // this is true when there is config block or ConfChange in flight
	blockInflight        int  // number of in flight blocks

	clock clock.Clock // Tests can inject a fake clock

	support consensus.ConsenterSupport

	lastBlock    *common.Block
	appliedIndex uint64

	// needed by snapshotting
	sizeLimit        uint32 // SnapshotIntervalSize in bytes
	accDataSize      uint32 // accumulative data size since last snapshot
	lastSnapBlockNum uint64
	confState        raftpb.ConfState // Etcdraft requires ConfState to be persisted within snapshot

	createPuller CreateBlockPuller // func used to create BlockPuller on demand

	fresh bool // indicate if this is a fresh raft node

	// this is exported so that test can use `Node.Status()` to get raft node status.
	Node *node
	opts Options

	Metrics *Metrics
	logger  *flogging.FabricLogger

	periodicChecker *PeriodicCheck

	haltCallback func()
}

// NewChain constructs a chain object.
func NewChain(
	support consensus.ConsenterSupport,
	opts Options,
	conf Configurator,
	rpc RPC,
	f CreateBlockPuller,
	haltCallback func(),
	observeC chan<- raft.SoftState) (*Chain, error) {

	lg := opts.Logger.With("channel", support.ChainID(), "node", opts.RaftID)

	fresh := !wal.Exist(opts.WALDir)
	storage, err := CreateStorage(lg, opts.WALDir, opts.SnapDir, opts.MemoryStorage)
	if err != nil {
		return nil, errors.Errorf("failed to restore persisted raft data: %s", err)
	}

	if opts.SnapshotCatchUpEntries == 0 {
		storage.SnapshotCatchUpEntries = DefaultSnapshotCatchUpEntries
	} else {
		storage.SnapshotCatchUpEntries = opts.SnapshotCatchUpEntries
	}

	sizeLimit := opts.SnapshotIntervalSize
	if sizeLimit == 0 {
		sizeLimit = DefaultSnapshotIntervalSize
	}

	// get block number in last snapshot, if exists
	var snapBlkNum uint64
	var cc raftpb.ConfState
	if s := storage.Snapshot(); !raft.IsEmptySnap(s) {
		b := utils.UnmarshalBlockOrPanic(s.Data)
		snapBlkNum = b.Header.Number
		cc = s.Metadata.ConfState
	}

	b := support.Block(support.Height() - 1)
	if b == nil {
		return nil, errors.Errorf("failed to get last block")
	}

	c := &Chain{
		configurator:     conf,
		rpc:              rpc,
		channelID:        support.ChainID(),
		raftID:           opts.RaftID,
		submitC:          make(chan *submit),
		applyC:           make(chan apply),
		haltC:            make(chan struct{}),
		doneC:            make(chan struct{}),
		startC:           make(chan struct{}),
		snapC:            make(chan *raftpb.Snapshot),
		errorC:           make(chan struct{}),
		gcC:              make(chan *gc),
		observeC:         observeC,
		support:          support,
		fresh:            fresh,
		appliedIndex:     opts.BlockMetadata.RaftIndex,
		lastBlock:        b,
		sizeLimit:        sizeLimit,
		lastSnapBlockNum: snapBlkNum,
		confState:        cc,
		createPuller:     f,
		clock:            opts.Clock,
		haltCallback:     haltCallback,
		Metrics: &Metrics{
			ClusterSize:             opts.Metrics.ClusterSize.With("channel", support.ChainID()),
			IsLeader:                opts.Metrics.IsLeader.With("channel", support.ChainID()),
			CommittedBlockNumber:    opts.Metrics.CommittedBlockNumber.With("channel", support.ChainID()),
			SnapshotBlockNumber:     opts.Metrics.SnapshotBlockNumber.With("channel", support.ChainID()),
			LeaderChanges:           opts.Metrics.LeaderChanges.With("channel", support.ChainID()),
			ProposalFailures:        opts.Metrics.ProposalFailures.With("channel", support.ChainID()),
			DataPersistDuration:     opts.Metrics.DataPersistDuration.With("channel", support.ChainID()),
			NormalProposalsReceived: opts.Metrics.NormalProposalsReceived.With("channel", support.ChainID()),
			ConfigProposalsReceived: opts.Metrics.ConfigProposalsReceived.With("channel", support.ChainID()),
		},
		logger: lg,
		opts:   opts,
	}

	// Sets initial values for metrics
	c.Metrics.ClusterSize.Set(float64(len(c.opts.BlockMetadata.ConsenterIds)))
	c.Metrics.IsLeader.Set(float64(0)) // all nodes start out as followers
	c.Metrics.CommittedBlockNumber.Set(float64(c.lastBlock.Header.Number))
	c.Metrics.SnapshotBlockNumber.Set(float64(c.lastSnapBlockNum))

	// DO NOT use Applied option in config, see https://github.com/etcd-io/etcd/issues/10217
	// We guard against replay of written blocks with `appliedIndex` instead.
	config := &raft.Config{
		ID:              c.raftID,
		ElectionTick:    c.opts.ElectionTick,
		HeartbeatTick:   c.opts.HeartbeatTick,
		MaxSizePerMsg:   c.opts.MaxSizePerMsg,
		MaxInflightMsgs: c.opts.MaxInflightBlocks,
		Logger:          c.logger,
		Storage:         c.opts.MemoryStorage,
		// PreVote prevents reconnected node from disturbing network.
		// See etcd/raft doc for more details.
		PreVote:                   true,
		CheckQuorum:               true,
		DisableProposalForwarding: true, // This prevents blocks from being accidentally proposed by followers
	}

	c.Node = &node{
		chainID:      c.channelID,
		chain:        c,
		logger:       c.logger,
		metrics:      c.Metrics,
		storage:      storage,
		rpc:          c.rpc,
		config:       config,
		tickInterval: c.opts.TickInterval,
		clock:        c.clock,
		metadata:     c.opts.BlockMetadata,
	}

	return c, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting Raft node")

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	isJoin := c.support.Height() > 1
	if isJoin && c.opts.MigrationInit {
		isJoin = false
		c.logger.Infof("Consensus-type migration detected, starting new raft node on an existing channel; height=%d", c.support.Height())
	}
	c.Node.start(c.fresh, isJoin)

	close(c.startC)
	close(c.errorC)

	go c.gc()
	go c.serveRequest()

	es := c.newEvictionSuspector()

	interval := DefaultLeaderlessCheckInterval
	if c.opts.LeaderCheckInterval != 0 {
		interval = c.opts.LeaderCheckInterval
	}

	c.periodicChecker = &PeriodicCheck{
		Logger:        c.logger,
		Report:        es.confirmSuspicion,
		CheckInterval: interval,
		Condition:     c.suspectEviction,
	}
	c.periodicChecker.Run()
}

// Order submits normal type transactions for ordering.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	c.Metrics.NormalProposalsReceived.Add(1)
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
}

// Configure submits config type transactions for ordering.
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	c.Metrics.ConfigProposalsReceived.Add(1)
	if err := c.checkConfigUpdateValidity(env); err != nil {
		c.logger.Warnf("Rejected config: %s", err)
		c.Metrics.ProposalFailures.Add(1)
		return err
	}
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
}

// Validate the config update for being of Type A or Type B as described in the design doc.
func (c *Chain) checkConfigUpdateValidity(ctx *common.Envelope) error {
	var err error
	payload, err := utils.UnmarshalPayload(ctx.Payload)
	if err != nil {
		return err
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err
	}

	if chdr.Type != int32(common.HeaderType_ORDERER_TRANSACTION) &&
		chdr.Type != int32(common.HeaderType_CONFIG) {
		return errors.Errorf("config transaction has unknown header type: %s", common.HeaderType(chdr.Type))
	}

	if chdr.Type == int32(common.HeaderType_ORDERER_TRANSACTION) {
		newChannelConfig, err := utils.UnmarshalEnvelope(payload.Data)
		if err != nil {
			return err
		}

		payload, err = utils.UnmarshalPayload(newChannelConfig.Payload)
		if err != nil {
			return err
		}
	}

	configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
	if err != nil {
		return err
	}

	metadata, err := MetadataFromConfigUpdate(configUpdate)
	if err != nil {
		return err
	}

	if metadata == nil {
		return nil // ConsensusType is not updated
	}

	if err = CheckConfigMetadata(metadata); err != nil {
		return err
	}

	switch chdr.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		c.raftMetadataLock.RLock()
		set := MembershipByCert(c.opts.Consenters)
		c.raftMetadataLock.RUnlock()

		for _, c := range metadata.Consenters {
			if _, exits := set[string(c.ClientTlsCert)]; !exits {
				return errors.Errorf("new channel has consenter that is not part of system consenter set")
			}
		}

		return nil

	case int32(common.HeaderType_CONFIG):
		c.raftMetadataLock.RLock()
		_, err = ComputeMembershipChanges(c.opts.BlockMetadata, c.opts.Consenters, metadata.Consenters)
		c.raftMetadataLock.RUnlock()

		return err

	default:
		// panic here because we have just check header type and return early
		c.logger.Panicf("Programming error, unknown header type")
	}

	return nil
}

// WaitReady blocks when the chain:
// - is catching up with other nodes using snapshot
//
// In any other case, it returns right away.
func (c *Chain) WaitReady() error {
	if err := c.isRunning(); err != nil {
		return err
	}

	select {
	case c.submitC <- nil:
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}

	return nil
}

// Errored returns a channel that closes when the chain stops.
func (c *Chain) Errored() <-chan struct{} {
	c.errorCLock.RLock()
	defer c.errorCLock.RUnlock()
	return c.errorC
}

// Halt stops the chain.
func (c *Chain) Halt() {
	select {
	case <-c.startC:
	default:
		c.logger.Warnf("Attempted to halt a chain that has not started")
		return
	}

	select {
	case c.haltC <- struct{}{}:
	case <-c.doneC:
		return
	}
	<-c.doneC

	if c.haltCallback != nil {
		c.haltCallback()
	}
}

func (c *Chain) isRunning() error {
	select {
	case <-c.startC:
	default:
		return errors.Errorf("chain is not started")
	}

	select {
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	default:
	}

	return nil
}

// Consensus passes the given ConsensusRequest message to the raft.Node instance
func (c *Chain) Consensus(req *orderer.ConsensusRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		return err
	}

	stepMsg := &raftpb.Message{}
	if err := proto.Unmarshal(req.Payload, stepMsg); err != nil {
		return fmt.Errorf("failed to unmarshal StepRequest payload to Raft Message: %s", err)
	}

	if err := c.Node.Step(context.TODO(), *stepMsg); err != nil {
		return fmt.Errorf("failed to process Raft Step message: %s", err)
	}

	return nil
}

// Submit forwards the incoming request to:
// - the local serveRequest goroutine if this is leader
// - the actual leader via the transport mechanism
// The call fails if there's no leader elected yet.
func (c *Chain) Submit(req *orderer.SubmitRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		c.Metrics.ProposalFailures.Add(1)
		return err
	}

	leadC := make(chan uint64, 1)
	select {
	case c.submitC <- &submit{req, leadC}:
		lead := <-leadC
		if lead == raft.None {
			c.Metrics.ProposalFailures.Add(1)
			return errors.Errorf("no Raft leader")
		}

		if lead != c.raftID {
			if err := c.rpc.SendSubmit(lead, req); err != nil {
				c.Metrics.ProposalFailures.Add(1)
				return err
			}
		}

	case <-c.doneC:
		c.Metrics.ProposalFailures.Add(1)
		return errors.Errorf("chain is stopped")
	}

	return nil
}

type apply struct {
	entries []raftpb.Entry
	soft    *raft.SoftState
}

func isCandidate(state raft.StateType) bool {
	return state == raft.StatePreCandidate || state == raft.StateCandidate
}

func (c *Chain) serveRequest() {
	ticking := false
	timer := c.clock.NewTimer(time.Second)
	// we need a stopped timer rather than nil,
	// because we will be select waiting on timer.C()
	if !timer.Stop() {
		<-timer.C()
	}

	// if timer is already started, this is a no-op
	startTimer := func() {
		if !ticking {
			ticking = true
			timer.Reset(c.support.SharedConfig().BatchTimeout())
		}
	}

	stopTimer := func() {
		if !timer.Stop() && ticking {
			// we only need to drain the channel if the timer expired (not explicitly stopped)
			<-timer.C()
		}
		ticking = false
	}

	var soft raft.SoftState
	submitC := c.submitC
	var bc *blockCreator

	var propC chan<- *common.Block
	var cancelProp context.CancelFunc
	cancelProp = func() {} // no-op as initial value

	becomeLeader := func() (chan<- *common.Block, context.CancelFunc) {
		c.Metrics.IsLeader.Set(1)

		c.blockInflight = 0
		c.justElected = true
		submitC = nil
		ch := make(chan *common.Block, c.opts.MaxInflightBlocks)

		// if there is unfinished ConfChange, we should resume the effort to propose it as
		// new leader, and wait for it to be committed before start serving new requests.
		if cc := c.getInFlightConfChange(); cc != nil {
			// The reason `ProposeConfChange` should be called in go routine is documented in `writeConfigBlock` method.
			go func() {
				if err := c.Node.ProposeConfChange(context.TODO(), *cc); err != nil {
					c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
				}
			}()

			c.confChangeInProgress = cc
			c.configInflight = true
		}

		// Leader should call Propose in go routine, because this method may be blocked
		// if node is leaderless (this can happen when leader steps down in a heavily
		// loaded network). We need to make sure applyC can still be consumed properly.
		ctx, cancel := context.WithCancel(context.Background())
		go func(ctx context.Context, ch <-chan *common.Block) {
			for {
				select {
				case b := <-ch:
					data := utils.MarshalOrPanic(b)
					if err := c.Node.Propose(ctx, data); err != nil {
						c.logger.Errorf("Failed to propose block [%d] to raft and discard %d blocks in queue: %s", b.Header.Number, len(ch), err)
						return
					}
					c.logger.Debugf("Proposed block [%d] to raft consensus", b.Header.Number)

				case <-ctx.Done():
					c.logger.Debugf("Quit proposing blocks, discarded %d blocks in the queue", len(ch))
					return
				}
			}
		}(ctx, ch)

		return ch, cancel
	}

	becomeFollower := func() {
		cancelProp()
		c.blockInflight = 0
		_ = c.support.BlockCutter().Cut()
		stopTimer()
		submitC = c.submitC
		bc = nil
		c.Metrics.IsLeader.Set(0)
	}

	for {
		select {
		case s := <-submitC:
			if s == nil {
				// polled by `WaitReady`
				continue
			}

			if soft.RaftState == raft.StatePreCandidate || soft.RaftState == raft.StateCandidate {
				s.leader <- raft.None
				continue
			}

			s.leader <- soft.Lead
			if soft.Lead != c.raftID {
				continue
			}

			batches, pending, err := c.ordered(s.req)
			if err != nil {
				c.logger.Errorf("Failed to order message: %s", err)
				continue
			}
			if pending {
				startTimer() // no-op if timer is already started
			} else {
				stopTimer()
			}

			c.propose(propC, bc, batches...)

			if c.configInflight {
				c.logger.Info("Received config transaction, pause accepting transaction till it is committed")
				submitC = nil
			} else if c.blockInflight >= c.opts.MaxInflightBlocks {
				c.logger.Debugf("Number of in-flight blocks (%d) reaches limit (%d), pause accepting transaction",
					c.blockInflight, c.opts.MaxInflightBlocks)
				submitC = nil
			}

		case app := <-c.applyC:
			if app.soft != nil {
				newLeader := atomic.LoadUint64(&app.soft.Lead) // etcdraft requires atomic access
				if newLeader != soft.Lead {
					c.logger.Infof("Raft leader changed: %d -> %d", soft.Lead, newLeader)
					c.Metrics.LeaderChanges.Add(1)

					atomic.StoreUint64(&c.lastKnownLeader, newLeader)

					if newLeader == c.raftID {
						propC, cancelProp = becomeLeader()
					}

					if soft.Lead == c.raftID {
						becomeFollower()
					}
				}

				foundLeader := soft.Lead == raft.None && newLeader != raft.None
				quitCandidate := isCandidate(soft.RaftState) && !isCandidate(app.soft.RaftState)

				if foundLeader || quitCandidate {
					c.errorCLock.Lock()
					c.errorC = make(chan struct{})
					c.errorCLock.Unlock()
				}

				if isCandidate(app.soft.RaftState) || newLeader == raft.None {
					atomic.StoreUint64(&c.lastKnownLeader, raft.None)
					select {
					case <-c.errorC:
					default:
						nodeCount := len(c.opts.BlockMetadata.ConsenterIds)
						// Only close the error channel (to signal the broadcast/deliver front-end a consensus backend error)
						// If we are a cluster of size 3 or more, otherwise we can't expand a cluster of size 1 to 2 nodes.
						if nodeCount > 2 {
							close(c.errorC)
						} else {
							c.logger.Warningf("No leader is present, cluster size is %d", nodeCount)
						}
					}
				}

				soft = raft.SoftState{Lead: newLeader, RaftState: app.soft.RaftState}

				// notify external observer
				select {
				case c.observeC <- soft:
				default:
				}
			}

			c.apply(app.entries)

			if c.justElected {
				msgInflight := c.Node.lastIndex() > c.appliedIndex
				if msgInflight {
					c.logger.Debugf("There are in flight blocks, new leader should not serve requests")
					continue
				}

				if c.configInflight {
					c.logger.Debugf("There is config block in flight, new leader should not serve requests")
					continue
				}

				c.logger.Infof("Start accepting requests as Raft leader at block [%d]", c.lastBlock.Header.Number)
				bc = &blockCreator{
					hash:   c.lastBlock.Header.Hash(),
					number: c.lastBlock.Header.Number,
					logger: c.logger,
				}
				submitC = c.submitC
				c.justElected = false
			} else if c.configInflight {
				c.logger.Info("Config block or ConfChange in flight, pause accepting transaction")
				submitC = nil
			} else if c.blockInflight < c.opts.MaxInflightBlocks {
				submitC = c.submitC
			}

		case <-timer.C():
			ticking = false

			batch := c.support.BlockCutter().Cut()
			if len(batch) == 0 {
				c.logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}

			c.logger.Debugf("Batch timer expired, creating block")
			c.propose(propC, bc, batch) // we are certain this is normal block, no need to block

		case sn := <-c.snapC:
			if sn.Metadata.Index != 0 {
				if sn.Metadata.Index <= c.appliedIndex {
					c.logger.Debugf("Skip snapshot taken at index %d, because it is behind current applied index %d", sn.Metadata.Index, c.appliedIndex)
					break
				}

				c.confState = sn.Metadata.ConfState
				c.appliedIndex = sn.Metadata.Index
			} else {
				c.logger.Infof("Received artificial snapshot to trigger catchup")
			}

			if err := c.catchUp(sn); err != nil {
				c.logger.Panicf("Failed to recover from snapshot taken at Term %d and Index %d: %s",
					sn.Metadata.Term, sn.Metadata.Index, err)
			}

		case <-c.doneC:
			cancelProp()

			select {
			case <-c.errorC: // avoid closing closed channel
			default:
				close(c.errorC)
			}

			c.logger.Infof("Stop serving requests")
			c.periodicChecker.Stop()
			return
		}
	}
}

func (c *Chain) writeBlock(block *common.Block, index uint64) {
	if block.Header.Number > c.lastBlock.Header.Number+1 {
		c.logger.Panicf("Got block [%d], expect block [%d]", block.Header.Number, c.lastBlock.Header.Number+1)
	} else if block.Header.Number < c.lastBlock.Header.Number+1 {
		c.logger.Infof("Got block [%d], expect block [%d], this node was forced to catch up", block.Header.Number, c.lastBlock.Header.Number+1)
		return
	}

	if c.blockInflight > 0 {
		c.blockInflight-- // only reduce on leader
	}
	c.lastBlock = block

	c.logger.Infof("Writing block [%d] (Raft index: %d) to ledger", block.Header.Number, index)

	if utils.IsConfigBlock(block) {
		c.writeConfigBlock(block, index)
		return
	}

	c.raftMetadataLock.Lock()
	c.opts.BlockMetadata.RaftIndex = index
	m := utils.MarshalOrPanic(c.opts.BlockMetadata)
	c.raftMetadataLock.Unlock()

	c.support.WriteBlock(block, m)
}

// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//   -- batches [][]*common.Envelope; the batches cut,
//   -- pending bool; if there are envelopes pending to be ordered,
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.
func (c *Chain) ordered(msg *orderer.SubmitRequest) (batches [][]*common.Envelope, pending bool, err error) {
	seq := c.support.Sequence()

	if c.isConfig(msg.Payload) {
		// ConfigMsg
		if msg.LastValidationSeq < seq {
			c.logger.Warnf("Config message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
			msg.Payload, _, err = c.support.ProcessConfigMsg(msg.Payload)
			if err != nil {
				c.Metrics.ProposalFailures.Add(1)
				return nil, true, errors.Errorf("bad config message: %s", err)
			}

			if err = c.checkConfigUpdateValidity(msg.Payload); err != nil {
				c.Metrics.ProposalFailures.Add(1)
				return nil, true, errors.Errorf("bad config message: %s", err)
			}
		}
		batch := c.support.BlockCutter().Cut()
		batches = [][]*common.Envelope{}
		if len(batch) != 0 {
			batches = append(batches, batch)
		}
		batches = append(batches, []*common.Envelope{msg.Payload})
		return batches, false, nil
	}
	// it is a normal message
	if msg.LastValidationSeq < seq {
		c.logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			c.Metrics.ProposalFailures.Add(1)
			return nil, true, errors.Errorf("bad normal message: %s", err)
		}
	}
	batches, pending = c.support.BlockCutter().Ordered(msg.Payload)
	return batches, pending, nil

}

func (c *Chain) propose(ch chan<- *common.Block, bc *blockCreator, batches ...[]*common.Envelope) {
	for _, batch := range batches {
		b := bc.createNextBlock(batch)
		c.logger.Infof("Created block [%d], there are %d blocks in flight", b.Header.Number, c.blockInflight)

		select {
		case ch <- b:
		default:
			c.logger.Panic("Programming error: limit of in-flight blocks does not properly take effect or block is proposed by follower")
		}

		// if it is config block, then we should wait for the commit of the block
		if utils.IsConfigBlock(b) {
			c.configInflight = true
		}

		c.blockInflight++
	}

	return
}

func (c *Chain) catchUp(snap *raftpb.Snapshot) error {
	b, err := utils.UnmarshalBlock(snap.Data)
	if err != nil {
		return errors.Errorf("failed to unmarshal snapshot data to block: %s", err)
	}

	if c.lastBlock.Header.Number >= b.Header.Number {
		c.logger.Warnf("Snapshot is at block [%d], local block number is %d, no sync needed", b.Header.Number, c.lastBlock.Header.Number)
		return nil
	}

	puller, err := c.createPuller()
	if err != nil {
		return errors.Errorf("failed to create block puller: %s", err)
	}
	defer puller.Close()

	next := c.lastBlock.Header.Number + 1

	c.logger.Infof("Catching up with snapshot taken at block [%d], starting from block [%d]", b.Header.Number, next)

	for next <= b.Header.Number {
		block := puller.PullBlock(next)
		if block == nil {
			return errors.Errorf("failed to fetch block [%d] from cluster", next)
		}
		if utils.IsConfigBlock(block) {
			c.support.WriteConfigBlock(block, nil)

			configMembership := c.detectConfChange(block)

			if configMembership != nil && configMembership.Changed() {
				c.logger.Infof("Config block [%d] changes consenter set, communication should be reconfigured", block.Header.Number)

				c.raftMetadataLock.Lock()
				c.opts.BlockMetadata = configMembership.NewBlockMetadata
				c.opts.Consenters = configMembership.NewConsenters
				c.raftMetadataLock.Unlock()

				if err := c.configureComm(); err != nil {
					c.logger.Panicf("Failed to configure communication: %s", err)
				}
			}
		} else {
			c.support.WriteBlock(block, nil)
		}

		c.lastBlock = block
		next++
	}

	c.logger.Infof("Finished syncing with cluster up to and including block [%d]", b.Header.Number)
	return nil
}

func (c *Chain) detectConfChange(block *common.Block) *MembershipChanges {
	// If config is targeting THIS channel, inspect consenter set and
	// propose raft ConfChange if it adds/removes node.
	configMetadata := c.newConfigMetadata(block)

	if configMetadata == nil {
		return nil
	}

	if configMetadata.Options != nil &&
		configMetadata.Options.SnapshotIntervalSize != 0 &&
		configMetadata.Options.SnapshotIntervalSize != c.sizeLimit {
		c.logger.Infof("Update snapshot interval size to %d bytes (was %d)",
			configMetadata.Options.SnapshotIntervalSize, c.sizeLimit)
		c.sizeLimit = configMetadata.Options.SnapshotIntervalSize
	}

	changes, err := ComputeMembershipChanges(c.opts.BlockMetadata, c.opts.Consenters, configMetadata.Consenters)
	if err != nil {
		c.logger.Panicf("illegal configuration change detected: %s", err)
	}

	if changes.Rotated() {
		c.logger.Infof("Config block [%d] rotates TLS certificate of node %d", block.Header.Number, changes.RotatedNode)
	}

	return changes
}

func (c *Chain) apply(ents []raftpb.Entry) {
	if len(ents) == 0 {
		return
	}

	if ents[0].Index > c.appliedIndex+1 {
		c.logger.Panicf("first index of committed entry[%d] should <= appliedIndex[%d]+1", ents[0].Index, c.appliedIndex)
	}

	var position int
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				break
			}

			position = i
			c.accDataSize += uint32(len(ents[i].Data))

			// We need to strictly avoid re-applying normal entries,
			// otherwise we are writing the same block twice.
			if ents[i].Index <= c.appliedIndex {
				c.logger.Debugf("Received block with raft index (%d) <= applied index (%d), skip", ents[i].Index, c.appliedIndex)
				break
			}

			block := utils.UnmarshalBlockOrPanic(ents[i].Data)
			c.writeBlock(block, ents[i].Index)
			c.Metrics.CommittedBlockNumber.Set(float64(block.Header.Number))

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				c.logger.Warnf("Failed to unmarshal ConfChange data: %s", err)
				continue
			}

			c.confState = *c.Node.ApplyConfChange(cc)

			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				c.logger.Infof("Applied config change to add node %d, current nodes in channel: %+v", cc.NodeID, c.confState.Nodes)
			case raftpb.ConfChangeRemoveNode:
				c.logger.Infof("Applied config change to remove node %d, current nodes in channel: %+v", cc.NodeID, c.confState.Nodes)
			default:
				c.logger.Panic("Programming error, encountered unsupported raft config change")
			}

			// This ConfChange was introduced by a previously committed config block,
			// we can now unblock submitC to accept envelopes.
			if c.confChangeInProgress != nil &&
				c.confChangeInProgress.NodeID == cc.NodeID &&
				c.confChangeInProgress.Type == cc.Type {

				if err := c.configureComm(); err != nil {
					c.logger.Panicf("Failed to configure communication: %s", err)
				}

				c.confChangeInProgress = nil
				c.configInflight = false
				// report the new cluster size
				c.Metrics.ClusterSize.Set(float64(len(c.opts.BlockMetadata.ConsenterIds)))
			}

			if cc.Type == raftpb.ConfChangeRemoveNode && cc.NodeID == c.raftID {
				c.logger.Infof("Current node removed from replica set for channel %s", c.channelID)
				// calling goroutine, since otherwise it will be blocked
				// trying to write into haltC
				lead := atomic.LoadUint64(&c.lastKnownLeader)
				if lead == c.raftID {
					c.logger.Info("This node is being removed as current leader, halt with delay")
					c.configInflight = true // toggle the flag so this node does not accept further tx
					go func() {
						select {
						case <-c.clock.After(time.Duration(c.opts.ElectionTick) * c.opts.TickInterval):
						case <-c.doneC:
						}

						c.Halt()
					}()
				} else {
					go c.Halt()
				}
			}
		}

		if ents[i].Index > c.appliedIndex {
			c.appliedIndex = ents[i].Index
		}
	}

	if c.accDataSize >= c.sizeLimit {
		b := utils.UnmarshalBlockOrPanic(ents[position].Data)

		select {
		case c.gcC <- &gc{index: c.appliedIndex, state: c.confState, data: ents[position].Data}:
			c.logger.Infof("Accumulated %d bytes since last snapshot, exceeding size limit (%d bytes), "+
				"taking snapshot at block [%d] (index: %d), last snapshotted block number is %d, current nodes: %+v",
				c.accDataSize, c.sizeLimit, b.Header.Number, c.appliedIndex, c.lastSnapBlockNum, c.confState.Nodes)
			c.accDataSize = 0
			c.lastSnapBlockNum = b.Header.Number
			c.Metrics.SnapshotBlockNumber.Set(float64(b.Header.Number))
		default:
			c.logger.Warnf("Snapshotting is in progress, it is very likely that SnapshotIntervalSize is too small")
		}
	}

	return
}

func (c *Chain) gc() {
	for {
		select {
		case g := <-c.gcC:
			c.Node.takeSnapshot(g.index, g.state, g.data)
		case <-c.doneC:
			c.logger.Infof("Stop garbage collecting")
			return
		}
	}
}

func (c *Chain) isConfig(env *common.Envelope) bool {
	h, err := utils.ChannelHeader(env)
	if err != nil {
		c.logger.Panicf("failed to extract channel header from envelope")
	}

	return h.Type == int32(common.HeaderType_CONFIG) || h.Type == int32(common.HeaderType_ORDERER_TRANSACTION)
}

func (c *Chain) configureComm() error {
	// Reset unreachable map when communication is reconfigured
	c.Node.unreachableLock.Lock()
	c.Node.unreachable = make(map[uint64]struct{})
	c.Node.unreachableLock.Unlock()

	nodes, err := c.remotePeers()
	if err != nil {
		return err
	}

	c.configurator.Configure(c.channelID, nodes)
	return nil
}

func (c *Chain) remotePeers() ([]cluster.RemoteNode, error) {
	c.raftMetadataLock.RLock()
	defer c.raftMetadataLock.RUnlock()

	var nodes []cluster.RemoteNode
	for raftID, consenter := range c.opts.Consenters {
		// No need to know yourself
		if raftID == c.raftID {
			continue
		}
		serverCertAsDER, err := pemToDER(consenter.ServerTlsCert, raftID, "server", c.logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := pemToDER(consenter.ClientTlsCert, raftID, "client", c.logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		nodes = append(nodes, cluster.RemoteNode{
			ID:            raftID,
			Endpoint:      fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
			ServerTLSCert: serverCertAsDER,
			ClientTLSCert: clientCertAsDER,
		})
	}
	return nodes, nil
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// writeConfigBlock writes configuration blocks into the ledger in
// addition extracts updates about raft replica set and if there
// are changes updates cluster membership as well
func (c *Chain) writeConfigBlock(block *common.Block, index uint64) {
	hdr, err := ConfigChannelHeader(block)
	if err != nil {
		c.logger.Panicf("Failed to get config header type from config block: %s", err)
	}

	c.configInflight = false

	switch common.HeaderType(hdr.Type) {
	case common.HeaderType_CONFIG:
		configMembership := c.detectConfChange(block)

		c.raftMetadataLock.Lock()
		c.opts.BlockMetadata.RaftIndex = index
		if configMembership != nil {
			c.opts.BlockMetadata = configMembership.NewBlockMetadata
			c.opts.Consenters = configMembership.NewConsenters
		}
		c.raftMetadataLock.Unlock()

		blockMetadataBytes := utils.MarshalOrPanic(c.opts.BlockMetadata)
		// write block with metadata
		c.support.WriteConfigBlock(block, blockMetadataBytes)

		if configMembership == nil {
			return
		}

		// update membership
		if configMembership.ConfChange != nil {
			// We need to propose conf change in a go routine, because it may be blocked if raft node
			// becomes leaderless, and we should not block `serveRequest` so it can keep consuming applyC,
			// otherwise we have a deadlock.
			go func() {
				// ProposeConfChange returns error only if node being stopped.
				// This proposal is dropped by followers because DisableProposalForwarding is enabled.
				if err := c.Node.ProposeConfChange(context.TODO(), *configMembership.ConfChange); err != nil {
					c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
				}
			}()

			c.confChangeInProgress = configMembership.ConfChange

			switch configMembership.ConfChange.Type {
			case raftpb.ConfChangeAddNode:
				c.logger.Infof("Config block just committed adds node %d, pause accepting transactions till config change is applied", configMembership.ConfChange.NodeID)
			case raftpb.ConfChangeRemoveNode:
				c.logger.Infof("Config block just committed removes node %d, pause accepting transactions till config change is applied", configMembership.ConfChange.NodeID)
			default:
				c.logger.Panic("Programming error, encountered unsupported raft config change")
			}

			c.configInflight = true
		} else if configMembership.Rotated() {
			lead := atomic.LoadUint64(&c.lastKnownLeader)
			if configMembership.RotatedNode == lead {
				c.logger.Infof("Certificate of Raft leader is being rotated, attempt leader transfer before reconfiguring communication")
				go func() {
					c.Node.abdicateLeader(lead)
					if err := c.configureComm(); err != nil {
						c.logger.Panicf("Failed to configure communication: %s", err)
					}
				}()
			} else {
				if err := c.configureComm(); err != nil {
					c.logger.Panicf("Failed to configure communication: %s", err)
				}
			}
		}

	case common.HeaderType_ORDERER_TRANSACTION:
		// If this config is channel creation, no extra inspection is needed
		c.raftMetadataLock.Lock()
		c.opts.BlockMetadata.RaftIndex = index
		m := utils.MarshalOrPanic(c.opts.BlockMetadata)
		c.raftMetadataLock.Unlock()

		c.support.WriteConfigBlock(block, m)

	default:
		c.logger.Panicf("Programming error: unexpected config type: %s", common.HeaderType(hdr.Type))
	}
}

// getInFlightConfChange returns ConfChange in-flight if any.
// It returns confChangeInProgress if it is not nil. Otherwise
// it returns ConfChange from the last committed block (might be nil).
func (c *Chain) getInFlightConfChange() *raftpb.ConfChange {
	if c.confChangeInProgress != nil {
		return c.confChangeInProgress
	}

	if c.lastBlock.Header.Number == 0 {
		return nil // nothing to failover just started the chain
	}

	if !utils.IsConfigBlock(c.lastBlock) {
		return nil
	}

	// extracting current Raft configuration state
	confState := c.Node.ApplyConfChange(raftpb.ConfChange{})

	if len(confState.Nodes) == len(c.opts.BlockMetadata.ConsenterIds) {
		// Raft configuration change could only add one node or
		// remove one node at a time, if raft conf state size is
		// equal to membership stored in block metadata field,
		// that means everything is in sync and no need to propose
		// config update.
		return nil
	}

	return ConfChange(c.opts.BlockMetadata, confState)
}

// newMetadata extract config metadata from the configuration block
func (c *Chain) newConfigMetadata(block *common.Block) *etcdraft.ConfigMetadata {
	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		c.logger.Panicf("error reading consensus metadata: %s", err)
	}
	return metadata
}

func (c *Chain) suspectEviction() bool {
	if c.isRunning() != nil {
		return false
	}

	return atomic.LoadUint64(&c.lastKnownLeader) == uint64(0)
}

func (c *Chain) newEvictionSuspector() *evictionSuspector {
	return &evictionSuspector{
		amIInChannel:               ConsenterCertificate(c.opts.Cert).IsConsenterOfChannel,
		evictionSuspicionThreshold: c.opts.EvictionSuspicion,
		writeBlock:                 c.support.Append,
		createPuller:               c.createPuller,
		height:                     c.support.Height,
		triggerCatchUp:             c.triggerCatchup,
		logger:                     c.logger,
		halt: func() {
			c.Halt()
		},
	}
}

func (c *Chain) triggerCatchup(sn *raftpb.Snapshot) {
	select {
	case c.snapC <- sn:
	case <-c.doneC:
	}
}
