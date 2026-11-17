/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"bytes"
	"context"
	"encoding/asn1"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine/protos"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// tipState is the last block the engine has finalized on this node. It runs
// ahead of the ledger by the blocks that are still collecting signatures.
type tipState struct {
	number     uint64
	hash       []byte // header hash of the block at `number`
	lastConfig uint64
}

// pendingBlock is a block the engine finalized, waiting for enough signatures.
type pendingBlock struct {
	block  *cb.Block
	header []byte // BlockHeaderBytes(block.Header)
	obm    []byte // OrdererBlockMetadata, as signed
	sigs   map[uint64]*blockSig
}

// Chain implements consensus.Chain on top of the BiniBFT engine:
//
//	Order/Configure -> engine request pool -> primary assembles a block
//	-> pre-prepare/prepare/commit -> Deliver(proposal) on every node
//	-> every node signs the block and broadcasts the signature
//	-> a node writes the block once it holds a BFT quorum of signatures.
//
// Peers require a quorum of consenter signatures in the block metadata
// (protoutil.BlockSignatureVerifier); the engine's own votes are not in that format.
type Chain struct {
	channel string
	selfID  uint64
	support consensus.ConsenterSupport
	set     *consenterSet
	quorum  int
	logger  *flogging.FabricLogger

	engine *engine.Consensus
	net    *engineNetwork
	asm    *assembler

	mu        sync.Mutex // protects tip, pending, early, nextWrite
	tip       tipState
	pending   map[uint64]*pendingBlock
	early     map[uint64]map[uint64]*blockSig // signatures for blocks not delivered yet
	nextWrite uint64

	writeMu sync.Mutex // serialises ledger writes

	stateMu  sync.Mutex
	started  bool
	haltOnce sync.Once
	haltC    chan struct{}
	errorC   chan struct{}
}

var (
	_ consensus.Chain            = (*Chain)(nil)
	_ engine.ApplicationDelivery = (*Chain)(nil)
	_ MessageReceiver            = (*Chain)(nil)
)

func newChain(
	support consensus.ConsenterSupport,
	set *consenterSet,
	selfID uint64,
	comm cluster.Communicator,
	logger *flogging.FabricLogger,
) (*Chain, error) {
	c := &Chain{
		channel: support.ChannelID(),
		selfID:  selfID,
		support: support,
		set:     set,
		quorum:  bftQuorum(len(set.IDs)),
		logger:  logger,
		pending: make(map[uint64]*pendingBlock),
		early:   make(map[uint64]map[uint64]*blockSig),
		haltC:   make(chan struct{}),
		errorC:  make(chan struct{}),
	}

	tip, err := tipFromLedger(support)
	if err != nil {
		return nil, err
	}
	c.tip = tip
	c.nextWrite = tip.number + 1

	rpc := &cluster.RPC{
		Logger:        logger,
		Channel:       c.channel,
		StreamsByType: cluster.NewStreamsByType(),
		Comm:          comm,
		Timeout:       5 * time.Minute,
	}
	c.net = newEngineNetwork(selfID, rpc, logger)
	c.asm = &assembler{support: support, tip: c.tipSnapshot}

	role := engine.RoleShardLeader
	if selfID == set.primary() {
		role = engine.RolePrimaryLeader
	}

	batchSize := support.SharedConfig().BatchSize()
	builder := engine.NewConsensusBuilder().
		WithChannelID(c.channel).
		WithNode(nodeID(selfID), engine.ShardID(selfID), role).
		WithPrimaryLeader(nodeID(set.primary())).
		WithSigner(&engineSigner{id: selfID, support: support}).
		WithVerifier(&engineVerifier{channel: c.channel, support: support, set: set, chain: c}).
		WithRequestInspector(requestInspector{}).
		WitAssembler(c.asm).
		WithNetwork(c.net).
		WithLogger(engineLogger{c.logger}).
		WithBatchingConfig(int(batchSize.MaxMessageCount), support.SharedConfig().BatchTimeout()).
		// Block N is engine sequence N, so every consenter that is level with the
		// ledger starts at the same sequence after a restart.
		WithViewMetaData(&protos.ViewMetadata{LatestSequence: tip.number})
	// Every consenter is its own single-node shard: the engine's cross-shard
	// quorum then acts as one flat BFT group over all consenters.
	for _, id := range set.IDs {
		builder.WithShard(engine.ShardID(id), nodeID(id), nil)
	}

	eng, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, "building the BiniBFT engine")
	}
	eng.SetApplication(c)
	c.engine = eng
	return c, nil
}

func tipFromLedger(support consensus.ConsenterSupport) (tipState, error) {
	height := support.Height()
	if height == 0 {
		return tipState{}, errors.New("ledger is empty: the channel must be joined with a config block")
	}
	last := support.Block(height - 1)
	if last == nil {
		return tipState{}, errors.Errorf("cannot read the last block %d from the ledger", height-1)
	}
	var lastConfig uint64
	if last.Header.Number != 0 {
		idx, err := protoutil.GetLastConfigIndexFromBlock(last)
		if err != nil {
			return tipState{}, errors.Wrap(err, "reading the last config index")
		}
		lastConfig = idx
	}
	return tipState{
		number:     last.Header.Number,
		hash:       protoutil.BlockHeaderHash(last.Header),
		lastConfig: lastConfig,
	}, nil
}

func (c *Chain) tipSnapshot() tipState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tip
}

func (c *Chain) Order(env *cb.Envelope, configSeq uint64) error {
	if err := c.running(); err != nil {
		return err
	}
	if seq := c.support.Sequence(); configSeq < seq {
		c.logger.Debugf("Config sequence advanced (%d < %d), revalidating transaction", configSeq, seq)
		if _, err := c.support.ProcessNormalMsg(env); err != nil {
			return errors.Wrap(err, "bad normal message")
		}
	}
	return c.submit(env)
}

func (c *Chain) Configure(config *cb.Envelope, configSeq uint64) error {
	if err := c.running(); err != nil {
		return err
	}
	if seq := c.support.Sequence(); configSeq < seq {
		c.logger.Debugf("Config sequence advanced (%d < %d), revalidating config update", configSeq, seq)
		rebuilt, _, err := c.support.ProcessConfigMsg(config)
		if err != nil {
			return errors.Wrap(err, "bad config message")
		}
		config = rebuilt
	}
	return c.submit(config)
}

func (c *Chain) submit(env *cb.Envelope) error {
	raw, err := proto.Marshal(env)
	if err != nil {
		return errors.Wrap(err, "marshalling envelope")
	}
	err = c.engine.SubmitRequest(raw)
	// The gateway sends each tx to every orderer and needs a quorum of successes;
	// the primary also receives forwarded copies, so a duplicate is not an error.
	if errors.Is(err, engine.ErrReqAlreadyExists) || errors.Is(err, engine.ErrReqAlreadyProcessed) {
		return nil
	}
	return err
}

func (c *Chain) running() error {
	select {
	case <-c.haltC:
		return errors.New("chain is halted")
	default:
		return nil
	}
}

func (c *Chain) WaitReady() error { return c.running() }

func (c *Chain) Errored() <-chan struct{} { return c.errorC }

func (c *Chain) Start() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.started {
		return
	}
	c.started = true
	c.logger.Infof("Starting BiniBFT chain on channel %s: consenter %d of %v, primary %d, block quorum %d",
		c.channel, c.selfID, c.set.IDs, c.set.primary(), c.quorum)
	if err := c.engine.Start(context.Background()); err != nil {
		c.logger.Panicf("Failed starting the BiniBFT engine: %v", err)
	}
}

func (c *Chain) Halt() {
	c.haltOnce.Do(func() {
		c.logger.Infof("Halting BiniBFT chain on channel %s", c.channel)
		close(c.haltC)
		close(c.errorC)
		_ = c.engine.Stop()
		c.net.Stop()
	})
}

func (c *Chain) ReceiveHeartbeat(engine.NodeID, time.Time) {}

// Deliver is called by the engine once a proposal is finalized. The block is
// signed and its signature is broadcast; it is written to the ledger as soon as
// a quorum of consenter signatures for it has been collected.
func (c *Chain) Deliver(p engine.Proposal) error {
	block, err := blockFromProposal(p)
	if err != nil {
		return errors.Wrap(err, "finalized proposal is not a valid block")
	}
	sigMD, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return err
	}
	number := block.Header.Number
	header := protoutil.BlockHeaderBytes(block.Header)

	own, err := c.signBlock(number, header, sigMD.Value)
	if err != nil {
		return errors.Wrapf(err, "signing block %d", number)
	}

	c.mu.Lock()
	if number <= c.tip.number {
		c.mu.Unlock()
		return nil // already delivered
	}
	if number != c.tip.number+1 {
		c.mu.Unlock()
		return errors.Errorf("out-of-order delivery of block %d, expected %d", number, c.tip.number+1)
	}
	c.tip.number = number
	c.tip.hash = protoutil.BlockHeaderHash(block.Header)
	if protoutil.IsConfigBlock(block) {
		c.tip.lastConfig = number
	}

	pb := &pendingBlock{
		block:  block,
		header: header,
		obm:    sigMD.Value,
		sigs:   map[uint64]*blockSig{c.selfID: own},
	}
	for id, s := range c.early[number] {
		if _, have := pb.sigs[id]; !have {
			pb.sigs[id] = s
		}
	}
	delete(c.early, number)
	c.pending[number] = pb
	c.mu.Unlock()

	c.logger.Debugf("Finalized block %d (%d transactions), broadcasting signature", number, len(block.Data.Data))
	c.broadcastSig(own)
	c.tryCommit()
	c.flushCarry()
	return nil
}

func (c *Chain) signBlock(number uint64, header, obm []byte) (*blockSig, error) {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, err
	}
	idHeader := protoutil.MarshalOrPanic(&cb.IdentifierHeader{Identifier: uint32(c.selfID), Nonce: nonce})
	signature, err := c.support.Sign(util.ConcatenateBytes(obm, idHeader, header))
	if err != nil {
		return nil, err
	}
	return &blockSig{
		Number:               number,
		BlockHeader:          header,
		IdentifierHeader:     idHeader,
		OrdererBlockMetadata: obm,
		Signature:            signature,
	}, nil
}

func (c *Chain) broadcastSig(s *blockSig) {
	raw, err := encodeBlockSig(s)
	if err != nil {
		c.logger.Errorf("Cannot encode signature of block %d: %v", s.Number, err)
		return
	}
	for _, id := range c.set.IDs {
		if id == c.selfID {
			continue
		}
		if err := c.net.sendWire(id, raw); err != nil {
			c.logger.Warnf("Cannot send signature of block %d to consenter %d: %v", s.Number, id, err)
		}
	}
}

func (c *Chain) HandleMessage(sender uint64, payload []byte) {
	if c.running() != nil {
		return
	}
	if _, ok := c.set.ByID[sender]; !ok {
		c.logger.Warnf("Ignoring message from %d: not a consenter of channel %s", sender, c.channel)
		return
	}
	w, err := decodeWire(payload)
	if err != nil {
		c.logger.Warnf("Ignoring message from %d: %v", sender, err)
		return
	}
	switch w.Kind {
	case kindEngine:
		c.net.receive(sender, w.Engine)
	case kindBlockSig:
		c.onBlockSig(sender, w.BlockSig)
	}
}

// HandleRequest receives a transaction forwarded by another consenter.
func (c *Chain) HandleRequest(sender uint64, req []byte) {
	if c.running() != nil {
		return
	}
	if err := c.engine.SubmitRequest(req); err != nil {
		c.logger.Debugf("Forwarded request from %d not accepted: %v", sender, err)
	}
}

type asn1BlockHeader struct {
	Number       *big.Int
	PreviousHash []byte
	DataHash     []byte
}

func (c *Chain) onBlockSig(sender uint64, s *blockSig) {
	info := c.set.ByID[sender]

	// The signature must come from the consenter it claims to be from.
	idHeader := &cb.IdentifierHeader{}
	if err := proto.Unmarshal(s.IdentifierHeader, idHeader); err != nil || uint64(idHeader.Identifier) != sender {
		c.logger.Warnf("Dropping block signature from %d: bad identifier header", sender)
		return
	}
	hdr := &asn1BlockHeader{}
	if _, err := asn1.Unmarshal(s.BlockHeader, hdr); err != nil || hdr.Number == nil || !hdr.Number.IsUint64() || hdr.Number.Uint64() != s.Number {
		c.logger.Warnf("Dropping block signature from %d: header does not match block number %d", sender, s.Number)
		return
	}
	msg := util.ConcatenateBytes(s.OrdererBlockMetadata, s.IdentifierHeader, s.BlockHeader)
	if !verifyECDSA(info.PubKey, msg, s.Signature) {
		c.logger.Warnf("Dropping block signature from %d for block %d: invalid signature", sender, s.Number)
		return
	}

	c.mu.Lock()
	if s.Number < c.nextWrite {
		c.mu.Unlock()
		return // block already written
	}
	if pb, ok := c.pending[s.Number]; ok {
		if _, have := pb.sigs[sender]; !have {
			pb.sigs[sender] = s
		}
	} else {
		if s.Number > c.tip.number+maxEarlyWindow {
			c.mu.Unlock()
			return
		}
		m := c.early[s.Number]
		if m == nil {
			m = make(map[uint64]*blockSig)
			c.early[s.Number] = m
		}
		if _, have := m[sender]; !have {
			m[sender] = s
		}
	}
	c.mu.Unlock()

	c.tryCommit()
}

// maxEarlyWindow bounds how far ahead of the local tip signatures are buffered.
const maxEarlyWindow = 64

// matching returns the signatures that sign exactly this block, ordered by
// signer ID so that the metadata a node writes is deterministic.
func (pb *pendingBlock) matching() []*blockSig {
	ids := make([]uint64, 0, len(pb.sigs))
	for id := range pb.sigs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]*blockSig, 0, len(ids))
	for _, id := range ids {
		s := pb.sigs[id]
		if bytes.Equal(s.BlockHeader, pb.header) && bytes.Equal(s.OrdererBlockMetadata, pb.obm) {
			out = append(out, s)
		}
	}
	return out
}

// tryCommit writes, in order, every block that has gathered a quorum.
func (c *Chain) tryCommit() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	for {
		c.mu.Lock()
		pb := c.pending[c.nextWrite]
		var sigs []*blockSig
		if pb != nil {
			sigs = pb.matching()
			if len(sigs) < c.quorum {
				pb = nil
			}
		}
		if pb == nil {
			c.mu.Unlock()
			return
		}
		delete(c.pending, c.nextWrite)
		c.nextWrite++
		c.mu.Unlock()

		c.writeBlock(pb, sigs)
	}
}

func (c *Chain) writeBlock(pb *pendingBlock, sigs []*blockSig) {
	metadataSigs := make([]*cb.MetadataSignature, 0, len(sigs))
	for _, s := range sigs {
		// The signature header is left empty on purpose: peers rebuild the signer
		// from the identifier header and the channel's consenter set.
		metadataSigs = append(metadataSigs, &cb.MetadataSignature{
			Signature:        s.Signature,
			IdentifierHeader: s.IdentifierHeader,
		})
	}
	block := pb.block
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value:      pb.obm,
		Signatures: metadataSigs,
	})

	if protoutil.IsConfigBlock(block) {
		c.logger.Infof("Writing config block %d with %d consenter signatures", block.Header.Number, len(sigs))
		c.support.WriteConfigBlock(block, nil)
		return
	}
	c.logger.Debugf("Writing block %d (%d transactions) with %d consenter signatures",
		block.Header.Number, len(block.Data.Data), len(sigs))
	c.support.WriteBlock(block, nil)
}

// flushCarry proposes requests the assembler held back (a config transaction
// that could not share a block). Only the primary proposes.
func (c *Chain) flushCarry() {
	if !c.asm.hasCarry() || c.set.primary() != c.selfID {
		return
	}
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-c.haltC:
				return
			case <-time.After(50 * time.Millisecond):
			}
			view := c.engine.GetCurrentView()
			if view == nil || view.IsProposalInProgress() {
				continue
			}
			prop := c.asm.AssembleProposal(view.GetMetadata(), nil)
			if len(prop.Header) == 0 {
				return
			}
			if err := view.Propose(prop); err != nil {
				c.logger.Warnf("Proposing carried-over requests failed: %v", err)
			}
			return
		}
	}()
}
