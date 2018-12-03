/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
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
)

// DefaultSnapshotCatchUpEntries is the default number of entries
// to preserve in memory when a snapshot is taken. This is for
// slow followers to catch up.
const DefaultSnapshotCatchUpEntries = uint64(500)

//go:generate mockery -dir . -name Configurator -case underscore -output ./mocks/

// Configurator is used to configure the communication layer
// when the chain starts.
type Configurator interface {
	Configure(channel string, newNodes []cluster.RemoteNode)
}

//go:generate counterfeiter -o mocks/mock_rpc.go . RPC

// RPC is used to mock the transport layer in tests.
type RPC interface {
	Step(dest uint64, msg *orderer.StepRequest) (*orderer.StepResponse, error)
	SendSubmit(dest uint64, request *orderer.SubmitRequest) error
}

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *common.Block
	Close()
}

type block struct {
	b *common.Block

	// i is the etcd/raft entry Index associated with block.
	// it is persisted as block metatdata so we know where
	// to continue rafting upon reboot.
	i uint64
}

// Options contains all the configurations relevant to the chain.
type Options struct {
	RaftID uint64

	Clock clock.Clock

	WALDir       string
	SnapDir      string
	SnapInterval uint64

	// This is configurable mainly for testing purpose. Users are not
	// expected to alter this. Instead, DefaultSnapshotCatchUpEntries is used.
	SnapshotCatchUpEntries uint64

	MemoryStorage MemoryStorage
	Logger        *flogging.FabricLogger

	TickInterval    time.Duration
	ElectionTick    int
	HeartbeatTick   int
	MaxSizePerMsg   uint64
	MaxInflightMsgs int

	RaftMetadata *etcdraft.RaftMetadata
}

// Chain implements consensus.Chain interface.
type Chain struct {
	configurator Configurator

	// access to `SendSubmit` should be serialzed because gRPC is not thread-safe
	submitLock sync.Mutex
	rpc        RPC

	raftID    uint64
	channelID string

	submitC  chan *orderer.SubmitRequest
	commitC  chan block
	observeC chan<- uint64         // Notifies external observer on leader change (passed in optionally as an argument for tests)
	haltC    chan struct{}         // Signals to goroutines that the chain is halting
	doneC    chan struct{}         // Closes when the chain halts
	resignC  chan struct{}         // Notifies node that it is no longer the leader
	startC   chan struct{}         // Closes when the node is started
	snapC    chan *raftpb.Snapshot // Signal to catch up with snapshot

	configChangeAppliedC   chan struct{} // Notifies that a Raft configuration change has been applied
	configChangeInProgress bool          // Flag to indicate node waiting for Raft config change to be applied
	raftMetadataLock       sync.RWMutex

	clock clock.Clock // Tests can inject a fake clock

	support      consensus.ConsenterSupport
	BlockCreator *blockCreator

	leader       uint64
	appliedIndex uint64

	// needed by snapshotting
	lastSnapBlockNum uint64
	syncLock         sync.Mutex       // Protects the manipulation of syncC
	syncC            chan struct{}    // Indicate sync in progress
	confState        raftpb.ConfState // Etcdraft requires ConfState to be persisted within snapshot
	puller           BlockPuller      // Deliver client to pull blocks from other OSNs

	fresh bool // indicate if this is a fresh raft node

	node    raft.Node
	storage *RaftStorage
	opts    Options

	logger *flogging.FabricLogger
}

// NewChain constructs a chain object.
func NewChain(
	support consensus.ConsenterSupport,
	opts Options,
	conf Configurator,
	rpc RPC,
	puller BlockPuller,
	observeC chan<- uint64) (*Chain, error) {

	lg := opts.Logger.With("channel", support.ChainID(), "node", opts.RaftID)

	fresh := !wal.Exist(opts.WALDir)

	appliedi := opts.RaftMetadata.RaftIndex
	storage, err := CreateStorage(lg, appliedi, opts.WALDir, opts.SnapDir, opts.MemoryStorage)
	if err != nil {
		return nil, errors.Errorf("failed to restore persisted raft data: %s", err)
	}

	if opts.SnapshotCatchUpEntries == 0 {
		storage.SnapshotCatchUpEntries = DefaultSnapshotCatchUpEntries
	} else {
		storage.SnapshotCatchUpEntries = opts.SnapshotCatchUpEntries
	}

	// get block number in last snapshot, if exists
	var snapBlkNum uint64
	if s := storage.Snapshot(); !raft.IsEmptySnap(s) {
		b := utils.UnmarshalBlockOrPanic(s.Data)
		snapBlkNum = b.Header.Number
	}

	lastBlock := support.Block(support.Height() - 1)

	return &Chain{
		configurator:         conf,
		rpc:                  rpc,
		channelID:            support.ChainID(),
		raftID:               opts.RaftID,
		submitC:              make(chan *orderer.SubmitRequest),
		commitC:              make(chan block),
		haltC:                make(chan struct{}),
		doneC:                make(chan struct{}),
		resignC:              make(chan struct{}),
		startC:               make(chan struct{}),
		syncC:                make(chan struct{}),
		snapC:                make(chan *raftpb.Snapshot),
		configChangeAppliedC: make(chan struct{}),
		observeC:             observeC,
		support:              support,
		fresh:                fresh,
		BlockCreator:         newBlockCreator(lastBlock, lg),
		appliedIndex:         appliedi,
		lastSnapBlockNum:     snapBlkNum,
		puller:               puller,
		clock:                opts.Clock,
		logger:               lg,
		storage:              storage,
		opts:                 opts,
	}, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting Raft node")

	// DO NOT use Applied option in config, see https://github.com/etcd-io/etcd/issues/10217
	// We guard against replay of written blocks in `entriesToApply` instead.
	config := &raft.Config{
		ID:              c.raftID,
		ElectionTick:    c.opts.ElectionTick,
		HeartbeatTick:   c.opts.HeartbeatTick,
		MaxSizePerMsg:   c.opts.MaxSizePerMsg,
		MaxInflightMsgs: c.opts.MaxInflightMsgs,
		Logger:          c.logger,
		Storage:         c.opts.MemoryStorage,
		// PreVote prevents reconnected node from disturbing network.
		// See etcd/raft doc for more details.
		PreVote:                   true,
		DisableProposalForwarding: true, // This prevents blocks from being accidentally proposed by followers
	}

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	raftPeers := RaftPeers(c.opts.RaftMetadata.Consenters)

	if c.fresh {
		c.logger.Info("starting new raft node")
		c.node = raft.StartNode(config, raftPeers)
	} else {
		c.logger.Info("restarting raft node")
		c.node = raft.RestartNode(config)
	}

	close(c.startC)

	go c.serveRaft()
	go c.serveRequest()
}

// Order submits normal type transactions for ordering.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Content: env, Channel: c.channelID}, 0)
}

// Configure submits config type transactions for ordering.
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	if err := c.checkConfigUpdateValidity(env); err != nil {
		return err
	}
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Content: env, Channel: c.channelID}, 0)
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

	switch chdr.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		return nil
	case int32(common.HeaderType_CONFIG):
		configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
		if err != nil {
			return err
		}

		// Check that only the ConsensusType is updated in the write-set
		if ordererConfigGroup, ok := configUpdate.WriteSet.Groups["Orderer"]; ok {
			if val, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
				return c.checkConsentersSet(val)
			}
		}
		return nil

	default:
		return errors.Errorf("config transaction has unknown header type")
	}
}

// WaitReady blocks when the chain:
// - is catching up with other nodes using snapshot
//
// In any other case, it returns right away.
func (c *Chain) WaitReady() error {
	if err := c.isRunning(); err != nil {
		return err
	}

	c.syncLock.Lock()
	ch := c.syncC
	c.syncLock.Unlock()

	select {
	case <-ch:
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}

	return nil
}

// Errored returns a channel that closes when the chain stops.
func (c *Chain) Errored() <-chan struct{} {
	return c.doneC
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

// Step passes the given StepRequest message to the raft.Node instance
func (c *Chain) Step(req *orderer.StepRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		return err
	}

	stepMsg := &raftpb.Message{}
	if err := proto.Unmarshal(req.Payload, stepMsg); err != nil {
		return fmt.Errorf("failed to unmarshal StepRequest payload to Raft Message: %s", err)
	}

	if err := c.node.Step(context.TODO(), *stepMsg); err != nil {
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
		return err
	}

	lead := atomic.LoadUint64(&c.leader)

	if lead == raft.None {
		return errors.Errorf("no Raft leader")
	}

	if lead == c.raftID {
		select {
		case c.submitC <- req:
			return nil
		case <-c.doneC:
			return errors.Errorf("chain is stopped")
		}
	}

	c.logger.Debugf("Forwarding submit request to Raft leader %d", lead)
	c.submitLock.Lock()
	defer c.submitLock.Unlock()
	return c.rpc.SendSubmit(lead, req)
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
	start := func() {
		if !ticking {
			ticking = true
			timer.Reset(c.support.SharedConfig().BatchTimeout())
		}
	}

	stop := func() {
		if !timer.Stop() && ticking {
			// we only need to drain the channel if the timer expired (not explicitly stopped)
			<-timer.C()
		}
		ticking = false
	}

	if s := c.storage.Snapshot(); !raft.IsEmptySnap(s) {
		if err := c.catchUp(&s); err != nil {
			c.logger.Errorf("Failed to recover from snapshot taken at Term %d and Index %d: %s",
				s.Metadata.Term, s.Metadata.Index, err)
		}
	} else {
		close(c.syncC)
	}

	for {
		select {
		case msg := <-c.submitC:
			batches, pending, err := c.ordered(msg)
			if err != nil {
				c.logger.Errorf("Failed to order message: %s", err)
			}
			if pending {
				start() // no-op if timer is already started
			} else {
				stop()
			}

			if err := c.commitBatches(batches...); err != nil {
				c.logger.Errorf("Failed to commit block: %s", err)
			}

		case b := <-c.commitC:
			c.writeBlock(b)

		case <-c.resignC:
			_ = c.support.BlockCutter().Cut()
			c.BlockCreator.resetCreatedBlocks()
			stop()

		case <-timer.C():
			ticking = false

			batch := c.support.BlockCutter().Cut()
			if len(batch) == 0 {
				c.logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}

			c.logger.Debugf("Batch timer expired, creating block")
			if err := c.commitBatches(batch); err != nil {
				c.logger.Errorf("Failed to commit block: %s", err)
			}

		case sn := <-c.snapC:
			if err := c.catchUp(sn); err != nil {
				c.logger.Errorf("Failed to recover from snapshot taken at Term %d and Index %d: %s",
					sn.Metadata.Term, sn.Metadata.Index, err)
			}

		case <-c.doneC:
			c.logger.Infof("Stop serving requests")
			return
		}
	}
}

func (c *Chain) writeBlock(b block) {
	c.BlockCreator.commitBlock(b.b)
	if utils.IsConfigBlock(b.b) {
		if err := c.writeConfigBlock(b); err != nil {
			c.logger.Panicf("failed to write configuration block, %+v", err)
		}
		return
	}

	c.raftMetadataLock.Lock()
	c.opts.RaftMetadata.RaftIndex = b.i
	m := utils.MarshalOrPanic(c.opts.RaftMetadata)
	c.raftMetadataLock.Unlock()

	c.support.WriteBlock(b.b, m)
}

// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//   -- batches [][]*common.Envelope; the batches cut,
//   -- pending bool; if there are envelopes pending to be ordered,
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.
func (c *Chain) ordered(msg *orderer.SubmitRequest) (batches [][]*common.Envelope, pending bool, err error) {
	seq := c.support.Sequence()

	if c.isConfig(msg.Content) {
		// ConfigMsg
		if msg.LastValidationSeq < seq {
			msg.Content, _, err = c.support.ProcessConfigMsg(msg.Content)
			if err != nil {
				return nil, true, errors.Errorf("bad config message: %s", err)
			}
		}
		batch := c.support.BlockCutter().Cut()
		batches = [][]*common.Envelope{}
		if len(batch) != 0 {
			batches = append(batches, batch)
		}
		batches = append(batches, []*common.Envelope{msg.Content})
		return batches, false, nil
	}
	// it is a normal message
	if msg.LastValidationSeq < seq {
		if _, err := c.support.ProcessNormalMsg(msg.Content); err != nil {
			return nil, true, errors.Errorf("bad normal message: %s", err)
		}
	}
	batches, pending = c.support.BlockCutter().Ordered(msg.Content)
	return batches, pending, nil

}

func (c *Chain) commitBatches(batches ...[]*common.Envelope) error {
	for _, batch := range batches {
		b := c.BlockCreator.createNextBlock(batch)
		data := utils.MarshalOrPanic(b)
		if err := c.node.Propose(context.TODO(), data); err != nil {
			return errors.Errorf("failed to propose data to Raft node: %s", err)
		}

		// if it is config block, then wait for the commit of the block
		if utils.IsConfigBlock(b) {
			// we need the loop to account for the normal blocks that might be in-flight before the arrival of the config block
		commitConfigLoop:
			for {
				select {
				case block := <-c.commitC:
					c.writeBlock(block)
					// since this is the config block that have been looking for, we break out of the loop
					if bytes.Equal(b.Header.Bytes(), block.b.Header.Bytes()) {
						break commitConfigLoop
					}

				case <-c.resignC:
					return errors.Errorf("aborted block committing: lost leadership")

				case <-c.doneC:
					return nil
				}
			}
		}
	}

	return nil
}

func (c *Chain) catchUp(snap *raftpb.Snapshot) error {
	b, err := utils.UnmarshalBlock(snap.Data)
	if err != nil {
		return errors.Errorf("failed to unmarshal snapshot data to block: %s", err)
	}

	c.logger.Infof("Catching up with snapshot taken at block %d", b.Header.Number)

	next := c.support.Height()
	if next > b.Header.Number {
		c.logger.Warnf("Snapshot is at block %d, local block number is %d, no sync needed", b.Header.Number, next-1)
		return nil
	}

	c.syncLock.Lock()
	c.syncC = make(chan struct{})
	c.syncLock.Unlock()
	defer func() {
		close(c.syncC)
		c.puller.Close()
	}()

	for next <= b.Header.Number {
		block := c.puller.PullBlock(next)
		if block == nil {
			return errors.Errorf("failed to fetch block %d from cluster", next)
		}

		c.BlockCreator.commitBlock(block)
		if utils.IsConfigBlock(block) {
			c.support.WriteConfigBlock(block, nil)
		} else {
			c.support.WriteBlock(block, nil)
		}

		next++
	}

	c.logger.Infof("Finished syncing with cluster up to block %d (incl.)", b.Header.Number)
	return nil
}

func (c *Chain) serveRaft() {
	ticker := c.clock.NewTicker(c.opts.TickInterval)

	for {
		select {
		case <-ticker.C():
			c.node.Tick()

		case rd := <-c.node.Ready():
			if err := c.storage.Store(rd.Entries, rd.HardState, rd.Snapshot); err != nil {
				c.logger.Panicf("Failed to persist etcd/raft data: %s", err)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				c.snapC <- &rd.Snapshot

				b := utils.UnmarshalBlockOrPanic(rd.Snapshot.Data)
				c.lastSnapBlockNum = b.Header.Number
				c.confState = rd.Snapshot.Metadata.ConfState
				c.appliedIndex = rd.Snapshot.Metadata.Index
			}

			c.apply(rd.CommittedEntries)
			c.node.Advance()

			// TODO(jay_guo) leader can write to disk in parallel with replicating
			// to the followers and them writing to their disks. Check 10.2.1 in thesis
			c.send(rd.Messages)

			if rd.SoftState != nil {
				newLead := atomic.LoadUint64(&rd.SoftState.Lead)
				lead := atomic.LoadUint64(&c.leader)
				if newLead != lead {
					c.logger.Infof("Raft leader changed: %d -> %d", lead, newLead)
					atomic.StoreUint64(&c.leader, newLead)

					if lead == c.raftID {
						c.resignC <- struct{}{}
					}

					// becoming a leader and configuration change is in progress
					if newLead == c.raftID && c.configChangeInProgress {
						// need to read recent config updates of replica set
						// and finish reconfiguration
						c.handleReconfigurationFailover()
					}

					// notify external observer
					select {
					case c.observeC <- newLead:
					default:
					}
				}
			}

		case <-c.haltC:
			ticker.Stop()
			c.node.Stop()
			c.storage.Close()
			c.logger.Infof("Raft node stopped")
			close(c.doneC) // close after all the artifacts are closed
			return
		}
	}
}

func (c *Chain) apply(ents []raftpb.Entry) {
	if len(ents) == 0 {
		return
	}

	if ents[0].Index > c.appliedIndex+1 {
		c.logger.Panicf("first index of committed entry[%d] should <= appliedIndex[%d]+1", ents[0].Index, c.appliedIndex)
	}

	var appliedb uint64
	var position int
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			// We need to strictly avoid re-applying normal entries,
			// otherwise we are writing the same block twice.
			if len(ents[i].Data) == 0 || ents[i].Index <= c.appliedIndex {
				break
			}

			b := utils.UnmarshalBlockOrPanic(ents[i].Data)
			// need to check whenever given block carries updates
			// which will lead to membership change and eventually
			// to the cluster reconfiguration
			c.raftMetadataLock.RLock()
			m := c.opts.RaftMetadata
			c.raftMetadataLock.RUnlock()

			isConfigMembershipUpdate, err := IsMembershipUpdate(b, m)
			if err != nil {
				c.logger.Warnf("Error while attempting to determine membership update, due to %s", err)
			}
			// if error occurred isConfigMembershipUpdate will be false, hence will skip setting config change in
			// progress
			if isConfigMembershipUpdate {
				// set flag config change is progress only if config block
				// and has updates for raft replica set
				c.configChangeInProgress = true
			}

			c.commitC <- block{b, ents[i].Index}

			appliedb = b.Header.Number
			position = i

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				c.logger.Warnf("Failed to unmarshal ConfChange data: %s", err)
				continue
			}

			c.confState = *c.node.ApplyConfChange(cc)

			if c.configChangeInProgress {
				// signal that config changes has been applied
				c.configChangeAppliedC <- struct{}{}
				// set flag back
				c.configChangeInProgress = false
			}
		}

		if ents[i].Index > c.appliedIndex {
			c.appliedIndex = ents[i].Index
		}
	}

	if c.opts.SnapInterval == 0 || appliedb == 0 {
		// snapshot is not enabled (SnapInterval == 0) or
		// no block has been written (appliedb == 0) in this round
		return
	}

	if appliedb-c.lastSnapBlockNum >= c.opts.SnapInterval {
		c.logger.Infof("Taking snapshot at block %d, last snapshotted block number is %d", appliedb, c.lastSnapBlockNum)
		if err := c.storage.TakeSnapshot(c.appliedIndex, &c.confState, ents[position].Data); err != nil {
			c.logger.Fatalf("Failed to create snapshot at index %d", c.appliedIndex)
		}

		c.lastSnapBlockNum = appliedb
	}
}

func (c *Chain) send(msgs []raftpb.Message) {
	for _, msg := range msgs {
		if msg.To == 0 {
			continue
		}

		status := raft.SnapshotFinish

		msgBytes := utils.MarshalOrPanic(&msg)
		_, err := c.rpc.Step(msg.To, &orderer.StepRequest{Channel: c.support.ChainID(), Payload: msgBytes})
		if err != nil {
			// TODO We should call ReportUnreachable if message delivery fails
			c.logger.Errorf("Failed to send StepRequest to %d, because: %s", msg.To, err)

			status = raft.SnapshotFailure
		}

		if msg.Type == raftpb.MsgSnap {
			c.node.ReportSnapshot(msg.To, status)
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
	nodes, err := c.remotePeers()
	if err != nil {
		return err
	}

	c.configurator.Configure(c.channelID, nodes)
	return nil
}

func (c *Chain) remotePeers() ([]cluster.RemoteNode, error) {
	var nodes []cluster.RemoteNode
	for raftID, consenter := range c.opts.RaftMetadata.Consenters {
		// No need to know yourself
		if raftID == c.raftID {
			continue
		}
		serverCertAsDER, err := c.pemToDER(consenter.ServerTlsCert, raftID, "server")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := c.pemToDER(consenter.ClientTlsCert, raftID, "client")
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

func (c *Chain) pemToDER(pemBytes []byte, id uint64, certType string) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		c.logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// checkConsentersSet validates correctness of the consenters set provided within configuration value
func (c *Chain) checkConsentersSet(configValue *common.ConfigValue) error {
	// read metadata update from configuration
	updatedMetadata, err := MetadataFromConfigValue(configValue)
	if err != nil {
		return err
	}

	c.raftMetadataLock.RLock()
	changes := ComputeMembershipChanges(c.opts.RaftMetadata.Consenters, updatedMetadata.Consenters)
	c.raftMetadataLock.RUnlock()

	if changes.TotalChanges > 1 {
		return errors.New("update of more than one consenters at a time is not supported")
	}

	return nil
}

// updateMembership updates raft metadata with new membership changes, apply raft changes to replica set
// by proposing config change and blocking until it get applied
func (c *Chain) updateMembership(metadata *etcdraft.RaftMetadata, change *raftpb.ConfChange) error {
	lead := atomic.LoadUint64(&c.leader)
	// leader to propose configuration change
	if lead == c.raftID {
		// ProposeConfChange returns error only if node being stopped.
		if err := c.node.ProposeConfChange(context.TODO(), *change); err != nil {
			c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
			return nil
		}
	}

	var err error

	for {
		select {
		case <-c.configChangeAppliedC: // Raft configuration changes of the raft cluster has been applied
			// update metadata once we have block committed
			c.raftMetadataLock.Lock()
			c.opts.RaftMetadata = metadata
			c.raftMetadataLock.Unlock()

			// now we need to reconfigure the communication layer with new updates
			return c.configureComm()
		case <-c.resignC:
			c.logger.Debug("Raft cluster leader has changed, new leader should re-propose Raft config change based on last config block")
		case <-c.doneC:
			c.logger.Debug("Shutting down node, aborting config change update")
			return err
		}
	}
}

// writeConfigBlock writes configuration blocks into the ledger in
// addition extracts updates about raft replica set and if there
// are changes updates cluster membership as well
func (c *Chain) writeConfigBlock(b block) error {
	metadata, raftMetadata := c.newRaftMetadata(b.b)

	var changes *MembershipChanges
	if metadata != nil {
		changes = ComputeMembershipChanges(raftMetadata.Consenters, metadata.Consenters)
	}

	confChange := changes.UpdateRaftMetadataAndConfChange(raftMetadata)
	raftMetadata.RaftIndex = b.i

	raftMetadataBytes := utils.MarshalOrPanic(raftMetadata)
	// write block with metadata
	c.support.WriteConfigBlock(b.b, raftMetadataBytes)
	if confChange != nil {
		if err := c.updateMembership(raftMetadata, confChange); err != nil {
			return errors.Wrap(err, "failed to update Raft with consenters membership changes")
		}
	}
	return nil
}

// handleReconfigurationFailover read last configuration block and proposes
// new raft configuration
func (c *Chain) handleReconfigurationFailover() {
	b := c.support.Block(c.support.Height() - 1)
	if b == nil {
		c.logger.Panic("nil block, failed to read last written block")
	}
	if !utils.IsConfigBlock(b) {
		// a node (leader or follower) leaving updateMembership in context of serverReq go routine,
		// *iff* configuration entry has appeared and successfully applied.
		// while it's blocked in updateMembership, it cannot commit any other block,
		// therefore we guarantee the last block is config block
		c.logger.Panic("while handling reconfiguration failover last expected block should be configuration")
	}

	metadata, raftMetadata := c.newRaftMetadata(b)

	var changes *MembershipChanges
	if metadata != nil {
		changes = ComputeMembershipChanges(raftMetadata.Consenters, metadata.Consenters)
	}

	confChange := changes.UpdateRaftMetadataAndConfChange(raftMetadata)
	if err := c.node.ProposeConfChange(context.TODO(), *confChange); err != nil {
		c.logger.Warnf("failed to propose configuration update to Raft node: %s", err)
	}
}

// newRaftMetadata extract raft metadata from the configuration block
func (c *Chain) newRaftMetadata(block *common.Block) (*etcdraft.Metadata, *etcdraft.RaftMetadata) {
	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		c.logger.Panicf("error reading consensus metadata: %s", err)
	}
	c.raftMetadataLock.RLock()
	raftMetadata := proto.Clone(c.opts.RaftMetadata).(*etcdraft.RaftMetadata)
	// proto.Clone doesn't copy an empty map, hence need to initialize it after
	// cloning
	if raftMetadata.Consenters == nil {
		raftMetadata.Consenters = map[uint64]*etcdraft.Consenter{}
	}
	c.raftMetadataLock.RUnlock()
	return metadata, raftMetadata
}
