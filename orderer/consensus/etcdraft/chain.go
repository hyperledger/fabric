/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"context"
	"encoding/pem"
	"fmt"
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

// Storage is currently backed by etcd/raft.MemoryStorage. This interface is
// defined to expose dependencies of fsm so that it may be swapped in the
// future. TODO(jay) Add other necessary methods to this interface once we need
// them in implementation, e.g. ApplySnapshot.
type Storage interface {
	raft.Storage
	Append(entries []raftpb.Entry) error
}

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

// Options contains all the configurations relevant to the chain.
type Options struct {
	RaftID uint64

	Clock clock.Clock

	Storage Storage
	Logger  *flogging.FabricLogger

	TickInterval    time.Duration
	ElectionTick    int
	HeartbeatTick   int
	MaxSizePerMsg   uint64
	MaxInflightMsgs int
	Peers           []raft.Peer
}

// Chain implements consensus.Chain interface.
type Chain struct {
	configurator Configurator
	rpc          RPC

	raftID    uint64
	channelID string

	submitC  chan *orderer.SubmitRequest
	commitC  chan *common.Block
	observeC chan<- uint64 // Notifies external observer on leader change (passed in optionally as an argument for tests)
	haltC    chan struct{} // Signals to goroutines that the chain is halting
	doneC    chan struct{} // Closes when the chain halts
	resignC  chan struct{} // Notifies node that it is no longer the leader
	startC   chan struct{} // Closes when the node is started

	clock clock.Clock // Tests can inject a fake clock

	support consensus.ConsenterSupport

	leader       uint64
	appliedIndex uint64

	node    raft.Node
	storage Storage
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
	return &Chain{
		configurator: conf,
		rpc:          rpc,
		channelID:    support.ChainID(),
		raftID:       opts.RaftID,
		submitC:      make(chan *orderer.SubmitRequest),
		commitC:      make(chan *common.Block),
		haltC:        make(chan struct{}),
		doneC:        make(chan struct{}),
		resignC:      make(chan struct{}),
		startC:       make(chan struct{}),
		observeC:     observeC,
		support:      support,
		clock:        opts.Clock,
		logger:       opts.Logger.With("channel", support.ChainID(), "node", opts.RaftID),
		storage:      opts.Storage,
		opts:         opts,
	}, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting Raft node")
	config := &raft.Config{
		ID:                        c.raftID,
		ElectionTick:              c.opts.ElectionTick,
		HeartbeatTick:             c.opts.HeartbeatTick,
		MaxSizePerMsg:             c.opts.MaxSizePerMsg,
		MaxInflightMsgs:           c.opts.MaxInflightMsgs,
		Logger:                    c.logger,
		Storage:                   c.opts.Storage,
		DisableProposalForwarding: true, // This prevents blocks from being accidentally proposed by followers
	}

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	c.node = raft.StartNode(config, c.opts.Peers)
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

		// TODO Consider the read-set when processing type B configuration transactions
		// Check that only the ConsensusType is updated in the write-set
		if ordererConfigGroup, ok := configUpdate.WriteSet.Groups["Orderer"]; ok {
			if _, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
				return errors.Errorf("updates to ConsensusType not supported currently")
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

func (c *Chain) writeBlock(b *common.Block) {
	if utils.IsConfigBlock(b) {
		c.support.WriteConfigBlock(b, nil)
		return
	}

	c.support.WriteBlock(b, nil)
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
			if utils.IsConfigBlock(block) {
				c.support.WriteConfigBlock(block, nil)
			} else {
				c.support.WriteBlock(block, nil)
			}

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
			c.storage.Append(rd.Entries)
			c.apply(c.entriesToApply(rd.CommittedEntries))
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
			close(c.doneC)
			ticker.Stop()
			c.node.Stop()
			c.logger.Infof("Raft node %x stopped", c.raftID)
			return
		}
	}
}

func (c *Chain) apply(ents []raftpb.Entry) {
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				break
			}

			c.commitC <- utils.UnmarshalBlockOrPanic(ents[i].Data)

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				c.logger.Warnf("Failed to unmarshal ConfChange data: %s", err)
				continue
			}

			c.node.ApplyConfChange(cc)
		}

		c.appliedIndex = ents[i].Index
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

// this is taken from coreos/contrib/raftexample/raft.go
func (c *Chain) entriesToApply(ents []raftpb.Entry) (nents []raftpb.Entry) {
	if len(ents) == 0 {
		return
	}

	firstIdx := ents[0].Index
	if firstIdx > c.appliedIndex+1 {
		c.logger.Panicf("first index of committed entry[%d] should <= progress.appliedIndex[%d]+1", firstIdx, c.appliedIndex)
	}

	// If we do have unapplied entries in nents.
	//    |     applied    |       unapplied      |
	//    |----------------|----------------------|
	// firstIdx       appliedIndex              last
	if c.appliedIndex-firstIdx+1 < uint64(len(ents)) {
		nents = ents[c.appliedIndex-firstIdx+1:]
	}
	return nents
}

func (c *Chain) isConfig(env *common.Envelope) bool {
	h, err := utils.ChannelHeader(env)
	if err != nil {
		c.logger.Panicf("failed to extract channel header from envelope")
	}

	return h.Type == int32(common.HeaderType_CONFIG) || h.Type == int32(common.HeaderType_ORDERER_TRANSACTION)
}

func (c *Chain) configureComm() error {
	nodes, err := c.nodeConfigFromMetadata()
	if err != nil {
		return err
	}

	c.configurator.Configure(c.channelID, nodes)
	return nil
}

func (c *Chain) nodeConfigFromMetadata() ([]cluster.RemoteNode, error) {
	var nodes []cluster.RemoteNode
	m := &etcdraft.Metadata{}
	if err := proto.Unmarshal(c.support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to extract consensus metadata")
	}

	for id, consenter := range m.Consenters {
		raftID := uint64(id + 1)
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
