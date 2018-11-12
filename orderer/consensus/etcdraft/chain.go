/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"context"
	"encoding/pem"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
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

	WALDir        string
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
	rpc          RPC

	raftID    uint64
	channelID string

	submitC  chan *orderer.SubmitRequest
	commitC  chan block
	observeC chan<- uint64 // Notifies external observer on leader change (passed in optionally as an argument for tests)
	haltC    chan struct{} // Signals to goroutines that the chain is halting
	doneC    chan struct{} // Closes when the chain halts
	resignC  chan struct{} // Notifies node that it is no longer the leader
	startC   chan struct{} // Closes when the node is started

	clock clock.Clock // Tests can inject a fake clock

	support consensus.ConsenterSupport

	leader       uint64
	appliedIndex uint64

	hasWAL bool // indicate if this is a fresh raft node

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
	observeC chan<- uint64) (*Chain, error) {

	lg := opts.Logger.With("channel", support.ChainID(), "node", opts.RaftID)

	applied := opts.RaftMetadata.RaftIndex
	storage, hasWAL, err := Restore(lg, applied, opts.WALDir, opts.MemoryStorage)
	if err != nil {
		return nil, errors.Errorf("failed to restore persited raft data: %s", err)
	}

	return &Chain{
		configurator: conf,
		rpc:          rpc,
		channelID:    support.ChainID(),
		raftID:       opts.RaftID,
		submitC:      make(chan *orderer.SubmitRequest),
		commitC:      make(chan block),
		haltC:        make(chan struct{}),
		doneC:        make(chan struct{}),
		resignC:      make(chan struct{}),
		startC:       make(chan struct{}),
		observeC:     observeC,
		support:      support,
		hasWAL:       hasWAL,
		appliedIndex: applied,
		clock:        opts.Clock,
		logger:       lg,
		storage:      storage,
		opts:         opts,
	}, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting Raft node")

	// DO NOT use Applied option in config, see https://github.com/etcd-io/etcd/issues/10217
	// We guard against replay of written blocks in `entriesToApply` instead.
	config := &raft.Config{
		ID:                        c.raftID,
		ElectionTick:              c.opts.ElectionTick,
		HeartbeatTick:             c.opts.HeartbeatTick,
		MaxSizePerMsg:             c.opts.MaxSizePerMsg,
		MaxInflightMsgs:           c.opts.MaxInflightMsgs,
		Logger:                    c.logger,
		Storage:                   c.opts.MemoryStorage,
		DisableProposalForwarding: true, // This prevents blocks from being accidentally proposed by followers
	}

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	raftPeers := RaftPeers(c.opts.RaftMetadata.Consenters)

	if !c.hasWAL {
		c.logger.Infof("starting new raft node %d", c.raftID)
		c.node = raft.StartNode(config, raftPeers)
	} else {
		c.logger.Infof("restarting raft node %d", c.raftID)
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
		configEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
		if err != nil {
			return err
		}
		configUpdateEnv, err := utils.EnvelopeToConfigUpdate(configEnv.LastUpdate)
		if err != nil {
			return err
		}
		configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
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

// WaitReady is currently a no-op.
func (c *Chain) WaitReady() error {
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

		case <-c.doneC:
			c.logger.Infof("Stop serving requests")
			return
		}
	}
}

func (c *Chain) writeBlock(b block) {
	c.opts.RaftMetadata.RaftIndex = b.i
	m := utils.MarshalOrPanic(c.opts.RaftMetadata)

	if utils.IsConfigBlock(b.b) {
		c.support.WriteConfigBlock(b.b, m)
		return
	}

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
		b := c.support.CreateNextBlock(batch)
		data := utils.MarshalOrPanic(b)
		if err := c.node.Propose(context.TODO(), data); err != nil {
			return errors.Errorf("failed to propose data to Raft node: %s", err)
		}

		select {
		case block := <-c.commitC:
			c.writeBlock(block)
		case <-c.resignC:
			return errors.Errorf("aborted block committing: lost leadership")

		case <-c.doneC:
			return nil
		}
	}

	return nil
}

func (c *Chain) serveRaft() {
	ticker := c.clock.NewTicker(c.opts.TickInterval)

	for {
		select {
		case <-ticker.C():
			c.node.Tick()

		case rd := <-c.node.Ready():
			if err := c.storage.Store(rd.Entries, rd.HardState); err != nil {
				c.logger.Panicf("Failed to persist etcd/raft data: %s", err)
			}
			c.apply(rd.CommittedEntries)
			c.node.Advance()
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

	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			// We need to strictly avoid re-applying normal entries,
			// otherwise we are writing the same block twice.
			if len(ents[i].Data) == 0 || ents[i].Index <= c.appliedIndex {
				break
			}

			c.commitC <- block{utils.UnmarshalBlockOrPanic(ents[i].Data), ents[i].Index}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				c.logger.Warnf("Failed to unmarshal ConfChange data: %s", err)
				continue
			}

			c.node.ApplyConfChange(cc)
		}

		if ents[i].Index > c.appliedIndex {
			c.appliedIndex = ents[i].Index
		}
	}
}

func (c *Chain) send(msgs []raftpb.Message) {
	for _, msg := range msgs {
		if msg.To == 0 {
			continue
		}

		msgBytes := utils.MarshalOrPanic(&msg)
		_, err := c.rpc.Step(msg.To, &orderer.StepRequest{Channel: c.support.ChainID(), Payload: msgBytes})
		if err != nil {
			// TODO We should call ReportUnreachable if message delivery fails
			c.logger.Errorf("Failed to send StepRequest to %d, because: %s", msg.To, err)
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

func (c *Chain) checkConsentersSet(configValue *common.ConfigValue) error {
	consensusTypeValue := &orderer.ConsensusType{}
	if err := proto.Unmarshal(configValue.Value, consensusTypeValue); err != nil {
		return errors.Wrap(err, "failed to unmarshal consensusType config update")
	}

	updatedMetadata := &etcdraft.Metadata{}
	if err := proto.Unmarshal(consensusTypeValue.Metadata, updatedMetadata); err != nil {
		return errors.Wrap(err, "failed to unmarshal updated (new) etcdraft metadata configuration")
	}

	if !ConsentersChanged(c.opts.RaftMetadata.Consenters, updatedMetadata.Consenters) {
		return errors.New("update of consenters set is not supported yet")
	}

	return nil
}

func (c *Chain) consentersChanged(newConsenters []*etcdraft.Consenter) bool {
	if len(c.opts.RaftMetadata.Consenters) != len(newConsenters) {
		return false
	}

	consentersSet1 := c.membershipByCert()
	consentersSet2 := c.consentersToMap(newConsenters)

	return reflect.DeepEqual(consentersSet1, consentersSet2)
}

func (c *Chain) membershipByCert() map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range c.opts.RaftMetadata.Consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

func (c *Chain) consentersToMap(consenters []*etcdraft.Consenter) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

func (c *Chain) membershipToRaftPeers() []raft.Peer {
	var peers []raft.Peer

	for raftID := range c.opts.RaftMetadata.Consenters {
		peers = append(peers, raft.Peer{ID: raftID})
	}
	return peers
}
