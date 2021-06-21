/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"context"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/fly2plan/fabric-protos-go/orderer/hlmirbft"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/mirbft"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
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

	//JIRA FLY2-57: Prepend flag to check request is forward
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

type submit struct {
	req *orderer.SubmitRequest
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

	errorCLock sync.RWMutex
	errorC     chan struct{} // returned by Errored()

	mirbftMetadataLock sync.RWMutex
	configInflight     bool // this is true when there is config block or ConfChange in flight
	blockInflight      int  // number of in flight blocks

	clock clock.Clock // Tests can inject a fake clock

	support consensus.ConsenterSupport

	lastBlock    *common.Block
	appliedIndex uint64

	// needed by snapshotting
	lastSnapBlockNum uint64

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

	go c.run()

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

// Check for forward flag in payload
func (c *Chain) CheckPrependForwardFlag(reqPayload []byte) bool {

	prependedFlag := reqPayload[:9]

	if string(prependedFlag) == ForwardFlag {
		return true
	}

	return false

}

func (c *Chain) PrependForwardFlag(reqPayload []byte) []byte {
	forwardFlag := []byte(ForwardFlag)
	prependReq := append([]byte{}, forwardFlag...)
	prependReq = append(prependReq, reqPayload...)
	return prependReq
}

// Submit forwards the incoming request to:
// - to all nodes via the transport mechanism if the request hasn't been forwarded
// - the underlying state machine if the request has been forwarded
func (c *Chain) Submit(req *orderer.SubmitRequest, sender uint64) error {

	if err := c.isRunning(); err != nil {
		c.Metrics.ProposalFailures.Add(1)
		return err
	}

	submitPayload := req.Payload.GetPayload()
	isForwardRequest := c.CheckPrependForwardFlag(submitPayload)

	if !isForwardRequest {

		req.Payload.Payload = c.PrependForwardFlag(submitPayload)

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

	req.Payload.Payload = submitPayload[9:]

	return c.ordered(req)

}

type apply struct {
	entries []raftpb.Entry
	soft    *raft.SoftState
}

func isCandidate(state raft.StateType) bool {
	return state == raft.StatePreCandidate || state == raft.StateCandidate
}

func (c *Chain) run() {

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
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.

//JIRA FLY2-57 - proposed changes
func (c *Chain) ordered(msg *orderer.SubmitRequest) (err error) {

	seq := c.support.Sequence()

	if msg.LastValidationSeq < seq {

		if c.isConfig(msg.Payload) {

			c.configInflight = true //JIRA FLY2-57 - proposed changes
		}

		c.logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)

		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			c.Metrics.ProposalFailures.Add(1)
			return errors.Errorf("bad normal message: %s", err)
		}

	}

	return c.proposeMsg(msg)

}

//FLY2-57 - Proposed Change: New function to propose normal messages to node
func (c *Chain) proposeMsg(msg *orderer.SubmitRequest) (err error) {

	clientID := c.MirBFTID
	proposer := c.Node.Client(clientID)
	reqNo, err := proposer.NextReqNo()

	if err != nil {
		return errors.Errorf("failed to generate next request number")
	}
	req := &msgs.Request{
		ClientId: clientID,
		ReqNo:    reqNo,
		Data:     msg.Payload.GetPayload(),
	}

	reqBytes, err := proto.Marshal(req)

	if err != nil {
		return errors.Errorf("Cannot marshal Message : %s", err)
	}

	err = proposer.Propose(context.Background(), reqNo, reqBytes)

	if err != nil {
		return errors.WithMessagef(err, "failed to propose message to client %d", clientID)
	}

	return nil

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

}

func (c *Chain) detectConfChange(block *common.Block) *MembershipChanges {

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

}

// getInFlightConfChange returns ConfChange in-flight if any.
// It returns confChangeInProgress if it is not nil. Otherwise
// it returns ConfChange from the last committed block (might be nil).
func (c *Chain) getInFlightConfChange() {

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
	// TODO(harrymknight) Possible Optimisation: Mir allows for complete reconfiguration i.e. add/remove
	//  multiple orderer nodes at a time. Check if current restriction (only remove/add one orderer node at a time)
	//  is imposed by Raft or Fabric.
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

//JIRA FLY2-66 proposed changes:Implemented the Snap Function
func (c *Chain) Snap(networkConfig *msgs.NetworkState_Config, clientsState []*msgs.NetworkState_Client) ([]byte, []*msgs.Reconfiguration, error) {

	pr := c.Node.PendingReconfigurations

	c.Node.PendingReconfigurations = nil

	data, err := proto.Marshal(&msgs.NetworkState{
		Config:                  networkConfig,
		Clients:                 clientsState,
		PendingReconfigurations: pr,
	})

	if err != nil {

		return nil, nil, errors.WithMessage(err, "could not marsshal network state")

	}

	c.Node.CheckpointSeqNo++

	countValue := make([]byte, 8)

	binary.BigEndian.PutUint64(countValue, uint64(c.Node.CheckpointSeqNo))

	networkStates := append(countValue, data...)

	err = c.Node.PersistSnapshot(c.Node.CheckpointSeqNo, networkStates)

	if err != nil {

		c.logger.Panicf("error while snap persist : %s", err)

	}

	return networkStates, pr, nil

}

//JIRA FLY2-58 proposed changes:Implemented the TransferTo Function
func (c *Chain) TransferTo(seqNo uint64, snap []byte) (*msgs.NetworkState, error) {

	networkState := &msgs.NetworkState{}

	checkSeqNo := snap[:8] //get the sequence number of the snap

	snapShot, err := c.Node.ReadSnapFiles(binary.BigEndian.Uint64(checkSeqNo), c.opts.SnapDir)

	if err != nil {

		return nil, err
	}

	if err := proto.Unmarshal(snapShot[8:], networkState); err != nil {

		return nil, err

	}

	return networkState, nil
}
