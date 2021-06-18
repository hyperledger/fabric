/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"context"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fly2plan/fabric-protos-go/orderer/hlmirbft"
	"github.com/hyperledger-labs/mirbft"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	pb "github.com/hyperledger-labs/mirbft/pkg/pb/msgs"

	"code.cloudfoundry.org/clock"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
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
	DefaultSnapshotCatchUpEntries = uint64(4)

	// DefaultSnapshotIntervalSize is the default snapshot interval. It is
	// used if SnapshotIntervalSize is not provided in channel config options.
	// It is needed to enforce snapshot being set.
	DefaultSnapshotIntervalSize = 16 * MEGABYTE

	// DefaultEvictionSuspicion is the threshold that a node will start
	// suspecting its own eviction if it has been leaderless for this
	// period of time.
	DefaultEvictionSuspicion = time.Minute * 10

	// DefaultLeaderlessCheckInterval is the interval that a chain checks
	// its own leadership status.
	DefaultLeaderlessCheckInterval = time.Second * 10
	//FLY2-64 Proposed change
	ForwardFlag = "@forward/"
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
	MirBFTID uint64

	Clock clock.Clock

	WALDir               string
	SnapDir              string
	ReqStoreDir          string
	SnapshotIntervalSize uint32

	// This is configurable mainly for testing purpose. Users are not
	// expected to alter this. Instead, DefaultSnapshotCatchUpEntries is used.
	SnapshotCatchUpEntries uint64

	MemoryStorage MemoryStorage
	Logger        *flogging.FabricLogger

	HeartbeatTicks       uint32
	SuspectTicks         uint32
	NewEpochTimeoutTicks uint32
	BufferSize           uint32
	MaxSizePerMsg        uint64

	// BlockMetdata and Consenters should only be modified while under lock
	// of raftMetadataLock
	BlockMetadata *hlmirbft.BlockMetadata
	Consenters    map[uint64]*hlmirbft.Consenter

	// MigrationInit is set when the node starts right after consensus-type migration
	MigrationInit bool

	Metrics *Metrics
	Cert    []byte
}

//FLY2-64 - proposed changes
// - leader ID not required
type submit struct {
	req *orderer.SubmitRequest
	// leader chan uint64
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

	MirBFTID  uint64
	channelID string

	lastKnownLeader uint64
	ActiveNodes     atomic.Value

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

	mirbftMetadataLock   sync.RWMutex
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

	statusReportMutex sync.Mutex
	consensusRelation types.ConsensusRelation
	status            types.Status

	// BCCSP instance
	CryptoProvider bccsp.BCCSP
}

type MirBFTLogger struct {
	*flogging.FabricLogger
}

func (ml *MirBFTLogger) Log(level mirbft.LogLevel, text string, args ...interface{}) {
	switch level {
	case mirbft.LevelDebug:
		ml.Debugf(text, args...)
	case mirbft.LevelError:
		ml.Errorf(text, args...)
	case mirbft.LevelInfo:
		ml.Infof(text, args...)
	case mirbft.LevelWarn:
		ml.Warnf(text, args...)
	}
}

// NewChain constructs a chain object.
func NewChain(
	support consensus.ConsenterSupport,
	opts Options,
	conf Configurator,
	rpc RPC,
	cryptoProvider bccsp.BCCSP,
	f CreateBlockPuller,
	haltCallback func(),
	observeC chan<- raft.SoftState,
) (*Chain, error) {
	lg := opts.Logger.With("channel", support.ChannelID(), "node", opts.MirBFTID)

	fresh := !wal.Exist(opts.WALDir)
	/*	//storage, err := CreateStorage(lg, opts.WALDir, opts.SnapDir, opts.MemoryStorage)
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
			b := protoutil.UnmarshalBlockOrPanic(s.Data)
			snapBlkNum = b.Header.Number
			cc = s.Metadata.ConfState
		}*/

	b := support.Block(support.Height() - 1)
	if b == nil {
		return nil, errors.Errorf("failed to get last block")
	}

	c := &Chain{
		configurator:      conf,
		rpc:               rpc,
		channelID:         support.ChannelID(),
		MirBFTID:          opts.MirBFTID,
		submitC:           make(chan *submit),
		applyC:            make(chan apply),
		haltC:             make(chan struct{}),
		doneC:             make(chan struct{}),
		startC:            make(chan struct{}),
		snapC:             make(chan *raftpb.Snapshot),
		errorC:            make(chan struct{}),
		gcC:               make(chan *gc),
		observeC:          observeC,
		support:           support,
		fresh:             fresh,
		appliedIndex:      opts.BlockMetadata.RaftIndex,
		lastBlock:         b,
		createPuller:      f,
		clock:             opts.Clock,
		haltCallback:      haltCallback,
		consensusRelation: types.ConsensusRelationConsenter,
		status:            types.StatusActive,
		Metrics: &Metrics{
			ClusterSize:             opts.Metrics.ClusterSize.With("channel", support.ChannelID()),
			IsLeader:                opts.Metrics.IsLeader.With("channel", support.ChannelID()),
			ActiveNodes:             opts.Metrics.ActiveNodes.With("channel", support.ChannelID()),
			CommittedBlockNumber:    opts.Metrics.CommittedBlockNumber.With("channel", support.ChannelID()),
			SnapshotBlockNumber:     opts.Metrics.SnapshotBlockNumber.With("channel", support.ChannelID()),
			LeaderChanges:           opts.Metrics.LeaderChanges.With("channel", support.ChannelID()),
			ProposalFailures:        opts.Metrics.ProposalFailures.With("channel", support.ChannelID()),
			DataPersistDuration:     opts.Metrics.DataPersistDuration.With("channel", support.ChannelID()),
			NormalProposalsReceived: opts.Metrics.NormalProposalsReceived.With("channel", support.ChannelID()),
			ConfigProposalsReceived: opts.Metrics.ConfigProposalsReceived.With("channel", support.ChannelID()),
		},
		logger:         lg,
		opts:           opts,
		CryptoProvider: cryptoProvider,
	}

	// Sets initial values for metrics
	c.Metrics.ClusterSize.Set(float64(len(c.opts.BlockMetadata.ConsenterIds)))
	c.Metrics.IsLeader.Set(float64(0)) // all nodes start out as followers
	c.Metrics.ActiveNodes.Set(float64(0))
	c.Metrics.CommittedBlockNumber.Set(float64(c.lastBlock.Header.Number))
	c.Metrics.SnapshotBlockNumber.Set(float64(c.lastSnapBlockNum))

	// DO NOT use Applied option in config, see https://github.com/etcd-io/etcd/issues/10217
	// We guard against replay of written blocks with `appliedIndex` instead.

	config := &mirbft.Config{
		Logger:               &MirBFTLogger{c.logger},
		BatchSize:            support.SharedConfig().BatchSize().MaxMessageCount,
		HeartbeatTicks:       opts.HeartbeatTicks,
		SuspectTicks:         opts.SuspectTicks,
		NewEpochTimeoutTicks: opts.NewEpochTimeoutTicks,
		BufferSize:           opts.BufferSize,
	}

	disseminator := &Disseminator{RPC: c.rpc}
	disseminator.UpdateMetadata(nil) // initialize
	c.ActiveNodes.Store([]uint64{})

	c.Node = &node{
		chainID:     c.channelID,
		chain:       c,
		logger:      c.logger,
		metrics:     c.Metrics,
		rpc:         disseminator,
		config:      config,
		WALDir:      opts.WALDir,
		ReqStoreDir: opts.ReqStoreDir,
		clock:       c.clock,
		metadata:    c.opts.BlockMetadata,
	}

	return c, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting MirBFT node")

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	isJoin := c.support.Height() > 1
	if isJoin && c.opts.MigrationInit {
		isJoin = false
		c.logger.Infof("Consensus-type migration detected, starting new mirbft node on an existing channel; height=%d", c.support.Height())
	}
	c.Node.start(c.fresh, isJoin)

	close(c.startC)
	close(c.errorC)

	go c.gc()
	go c.run()

	/*	es := c.newEvictionSuspector()

		interval := DefaultLeaderlessCheckInterval
		if c.opts.LeaderCheckInterval != 0 {
			interval = c.opts.LeaderCheckInterval
		}

		c.periodicChecker = &PeriodicCheck{
			Logger:        c.logger,
			Report:        es.confirmSuspicion,
			ReportCleared: es.clearSuspicion,
			CheckInterval: interval,
			Condition:     c.suspectEviction,
		}
		c.periodicChecker.Run()*/
}

// Order submits normal type transactions for ordering.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	c.Metrics.NormalProposalsReceived.Add(1)
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
}

// Configure submits config type transactions for ordering.
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	c.Metrics.ConfigProposalsReceived.Add(1)
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, 0)
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
	c.stop()
}

func (c *Chain) stop() bool {
	select {
	case <-c.startC:
	default:
		c.logger.Warn("Attempted to halt a chain that has not started")
		return false
	}

	select {
	case c.haltC <- struct{}{}:
	case <-c.doneC:
		return false
	}
	<-c.doneC

	c.statusReportMutex.Lock()
	defer c.statusReportMutex.Unlock()
	c.status = types.StatusInactive

	return true
}

// halt stops the chain and calls the haltCallback function, which allows the
// chain to transfer responsibility to a follower or the inactive chain registry when a chain
// discovers it is no longer a member of a channel.
func (c *Chain) halt() {
	if stopped := c.stop(); !stopped {
		c.logger.Info("This node was stopped, the haltCallback will not be called")
		return
	}
	if c.haltCallback != nil {
		c.haltCallback() // Must be invoked WITHOUT any internal lock

		c.statusReportMutex.Lock()
		defer c.statusReportMutex.Unlock()

		// If the haltCallback registers the chain in to the inactive chain registry (i.e., system channel exists) then
		// this is the correct consensusRelation. If the haltCallback transfers responsibility to a follower.Chain, then
		// this chain is about to be GC anyway. The new follower.Chain replacing this one will report the correct
		// StatusReport.
		c.consensusRelation = types.ConsensusRelationConfigTracker
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

	stepMsg := &msgs.Msg{}
	if err := proto.Unmarshal(req.Payload, stepMsg); err != nil {
		return fmt.Errorf("failed to unmarshal StepRequest payload to Raft Message: %s", err)
	}

	if err := c.Node.Step(context.TODO(), sender, stepMsg); err != nil {
		return fmt.Errorf("failed to process Raft Step message: %s", err)
	}

	if len(req.Metadata) == 0 || atomic.LoadUint64(&c.lastKnownLeader) != sender { // ignore metadata from non-leader
		return nil
	}

	clusterMetadata := &etcdraft.ClusterMetadata{}
	if err := proto.Unmarshal(req.Metadata, clusterMetadata); err != nil {
		return errors.Errorf("failed to unmarshal ClusterMetadata: %s", err)
	}

	c.Metrics.ActiveNodes.Set(float64(len(clusterMetadata.ActiveNodes)))
	c.ActiveNodes.Store(clusterMetadata.ActiveNodes)

	return nil
}

//FLy2-64 - proposed changes
// - check for forward flag in payload
func checkForwardFlag(reqPayload []byte) bool {
	forwardBytes := reqPayload[:8]
	if string(forwardBytes) == ForwardFlag {
		return true
	}
	return false
}

//FLY2-64 - proposed changes
// - append forward bytes

func appendForwardFlag(reqPayload []byte) []byte {
	ForwardFlagByte := []byte(ForwardFlag)
	appenedReq := append([]byte{}, ForwardFlagByte...)
	appenedReq = append(appenedReq, reqPayload...)
	return appenedReq
}

// Submit forwards the incoming request to:
// - the local run goroutine if this is leader
// - the actual leader via the transport mechanism
// The call fails if there's no leader elected yet.
// TODO(harry_knight) no longer single leader in case of hlmirbft. Send to bucket/s which is watched by a leader?
func (c *Chain) Submit(req *orderer.SubmitRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		c.Metrics.ProposalFailures.Add(1)
		return err
	}
	submitPayload := req.Payload.GetPayload()
	isfowarded := checkForwardFlag(submitPayload)
	if isfowarded {
		req.Payload.Payload = submitPayload[8:]
	}
	//leadC := make(chan uint64, 1)
	select {
	// case c.submitC <- &submit{req, leadC}:
	//FLY2-64 Proposed change
	//broadcast message to all nodes
	case c.submitC <- &submit{req}:
		//FLY2-57 proposed change
		// - marshal submit request and pass it to sendConsensus as payload
		if !isfowarded {
			appenedPayload := appendForwardFlag(submitPayload)
			req.Payload.Payload = appenedPayload
			for nodeID, _ := range c.opts.Consenters {
				if nodeID != c.MirBFTID {
					err := c.Node.rpc.SendSubmit(nodeID, req)
					if err != nil {
						c.logger.Warnf("Failed to broadcast Message to Node : %d ", nodeID)
						return err
					}
				}

			}

		}

		// lead := <-leadC
		// if lead == raft.None {
		// 	c.Metrics.ProposalFailures.Add(1)
		// 	return errors.Errorf("no Raft leader")
		// }

		// if lead != c.MirBFTID {

		// 	if err := c.rpc.SendSubmit(lead, req); err != nil {
		// 		c.Metrics.ProposalFailures.Add(1)
		// 		return err
		// 	}
		// }
		//broadcast message

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

func (c *Chain) run() {
	// ticking := false
	// timer := c.clock.NewTimer(time.Second)
	// // we need a stopped timer rather than nil,
	// // because we will be select waiting on timer.C()
	// if !timer.Stop() {
	// 	<-timer.C()
	// }
	// // if timer is already started, this is a no-op
	// startTimer := func() {
	// 	if !ticking {
	// 		ticking = true
	// 		timer.Reset(c.support.SharedConfig().BatchTimeout())
	// 	}
	// }
	// stopTimer := func() {
	// 	if !timer.Stop() && ticking {
	// 		// we only need to drain the channel if the timer expired (not explicitly stopped)
	// 		<-timer.C()
	// 	}
	// 	ticking = false
	// }
	// var soft raft.SoftState
	submitC := c.submitC
	// var bc *blockCreator
	// var propC chan<- *common.Block
	// var cancelProp context.CancelFunc
	// cancelProp = func() {} // no-op as initial value
	// // TODO(harry_knight) Intrinsic to raft? If so is it safe to remove?
	// // 	May reuse this pattern for hlmirbft.
	// becomeLeader := func() (chan<- *common.Block, context.CancelFunc) {
	// 	c.Metrics.IsLeader.Set(1)
	// 	c.blockInflight = 0
	// 	c.justElected = true
	// 	submitC = nil
	// 	ch := make(chan *common.Block, c.opts.MaxInflightBlocks)
	// 	// if there is unfinished ConfChange, we should resume the effort to propose it as
	// 	// new leader, and wait for it to be committed before start serving new requests.
	// 	if cc := c.getInFlightConfChange(); cc != nil {
	// 		// The reason `ProposeConfChange` should be called in go routine is documented in `writeConfigBlock` method.
	// 		go func() {
	// 			if err := c.Node.ProposeConfChange(context.TODO(), *cc); err != nil {
	// 				c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
	// 			}
	// 		}()
	// 		c.confChangeInProgress = cc
	// 		c.configInflight = true
	// 	}
	// 	// Leader should call Propose in go routine, because this method may be blocked
	// 	// if node is leaderless (this can happen when leader steps down in a heavily
	// 	// loaded network). We need to make sure applyC can still be consumed properly.
	// 	ctx, cancel := context.WithCancel(context.Background())
	// 	go func(ctx context.Context, ch <-chan *common.Block) {
	// 		for {
	// 			select {
	// 			case b := <-ch:
	// 				data := protoutil.MarshalOrPanic(b)
	// 				if err := c.Node.Propose(ctx, data); err != nil {
	// 					c.logger.Errorf("Failed to propose block [%d] to raft and discard %d blocks in queue: %s", b.Header.Number, len(ch), err)
	// 					return
	// 				}
	// 				c.logger.Debugf("Proposed block [%d] to raft consensus", b.Header.Number)
	// 			case <-ctx.Done():
	// 				c.logger.Debugf("Quit proposing blocks, discarded %d blocks in the queue", len(ch))
	// 				return
	// 			}
	// 		}
	// 	}(ctx, ch)
	// 	return ch, cancel
	// }
	// // TODO(harry_knight) Also intrinsic to raft but may be reusable.
	// // 	In the case of hlmirbft a follower is a replica that isn't a leader.
	// becomeFollower := func() {
	// 	cancelProp()
	// 	c.blockInflight = 0
	// 	_ = c.support.BlockCutter().Cut()
	// 	stopTimer()
	// 	submitC = c.submitC
	// 	bc = nil
	// 	c.Metrics.IsLeader.Set(0)
	// }
	for {
		// 	// TODO(harry_knight) Infinite loop which manages chain. Will be adapted for hlmirbft.
		select {
		// 	// TODO(harry_knight) Submit channel takes transactions which are to be batched (into a block).
		case s := <-submitC:
			if s == nil {
				// polled by `WaitReady`
				continue
			}
			// 		if soft.RaftState == raft.StatePreCandidate || soft.RaftState == raft.StateCandidate {
			// 			s.leader <- raft.None
			// 			continue
			// 		}
			// 		s.leader <- soft.Lead
			// 		// TODO(harry_knight) If not the leader then continue. Keep submit channel and method?
			// 		if soft.Lead != c.raftID {
			// 			continue
			// 		}
			// 		// TODO(harry_knight) Check if the method, ordered, is independent of raft
			// 		// 	Tentative answer: Only dependent on config sequence which is dependent on configtx.
			// 		batches, pending, err := c.ordered(s.req)

			//FLY2-64 - proposed changes
			// passing request to ordered function
			err := c.ordered(s.req)
			if err != nil {
				c.logger.Errorf("Failed to order message: %s", err)
				continue
			}
			// 		if err != nil {
			// 			c.logger.Errorf("Failed to order message: %s", err)
			// 			continue
			// 		}
			// 		if pending {
			// 			startTimer() // no-op if timer is already started
			// 		} else {
			// 			stopTimer()
			// 		}
			// 		// TODO(harry_knight) Likewise for propose
			// 		c.propose(propC, bc, batches...)
			// if c.configInflight {
			// 	c.logger.Info("Received config transaction, pause accepting transaction till it is committed")
			// 	submitC = nil
			// } else if c.blockInflight >= c.opts.MaxInflightBlocks {
			// 	c.logger.Debugf("Number of in-flight blocks (%d) reaches limit (%d), pause accepting transaction",
			// 		c.blockInflight, c.opts.MaxInflightBlocks)
			// 	submitC = nil
			// }
			// 	// TODO(harry_knight) c.applyC is tied to raft FSM. Remove?
			// 	// 	Tentative answer: applyC executes actions which modify the chain e.g. adding a block.
			// 	// 	So channel must be retained for hlmirbft
			// 	case app := <-c.applyC:
			// 		if app.soft != nil {
			// 			newLeader := atomic.LoadUint64(&app.soft.Lead) // etcdraft requires atomic access
			// 			if newLeader != soft.Lead {
			// 				c.logger.Infof("Raft leader changed: %d -> %d", soft.Lead, newLeader)
			// 				c.Metrics.LeaderChanges.Add(1)
			// 				atomic.StoreUint64(&c.lastKnownLeader, newLeader)
			// 				if newLeader == c.raftID {
			// 					propC, cancelProp = becomeLeader()
			// 				}
			// 				if soft.Lead == c.raftID {
			// 					becomeFollower()
			// 				}
			// 			}
			// 			foundLeader := soft.Lead == raft.None && newLeader != raft.None
			// 			quitCandidate := isCandidate(soft.RaftState) && !isCandidate(app.soft.RaftState)
			// 			if foundLeader || quitCandidate {
			// 				c.errorCLock.Lock()
			// 				c.errorC = make(chan struct{})
			// 				c.errorCLock.Unlock()
			// 			}
			// 			if isCandidate(app.soft.RaftState) || newLeader == raft.None {
			// 				atomic.StoreUint64(&c.lastKnownLeader, raft.None)
			// 				select {
			// 				case <-c.errorC:
			// 				default:
			// 					nodeCount := len(c.opts.BlockMetadata.ConsenterIds)
			// 					// Only close the error channel (to signal the broadcast/deliver front-end a consensus backend error)
			// 					// If we are a cluster of size 3 or more, otherwise we can't expand a cluster of size 1 to 2 nodes.
			// 					if nodeCount > 2 {
			// 						close(c.errorC)
			// 					} else {
			// 						c.logger.Warningf("No leader is present, cluster size is %d", nodeCount)
			// 					}
			// 				}
			// 			}
			// 			soft = raft.SoftState{Lead: newLeader, RaftState: app.soft.RaftState}
			// 			// notify external observer
			// 			select {
			// 			case c.observeC <- soft:
			// 			default:
			// 			}
			// 		}
			// 		// TODO(harry_knight) Adapt for hlmirbft.
			// 		c.apply(app.entries)
			// 		if c.justElected {
			// 			msgInflight := c.Node.lastIndex() > c.appliedIndex
			// 			if msgInflight {
			// 				c.logger.Debugf("There are in flight blocks, new leader should not serve requests")
			// 				continue
			// 			}
			// 			if c.configInflight {
			// 				c.logger.Debugf("There is config block in flight, new leader should not serve requests")
			// 				continue
			// 			}
			// 			c.logger.Infof("Start accepting requests as Raft leader at block [%d]", c.lastBlock.Header.Number)
			// 			bc = &blockCreator{
			// 				hash:   protoutil.BlockHeaderHash(c.lastBlock.Header),
			// 				number: c.lastBlock.Header.Number,
			// 				logger: c.logger,
			// 			}
			// 			submitC = c.submitC
			// 			c.justElected = false
			// 		} else if c.configInflight {
			// 			c.logger.Info("Config block or ConfChange in flight, pause accepting transaction")
			// 			submitC = nil
			// 		} else if c.blockInflight < c.opts.MaxInflightBlocks {
			// 			submitC = c.submitC
			// 		}
			// 	case <-timer.C():
			// 		ticking = false
			// 		batch := c.support.BlockCutter().Cut()
			// 		if len(batch) == 0 {
			// 			c.logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
			// 			continue
			// 		}
			// 		c.logger.Debugf("Batch timer expired, creating block")
			// 		c.propose(propC, bc, batch) // we are certain this is normal block, no need to block
			// 	// TODO(harry_knight) snapshot is associated with raft FSM. Remove?
			// 	// 	Tentative answer: hlmirbft has snapshot functionality so adapt instead
			// 	case sn := <-c.snapC:
			// 		if sn.Metadata.Index != 0 {
			// 			if sn.Metadata.Index <= c.appliedIndex {
			// 				c.logger.Debugf("Skip snapshot taken at index %d, because it is behind current applied index %d", sn.Metadata.Index, c.appliedIndex)
			// 				break
			// 			}
			// 			c.confState = sn.Metadata.ConfState
			// 			c.appliedIndex = sn.Metadata.Index
			// 		} else {
			// 			c.logger.Infof("Received artificial snapshot to trigger catchup")
			// 		}
			// 		if err := c.catchUp(sn); err != nil {
			// 			c.logger.Panicf("Failed to recover from snapshot taken at Term %d and Index %d: %s",
			// 				sn.Metadata.Term, sn.Metadata.Index, err)
			// 		}
			// 	case <-c.doneC:
			// 		stopTimer()
			// 		cancelProp()
			// 		select {
			// 		case <-c.errorC: // avoid closing closed channel
			// 		default:
			// 			close(c.errorC)
			// 		}
			// 		c.logger.Infof("Stop serving requests")
			// 		c.periodicChecker.Stop()
			// 		return
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

	if protoutil.IsConfigBlock(block) {
		c.writeConfigBlock(block, index)
		return
	}

	c.mirbftMetadataLock.Lock()
	c.opts.BlockMetadata.RaftIndex = index
	m := protoutil.MarshalOrPanic(c.opts.BlockMetadata)
	c.mirbftMetadataLock.Unlock()

	c.support.WriteBlock(block, m)
}

// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//   -- batches [][]*common.Envelope; the batches cut,
//   -- pending bool; if there are envelopes pending to be ordered,
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.

//FLY2-64 PROPOSED CHANGE
// - from ordered we propose the message
func (c *Chain) ordered(msg *orderer.SubmitRequest) (err error) {
	seq := c.support.Sequence()

	if c.isConfig(msg.Payload) {
		//FLY2-57 proposed change
		// - proposing the configMsg
		c.configInflight = true

		if msg.LastValidationSeq < seq {
			c.logger.Warnf("Config message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
			if err := c.proposeMsg(msg); err != nil {
				return err
			}
		}

		// batch := c.support.BlockCutter().Cut()
		// batches = [][]*common.Envelope{}
		// if len(batch) != 0 {
		// 	batches = append(batches, batch)
		// }
		// batches = append(batches, []*common.Envelope{msg.Payload})
		// return batches, false, nil
	} else if msg.LastValidationSeq < seq { // it is a normal message
		c.logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			c.Metrics.ProposalFailures.Add(1)
			return errors.Errorf("bad normal message: %s", err)
		}

		//FLY2-64 - proposed change
		// new function to propose message
		if err := c.proposeMsg(msg); err != nil {
			return err
		}
	}
	return nil

	// batches = append(batches, []*common.Envelope{msg.Payload})

}

//FLY2-64 - Proposed Change
//New function to propose normal messages to node
func (c *Chain) proposeMsg(msg *orderer.SubmitRequest) (err error) {

	//FLY2-57 proposed changes
	// - added standerdized code for dealing with requests

	// For Testing, we have derived the clientID and request number using the following code
	//convert common.envelope into signedData list
	clientID := c.MirBFTID

	proposer := c.Node.Client(clientID)
	reqNo, err := proposer.NextReqNo() //

	if err != nil {
		return errors.Errorf("Cannot generate Next Request Number")
	}
	//fly2-64 proposed changes
	// - send request using the request object
	req := &pb.Request{
		ClientId: clientID,
		ReqNo:    reqNo,
		Data:     msg.Payload.GetPayload(),
	}
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return errors.Errorf("Cannot marshal Message : %s",err)
	}
	err = proposer.Propose(context.Background(), reqNo, reqBytes)

	if err != nil {
		return errors.WithMessagef(err, "failed to propose message to client %d", clientID)
	}
	return nil

	//In actuality the payload of submit request should be unmarsheld to pb.Request.
	// the following code represents the standered way of addressing a request
	// reqMsg := &pb.Request{}
	// err = proto.Unmarshal(msg.Payload.Payload, reqMsg)
	// if err != nil {
	// 	return errors.WithMessage(err, "unexpected unmarshaling error")
	// }
	// clientID := reqMsg.ClientId
	// data := reqMsg.Data
	// reqNo := reqMsg.ReqNo

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
		if protoutil.IsConfigBlock(b) {
			c.configInflight = true
		}

		c.blockInflight++
	}
}

func (c *Chain) catchUp(snap *raftpb.Snapshot) error {
	b, err := protoutil.UnmarshalBlock(snap.Data)
	if err != nil {
		return errors.Errorf("failed to unmarshal snapshot data to block: %s", err)
	}

	if c.lastBlock.Header.Number >= b.Header.Number {
		c.logger.Warnf("Snapshot is at block [%d], local block number is %d, no sync needed", b.Header.Number, c.lastBlock.Header.Number)
		return nil
	} else if b.Header.Number == c.lastBlock.Header.Number+1 {
		c.logger.Infof("The only missing block [%d] is encapsulated in snapshot, committing it to shortcut catchup process", b.Header.Number)
		c.commitBlock(b)
		c.lastBlock = b
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
		c.commitBlock(block)
		c.lastBlock = block
		next++
	}

	c.logger.Infof("Finished syncing with cluster up to and including block [%d]", b.Header.Number)
	return nil
}

func (c *Chain) commitBlock(block *common.Block) {
	/*	if !protoutil.IsConfigBlock(block) {
			c.support.WriteBlock(block, nil)
			return
		}

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
		}*/
}

func (c *Chain) detectConfChange(block *common.Block) *MembershipChanges {
	/*	// If config is targeting THIS channel, inspect consenter set and
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

		return changes*/
	return &MembershipChanges{
		NewBlockMetadata: nil,
		NewConsenters:    nil,
		AddedNodes:       nil,
		RemovedNodes:     nil,
		ConfChange:       nil,
		RotatedNode:      0,
	}
}

// TODO(harry_knight) Will have to be adapted for hlmirbft as a block is written in this method (line 1047).
// 	Unsure if equivalent ApplyConfChange method exists.
func (c *Chain) apply(ents []raftpb.Entry) {
	/*if len(ents) == 0 {
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

			block := protoutil.UnmarshalBlockOrPanic(ents[i].Data)
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
			var configureComm bool
			if c.confChangeInProgress != nil &&
				c.confChangeInProgress.NodeID == cc.NodeID &&
				c.confChangeInProgress.Type == cc.Type {

				configureComm = true
				c.confChangeInProgress = nil
				c.configInflight = false
				// report the new cluster size
				c.Metrics.ClusterSize.Set(float64(len(c.opts.BlockMetadata.ConsenterIds)))
			}

			lead := atomic.LoadUint64(&c.lastKnownLeader)
			removeLeader := cc.Type == raftpb.ConfChangeRemoveNode && cc.NodeID == lead
			shouldHalt := cc.Type == raftpb.ConfChangeRemoveNode && cc.NodeID == c.raftID

			// unblock `run` go routine so it can still consume Raft messages
			go func() {
				if removeLeader {
					c.logger.Infof("Current leader is being removed from channel, attempt leadership transfer")
					c.Node.abdicateLeader(lead)
				}

				if configureComm && !shouldHalt { // no need to configure comm if this node is going to halt
					if err := c.configureComm(); err != nil {
						c.logger.Panicf("Failed to configure communication: %s", err)
					}
				}

				if shouldHalt {
					c.logger.Infof("This node is being removed from replica set")
					c.halt()
					return
				}
			}()
		}

		if ents[i].Index > c.appliedIndex {
			c.appliedIndex = ents[i].Index
		}
	}

	if c.accDataSize >= c.sizeLimit {
		b := protoutil.UnmarshalBlockOrPanic(ents[position].Data)

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
	}*/
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
	h, err := protoutil.ChannelHeader(env)
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
	c.mirbftMetadataLock.RLock()
	defer c.mirbftMetadataLock.RUnlock()

	var nodes []cluster.RemoteNode
	for raftID, consenter := range c.opts.Consenters {
		// No need to know yourself
		if raftID == c.MirBFTID {
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
	/*hdr, err := ConfigChannelHeader(block)
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

		blockMetadataBytes := protoutil.MarshalOrPanic(c.opts.BlockMetadata)

		// write block with metadata
		c.support.WriteConfigBlock(block, blockMetadataBytes)

		if configMembership == nil {
			return
		}

		// update membership
		if configMembership.ConfChange != nil {
			// We need to propose conf change in a go routine, because it may be blocked if raft node
			// becomes leaderless, and we should not block `run` so it can keep consuming applyC,
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
		m := protoutil.MarshalOrPanic(c.opts.BlockMetadata)
		c.raftMetadataLock.Unlock()

		c.support.WriteConfigBlock(block, m)

	default:
		c.logger.Panicf("Programming error: unexpected config type: %s", common.HeaderType(hdr.Type))
	}*/
}

// getInFlightConfChange returns ConfChange in-flight if any.
// It returns confChangeInProgress if it is not nil. Otherwise
// it returns ConfChange from the last committed block (might be nil).
func (c *Chain) getInFlightConfChange() {
	/*	if c.confChangeInProgress != nil {
			return c.confChangeInProgress
		}

		if c.lastBlock.Header.Number == 0 {
			return nil // nothing to failover just started the chain
		}

		if !protoutil.IsConfigBlock(c.lastBlock) {
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

		return ConfChange(c.opts.BlockMetadata, confState)*/
}

// newMetadata extract config metadata from the configuration block
func (c *Chain) newConfigMetadata(block *common.Block) *hlmirbft.ConfigMetadata {
	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		c.logger.Panicf("error reading consensus metadata: %s", err)
	}
	return metadata
}

// ValidateConsensusMetadata determines the validity of a
// ConsensusMetadata update during config updates on the channel.
func (c *Chain) ValidateConsensusMetadata(oldOrdererConfig, newOrdererConfig channelconfig.Orderer, newChannel bool) error {
	if newOrdererConfig == nil {
		c.logger.Panic("Programming Error: ValidateConsensusMetadata called with nil new channel config")
		return nil
	}

	// metadata was not updated
	if newOrdererConfig.ConsensusMetadata() == nil {
		return nil
	}

	if oldOrdererConfig == nil {
		c.logger.Panic("Programming Error: ValidateConsensusMetadata called with nil old channel config")
		return nil
	}

	if oldOrdererConfig.ConsensusMetadata() == nil {
		c.logger.Panic("Programming Error: ValidateConsensusMetadata called with nil old metadata")
		return nil
	}

	oldMetadata := &hlmirbft.ConfigMetadata{}
	if err := proto.Unmarshal(oldOrdererConfig.ConsensusMetadata(), oldMetadata); err != nil {
		c.logger.Panicf("Programming Error: Failed to unmarshal old hlmirbft consensus metadata: %v", err)
	}

	newMetadata := &hlmirbft.ConfigMetadata{}
	if err := proto.Unmarshal(newOrdererConfig.ConsensusMetadata(), newMetadata); err != nil {
		return errors.Wrap(err, "failed to unmarshal new hlmirbft metadata configuration")
	}

	verifyOpts, err := createX509VerifyOptions(newOrdererConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to create x509 verify options from old and new orderer config")
	}

	if err := VerifyConfigMetadata(newMetadata, verifyOpts); err != nil {
		return errors.Wrap(err, "invalid new config metadata")
	}

	if newChannel {
		// check if the consenters are a subset of the existing consenters (system channel consenters)
		set := ConsentersToMap(oldMetadata.Consenters)
		for _, c := range newMetadata.Consenters {
			if !set.Exists(c) {
				return errors.New("new channel has consenter that is not part of system consenter set")
			}
		}
		return nil
	}

	// create the dummy parameters for ComputeMembershipChanges
	c.mirbftMetadataLock.RLock()
	dummyOldBlockMetadata := proto.Clone(c.opts.BlockMetadata).(*hlmirbft.BlockMetadata)
	c.mirbftMetadataLock.RUnlock()

	dummyOldConsentersMap := CreateConsentersMap(dummyOldBlockMetadata, oldMetadata)
	changes, err := ComputeMembershipChanges(dummyOldBlockMetadata, dummyOldConsentersMap, newMetadata.Consenters)
	if err != nil {
		return err
	}

	// new config metadata was verified above. Additionally need to check new consenters for certificates expiration
	for _, c := range changes.AddedNodes {
		if err := validateConsenterTLSCerts(c, verifyOpts, false); err != nil {
			return errors.Wrapf(err, "consenter %s:%d has invalid certificates", c.Host, c.Port)
		}
	}

	//TODO(harrymknight) Possibly remove c.ActiveNodes field from Metrics
	if changes.UnacceptableQuorumLoss() {
		return errors.Errorf("only %d out of a required 4 nodes are provided, configuration will result in quorum loss", len(changes.NewConsenters))
	}

	return nil
}

// StatusReport returns the ConsensusRelation & Status
func (c *Chain) StatusReport() (types.ConsensusRelation, types.Status) {
	c.statusReportMutex.Lock()
	defer c.statusReportMutex.Unlock()

	return c.consensusRelation, c.status
}

func (c *Chain) suspectEviction() bool {
	if c.isRunning() != nil {
		return false
	}

	return atomic.LoadUint64(&c.lastKnownLeader) == uint64(0)
}

func (c *Chain) newEvictionSuspector() *evictionSuspector {
	consenterCertificate := &ConsenterCertificate{
		Logger:               c.logger,
		ConsenterCertificate: c.opts.Cert,
		CryptoProvider:       c.CryptoProvider,
	}

	return &evictionSuspector{
		amIInChannel:               consenterCertificate.IsConsenterOfChannel,
		evictionSuspicionThreshold: 0,
		writeBlock:                 c.support.Append,
		createPuller:               c.createPuller,
		height:                     c.support.Height,
		triggerCatchUp:             c.triggerCatchup,
		logger:                     c.logger,
		halt: func() {
			c.halt()
		},
	}
}

func (c *Chain) triggerCatchup(sn *raftpb.Snapshot) {
	select {
	case c.snapC <- sn:
	case <-c.doneC:
	}
}

// TODO(harrymknight) Implement these methods
func (c *Chain) Apply(*msgs.QEntry) error {
	return nil
}

func (c *Chain) Snap(networkConfig *msgs.NetworkState_Config, clientsState []*msgs.NetworkState_Client) ([]byte, []*msgs.Reconfiguration, error) {
	return nil, nil, nil
}

func (c *Chain) TransferTo(seqNo uint64, snap []byte) (*msgs.NetworkState, error) {
	return nil, nil
}
