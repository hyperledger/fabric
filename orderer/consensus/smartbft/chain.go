/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"

	mspi "github.com/hyperledger/fabric/msp"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	smartbft "github.com/SmartBFT-Go/consensus/pkg/consensus"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/pkg/wal"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	types2 "github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

//go:generate mockery --dir . --name Communicator --case underscore --with-expecter=true --output mocks

// Communicator defines communication for a consenter
type Communicator interface {
	// Remote returns a RemoteContext for the given RemoteNode ID in the context
	// of the given channel, or error if connection cannot be established, or
	// the channel wasn't configured
	Remote(channel string, id uint64) (*cluster.RemoteContext, error)
	// Configure configures the communication to connect to all
	// given members, and disconnect from any members not among the given
	// members.
	Configure(channel string, members []cluster.RemoteNode)
	// Shutdown shuts down the communicator
	Shutdown()
}

//go:generate mockery --dir . --name Policy --case underscore --with-expecter=true --output mocks

// Policy is used to determine if a signature is valid
type Policy interface {
	// EvaluateSignedData takes a set of SignedData and evaluates whether
	// 1) the signatures are valid over the related message
	// 2) the signing identities satisfy the policy
	EvaluateSignedData(signatureSet []*protoutil.SignedData) error

	// EvaluateIdentities takes an array of identities and evaluates whether
	// they satisfy the policy
	EvaluateIdentities(identities []mspi.Identity) error
}

//go:generate mockery --dir . --name PolicyManager --case underscore --with-expecter=true --output mocks

// PolicyManager is a read only subset of the policy ManagerImpl
type PolicyManager interface {
	// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
	GetPolicy(id string) (policies.Policy, bool)

	// Manager returns the sub-policy manager for a given path and whether it exists
	Manager(path []string) (policies.Manager, bool)
}

//go:generate mockery --dir . --name Processor --case underscore --with-expecter=true --output mocks

// Processor provides the methods necessary to classify and process any message which
// arrives through the Broadcast interface.
type Processor interface {
	// ClassifyMsg inspects the message header to determine which type of processing is necessary
	ClassifyMsg(chdr *cb.ChannelHeader) msgprocessor.Classification

	// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
	// configuration sequence number and nil on success, or an error if the message is not valid
	ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error)

	// ProcessConfigUpdateMsg will attempt to apply the config update to the current configuration, and if successful
	// return the resulting config message and the configSeq the config was computed from.  If the config update message
	// is invalid, an error is returned.
	ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error)

	// ProcessConfigMsg takes message of type `ORDERER_TX` or `CONFIG`, unpack the ConfigUpdate envelope embedded
	// in it, and call `ProcessConfigUpdateMsg` to produce new Config message of the same type as original message.
	// This method is used to re-validate and reproduce config message, if it's deemed not to be valid anymore.
	ProcessConfigMsg(env *cb.Envelope) (*cb.Envelope, uint64, error)
}

//go:generate mockery --dir . --name BlockPuller --case underscore --with-expecter=true --output mocks

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *cb.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

// WALConfig consensus specific configuration parameters from orderer.yaml; for SmartBFT only WALDir is relevant.
type WALConfig struct {
	WALDir            string // WAL data of <my-channel> is stored in WALDir/<my-channel>
	SnapDir           string // Snapshots of <my-channel> are stored in SnapDir/<my-channel>
	EvictionSuspicion string // Duration threshold that the node samples in order to suspect its eviction from the channel.
}

//go:generate mockery --dir . --name ConfigValidator --case underscore --with-expecter=true --output mocks

// ConfigValidator interface
type ConfigValidator interface {
	ValidateConfig(env *cb.Envelope) error
}

// TODO why do we need this interface? why not use smartbft.SignerSerializer?
type signerSerializer interface {
	// Sign a message and return the signature over the digest, or error on failure
	Sign(message []byte) ([]byte, error)

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)
}

// BFTChain implements Chain interface to wire with
// BFT smart library
type BFTChain struct {
	RuntimeConfigManager RuntimeConfigManager
	Channel              string
	Config               types.Configuration
	BlockPuller          BlockPuller
	Comm                 Communicator
	SignerSerializer     signerSerializer
	PolicyManager        policies.Manager
	Logger               *flogging.FabricLogger
	WALDir               string
	consensus            *smartbft.Consensus
	processor            msgprocessor.Processor
	ledger               Ledger
	sequencer            Sequencer
	clusterService       *cluster.ClusterService
	verifier             *Verifier
	assembler            *Assembler
	Metrics              *Metrics
	MetricsBFT           *api.Metrics
	MetricsWalBFT        *wal.Metrics
	bccsp                bccsp.BCCSP

	statusReportMutex sync.Mutex
	consensusRelation types2.ConsensusRelation
	status            types2.Status
}

// NewChain creates new BFT Smart chain
func NewChain(
	channelID string,
	cv ConfigValidator,
	selfID uint64,
	config types.Configuration,
	walDir string,
	blockPuller BlockPuller,
	comm Communicator,
	signerSerializer signerSerializer,
	policyManager PolicyManager,
	metrics *Metrics,
	metricsBFT *api.Metrics,
	metricsWalBFT *wal.Metrics,
	bccsp bccsp.BCCSP,
	runtimeConfigManagerFactory RuntimeConfigManagerFactory,
	processor Processor,
	ledger Ledger,
	sequencer Sequencer,
	egressCommFactory EgressCommFactory,
) (*BFTChain, error) {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft.chain").With(zap.String("channel", channelID))

	requestInspector := &RequestInspector{
		ValidateIdentityStructure: func(_ *msp.SerializedIdentity) error {
			return nil
		},
		Logger: logger,
	}

	rtc := RuntimeConfig{
		logger: logger,
		id:     selfID,
	}

	c := &BFTChain{
		RuntimeConfigManager: runtimeConfigManagerFactory(rtc),
		Channel:              channelID,
		Config:               config,
		WALDir:               walDir,
		Comm:                 comm,
		processor:            processor,
		ledger:               ledger,
		sequencer:            sequencer,
		SignerSerializer:     signerSerializer,
		PolicyManager:        policyManager,
		BlockPuller:          blockPuller,
		Logger:               logger,
		consensusRelation:    types2.ConsensusRelationConsenter,
		status:               types2.StatusActive,
		Metrics: &Metrics{
			ClusterSize:          metrics.ClusterSize.With("channel", channelID),
			CommittedBlockNumber: metrics.CommittedBlockNumber.With("channel", channelID),
			IsLeader:             metrics.IsLeader.With("channel", channelID),
			LeaderID:             metrics.LeaderID.With("channel", channelID),
		},
		MetricsBFT:    metricsBFT.With("channel", channelID),
		MetricsWalBFT: metricsWalBFT.With("channel", channelID),
		bccsp:         bccsp,
	}

	lastBlock := LastBlockFromLedgerOrPanic(ledger, c.Logger)
	lastConfigBlock := LastConfigBlockFromLedgerOrPanic(ledger, c.Logger)

	rtc, err := c.RuntimeConfigManager.UpdateUsingBlock(lastConfigBlock, bccsp)
	if err != nil {
		return nil, err
	}
	// why do we need to update the last block? isn't the config enough?
	rtc, err = c.RuntimeConfigManager.UpdateUsingBlock(lastBlock, bccsp)
	if err != nil {
		return nil, err
	}

	c.verifier = buildVerifier(cv, c.RuntimeConfigManager, channelID, ledger, sequencer, requestInspector, policyManager)
	c.consensus = bftSmartConsensusBuild(c, requestInspector, egressCommFactory)

	// Setup communication with list of remotes notes for the new channel
	c.Comm.Configure(channelID, rtc.RemoteNodes)

	if err = c.consensus.ValidateConfiguration(rtc.Nodes); err != nil {
		return nil, errors.Wrap(err, "failed to verify SmartBFT-Go configuration")
	}

	logger.Infof("SmartBFT-v3 is now servicing chain %s", channelID)

	return c, nil
}

func bftSmartConsensusBuild(
	c *BFTChain,
	requestInspector *RequestInspector,
	egressCommFactory EgressCommFactory,
) *smartbft.Consensus {
	var err error

	rtc := c.RuntimeConfigManager.GetConfig()

	latestMetadata, err := getViewMetadataFromBlock(rtc.LastBlock)
	if err != nil {
		c.Logger.Panicf("Failed extracting view metadata from ledger: %v", err)
	}

	var consensusWAL *wal.WriteAheadLogFile
	var walInitState [][]byte

	c.Logger.Infof("Initializing a WAL for chain %s, on dir: %s", c.Channel, c.WALDir)
	opt := wal.DefaultOptions()
	opt.Metrics = c.MetricsWalBFT
	consensusWAL, walInitState, err = wal.InitializeAndReadAll(c.Logger, c.WALDir, opt)
	if err != nil {
		c.Logger.Panicf("failed to initialize a WAL for chain %s, err %s", c.Channel, err)
	}

	clusterSize := uint64(len(rtc.Nodes))

	// report cluster size
	c.Metrics.ClusterSize.Set(float64(clusterSize))

	sync := &Synchronizer{
		selfID:          rtc.id,
		BlockToDecision: c.blockToDecision,
		OnCommit: func(block *cb.Block) types.Reconfig {
			c.pruneCommittedRequests(block)
			return c.updateRuntimeConfig(block)
		},
		Sequencer:   c.sequencer,
		Ledger:      c.ledger,
		BlockPuller: c.BlockPuller,
		ClusterSize: clusterSize,
		Logger:      c.Logger,
		LatestConfig: func() (types.Configuration, []uint64) {
			rtc := c.RuntimeConfigManager.GetConfig()
			return rtc.BFTConfig, rtc.Nodes
		},
	}

	channelDecorator := zap.String("channel", c.Channel)
	logger := flogging.MustGetLogger("orderer.consensus.smartbft.consensus").With(channelDecorator)

	c.assembler = &Assembler{
		RuntimeConfigManager: c.RuntimeConfigManager,
		VerificationSeq:      c.verifier.VerificationSequence,
		Logger:               flogging.MustGetLogger("orderer.consensus.smartbft.assembler").With(channelDecorator),
	}

	consensus := &smartbft.Consensus{
		Config:   c.Config,
		Logger:   logger,
		Verifier: c.verifier,
		Signer: &Signer{
			ID:               c.Config.SelfID,
			Logger:           flogging.MustGetLogger("orderer.consensus.smartbft.signer").With(channelDecorator),
			SignerSerializer: c.SignerSerializer,
			LastConfigBlockNum: func(block *cb.Block) uint64 {
				if protoutil.IsConfigBlock(block) {
					return block.Header.Number
				}

				return c.RuntimeConfigManager.GetConfig().LastConfigBlock.Header.Number
			},
		},
		Metrics: c.MetricsBFT,
		Metadata: &smartbftprotos.ViewMetadata{
			ViewId:                    latestMetadata.ViewId,
			LatestSequence:            latestMetadata.LatestSequence,
			DecisionsInView:           latestMetadata.DecisionsInView,
			BlackList:                 latestMetadata.BlackList,
			PrevCommitSignatureDigest: latestMetadata.PrevCommitSignatureDigest,
		},
		WAL:               consensusWAL,
		WALInitialContent: walInitState, // Read from WAL entries
		Application:       c,
		Assembler:         c.assembler,
		RequestInspector:  requestInspector,
		Synchronizer:      sync,
		Comm:              egressCommFactory(c.RuntimeConfigManager, c.Channel, c.Comm),
		Scheduler:         time.NewTicker(time.Second).C,
		ViewChangerTicker: time.NewTicker(time.Second).C,
	}

	proposal, signatures := c.lastPersistedProposalAndSignatures()
	if proposal != nil {
		consensus.LastProposal = *proposal
		consensus.LastSignatures = signatures
	}

	return consensus
}

func (c *BFTChain) pruneCommittedRequests(block *cb.Block) {
	workerNum := runtime.NumCPU()

	var workers []*worker

	for i := 0; i < workerNum; i++ {
		workers = append(workers, &worker{
			id:        i,
			work:      block.Data.Data,
			workerNum: workerNum,
			f: func(tx []byte) {
				ri := c.verifier.ReqInspector.RequestID(tx)
				c.consensus.Pool.RemoveRequest(ri)
			},
		})
	}

	var wg sync.WaitGroup
	wg.Add(len(workers))

	for i := 0; i < len(workers); i++ {
		go func(w *worker) {
			defer wg.Done()
			w.doWork()
		}(workers[i])
	}

	wg.Wait()
}

func (c *BFTChain) pruneBadRequests() {
	c.consensus.Pool.Prune(func(req []byte) error {
		_, err := c.consensus.Verifier.VerifyRequest(req)
		return err
	})
}

func (c *BFTChain) submit(env *cb.Envelope, configSeq uint64) error {
	reqBytes, err := proto.Marshal(env)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal request envelope")
	}

	c.Logger.Debugf("Consensus.SubmitRequest, node id %d", c.Config.SelfID)
	if err = c.consensus.SubmitRequest(reqBytes); err != nil {
		return errors.Wrapf(err, "failed to submit request")
	}
	return nil
}

// Order accepts a message which has been processed at a given configSeq.
// If the configSeq advances, it is the responsibility of the consenter
// to revalidate and potentially discard the message
// The consenter may return an error, indicating the message was not accepted
func (c *BFTChain) Order(env *cb.Envelope, configSeq uint64) error {
	seq := c.sequencer.Sequence()
	if configSeq < seq {
		c.Logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", configSeq, seq)
		if _, err := c.processor.ProcessNormalMsg(env); err != nil {
			return errors.Errorf("bad normal message: %s", err)
		}
	}

	return c.submit(env, configSeq)
}

// Configure accepts a message which reconfigures the channel and will
// trigger an update to the configSeq if committed.  The configuration must have
// been triggered by a ConfigUpdate message. If the config sequence advances,
// it is the responsibility of the consenter to recompute the resulting config,
// discarding the message if the reconfiguration is no longer valid.
// The consenter may return an error, indicating the message was not accepted
func (c *BFTChain) Configure(config *cb.Envelope, configSeq uint64) error {
	if err := c.verifier.ConfigValidator.ValidateConfig(config); err != nil {
		return err
	}
	seq := c.sequencer.Sequence()
	if configSeq < seq {
		c.Logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", configSeq, seq)
		if configEnv, _, err := c.processor.ProcessConfigMsg(config); err != nil {
			return errors.Errorf("bad normal message: %s", err)
		} else {
			return c.submit(configEnv, configSeq)
		}
	}
	return c.submit(config, configSeq)
}

// Deliver delivers proposal, writes block with transactions and metadata
func (c *BFTChain) Deliver(proposal types.Proposal, signatures []types.Signature) types.Reconfig {
	block, err := ProposalToBlock(proposal)
	if err != nil {
		c.Logger.Panicf("failed to read proposal, err: %s", err)
	}

	var sigs []*cb.MetadataSignature
	var ordererBlockMetadata []byte

	var signers []uint64

	for _, s := range signatures {
		sig := &Signature{}
		if err = sig.Unmarshal(s.Msg); err != nil {
			c.Logger.Errorf("Failed unmarshaling signature from %d: %v", s.ID, err)
			c.Logger.Errorf("Offending signature Msg: %s", base64.StdEncoding.EncodeToString(s.Msg))
			c.Logger.Errorf("Offending signature Value: %s", base64.StdEncoding.EncodeToString(s.Value))
			c.Logger.Errorf("Halting chain.")
			c.Halt()
			return types.Reconfig{}
		}

		if ordererBlockMetadata == nil {
			ordererBlockMetadata = sig.OrdererBlockMetadata
		}

		sigs = append(sigs, &cb.MetadataSignature{
			//	AuxiliaryInput: sig.AuxiliaryInput,
			Signature: s.Value,
			// We do not put a signature header when we commit the block.
			// Instead, we put the nonce and the identifier and at validation
			// we reconstruct the signature header at runtime.
			// SignatureHeader: sig.SignatureHeader,
			//	Nonce:    sig.Nonce,
			//	SignerId: s.ID,
			IdentifierHeader: sig.IdentifierHeader,
		})

		signers = append(signers, s.ID)
	}

	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value:      ordererBlockMetadata,
		Signatures: sigs,
	})

	var mdTotalSize int
	for _, md := range block.Metadata.Metadata {
		mdTotalSize += len(md)
	}

	c.Logger.Infof("Delivering proposal, writing block %d with %d transactions and metadata of total size %d with signatures from %v to the ledger, node id %d",
		block.Header.Number,
		len(block.Data.Data),
		mdTotalSize,
		signers,
		c.Config.SelfID)
	c.Metrics.CommittedBlockNumber.Set(float64(block.Header.Number)) // report the committed block number
	c.reportIsLeader()                                               // report the leader
	if protoutil.IsConfigBlock(block) {
		c.ledger.WriteConfigBlock(block, nil)
	} else {
		c.ledger.WriteBlock(block, nil)
	}

	reconfig := c.updateRuntimeConfig(block)
	return reconfig
}

// WaitReady blocks waiting for consenter to be ready for accepting new messages.
// This is useful when consenter needs to temporarily block ingress messages so
// that in-flight messages can be consumed. It could return error if consenter is
// in erroneous states. If this blocking behavior is not desired, consenter could
// simply return nil.
func (c *BFTChain) WaitReady() error {
	return nil
}

// Errored returns a channel which will close when an error has occurred.
// This is especially useful for the Deliver client, who must terminate waiting
// clients when the consenter is not up to date.
func (c *BFTChain) Errored() <-chan struct{} {
	// TODO: Implement Errored
	return nil
}

// Start should allocate whatever resources are needed for staying up to date with the chain.
// Typically, this involves creating a thread which reads from the ordering source, passes those
// messages to a block cutter, and writes the resulting blocks to the ledger.
func (c *BFTChain) Start() {
	if err := c.consensus.Start(); err != nil {
		c.Logger.Panicf("Failed to start chain, aborting: %+v", err)
	}
	c.reportIsLeader() // report the leader
}

// Halt frees the resources which were allocated for this Chain.
func (c *BFTChain) Halt() {
	c.Logger.Infof("Shutting down chain")
	c.consensus.Stop()
}

func (c *BFTChain) blockToProposalWithoutSignaturesInMetadata(block *cb.Block) types.Proposal {
	blockClone := proto.Clone(block).(*cb.Block)
	if len(blockClone.Metadata.Metadata) > int(cb.BlockMetadataIndex_SIGNATURES) {
		signatureMetadata := &cb.Metadata{}
		// Nil out signatures because we carry them around separately in the library format.
		if err := proto.Unmarshal(blockClone.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], signatureMetadata); err != nil {
			// nothing to do
			c.Logger.Errorf("Error unmarshalling signature metadata from block: %s", err)
		}
		signatureMetadata.Signatures = nil
		blockClone.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(signatureMetadata)
	}
	prop := types.Proposal{
		Header: protoutil.BlockHeaderBytes(blockClone.Header),
		Payload: (&ByteBufferTuple{
			A: protoutil.MarshalOrPanic(blockClone.Data),
			B: protoutil.MarshalOrPanic(blockClone.Metadata),
		}).ToBytes(),
		VerificationSequence: int64(c.verifier.VerificationSequence()),
	}

	if protoutil.IsConfigBlock(block) {
		prop.VerificationSequence--
	}

	return prop
}

func (c *BFTChain) blockToDecision(block *cb.Block) *types.Decision {
	proposal := c.blockToProposalWithoutSignaturesInMetadata(block)
	if block.Header.Number == 0 {
		return &types.Decision{
			Proposal: proposal,
		}
	}

	signatureMetadata := &cb.Metadata{}
	if err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], signatureMetadata); err != nil {
		c.Logger.Panicf("Failed unmarshaling signatures from block metadata: %v", err)
	}

	ordererMDFromBlock := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(signatureMetadata.Value, ordererMDFromBlock); err != nil {
		c.Logger.Panicf("Failed unmarshaling OrdererBlockMetadata from block signature metadata: %v", err)
	}

	proposal.Metadata = ordererMDFromBlock.ConsenterMetadata

	var signatures []types.Signature
	for _, sigMD := range signatureMetadata.Signatures {
		idHdr := &cb.IdentifierHeader{}
		if err := proto.Unmarshal(sigMD.IdentifierHeader, idHdr); err != nil {
			c.Logger.Panicf("Failed unmarshaling identifier header for %s", base64.StdEncoding.EncodeToString(sigMD.IdentifierHeader))
		}
		sig := &Signature{
			IdentifierHeader:     sigMD.IdentifierHeader,
			BlockHeader:          protoutil.BlockHeaderBytes(block.Header),
			OrdererBlockMetadata: signatureMetadata.Value,
		}
		signatures = append(signatures, types.Signature{
			Msg:   sig.Marshal(),
			Value: sigMD.Signature,
			ID:    uint64(idHdr.Identifier),
		})
	}

	return &types.Decision{
		Signatures: signatures,
		Proposal:   proposal,
	}
}

// HandleMessage handles the message from the sender
func (c *BFTChain) HandleMessage(sender uint64, m *smartbftprotos.Message) {
	c.Logger.Debugf("Message from %d", sender)
	c.consensus.HandleMessage(sender, m)
}

// HandleRequest handles the request from the sender
func (c *BFTChain) HandleRequest(sender uint64, req []byte) {
	c.Logger.Debugf("HandleRequest from %d", sender)
	if _, err := c.verifier.VerifyRequest(req); err != nil {
		c.Logger.Warnf("Got bad request from %d: %v", sender, err)
		return
	}
	c.consensus.SubmitRequest(req)
}

func (c *BFTChain) updateRuntimeConfig(block *cb.Block) types.Reconfig {
	prevRTC := c.RuntimeConfigManager.GetConfig()
	newRTC, err := c.RuntimeConfigManager.UpdateUsingBlock(block, c.bccsp)
	if err != nil {
		c.Logger.Errorf("Failed constructing RuntimeConfig from block %d, halting chain", block.Header.Number)
		c.Halt()
		return types.Reconfig{}
	}
	if protoutil.IsConfigBlock(block) {
		c.Comm.Configure(c.Channel, newRTC.RemoteNodes)
		c.clusterService.ConfigureNodeCerts(c.Channel, newRTC.consenters)
		c.pruneBadRequests()
	}

	membershipDidNotChange := reflect.DeepEqual(newRTC.Nodes, prevRTC.Nodes)
	configDidNotChange := reflect.DeepEqual(newRTC.BFTConfig, prevRTC.BFTConfig)
	noChangeDetected := membershipDidNotChange && configDidNotChange
	return types.Reconfig{
		InLatestDecision: !noChangeDetected,
		CurrentNodes:     newRTC.Nodes,
		CurrentConfig:    newRTC.BFTConfig,
	}
}

func (c *BFTChain) lastPersistedProposalAndSignatures() (*types.Proposal, []types.Signature) {
	lastBlock := LastBlockFromLedgerOrPanic(c.ledger, c.Logger)
	// initial report of the last committed block number
	c.Metrics.CommittedBlockNumber.Set(float64(lastBlock.Header.Number))
	decision := c.blockToDecision(lastBlock)
	return &decision.Proposal, decision.Signatures
}

func (c *BFTChain) GetLeaderID() uint64 {
	return c.consensus.GetLeaderID()
}

func (c *BFTChain) reportIsLeader() {
	leaderID := c.GetLeaderID()
	c.Metrics.LeaderID.Set(float64(leaderID))

	if leaderID == c.Config.SelfID {
		c.Metrics.IsLeader.Set(1)
	} else {
		c.Metrics.IsLeader.Set(0)
	}
}

// StatusReport returns the ConsensusRelation & Status
func (c *BFTChain) StatusReport() (types2.ConsensusRelation, types2.Status) {
	c.statusReportMutex.Lock()
	defer c.statusReportMutex.Unlock()

	return c.consensusRelation, c.status
}

func buildVerifier(
	cv ConfigValidator,
	runtimeConfigManager RuntimeConfigManager,
	channelId string,
	ledgerReader LedgerReader,
	sequencer Sequencer,
	requestInspector *RequestInspector,
	policyManager PolicyManager,
) *Verifier {
	channelDecorator := zap.String("channel", channelId)
	logger := flogging.MustGetLogger("orderer.consensus.smartbft.verifier").With(channelDecorator)
	return &Verifier{
		Channel:               channelId,
		ConfigValidator:       cv,
		VerificationSequencer: sequencer,
		ReqInspector:          requestInspector,
		Logger:                logger,
		RuntimeConfigManager:  runtimeConfigManager,
		ConsenterVerifier: &consenterVerifier{
			logger:        logger,
			channel:       channelId,
			policyManager: policyManager,
		},

		AccessController: &chainACL{
			policyManager: policyManager,
			Logger:        logger,
		},
		Ledger: ledgerReader,
	}
}

type chainACL struct {
	policyManager policies.Manager
	Logger        *flogging.FabricLogger
}

// Evaluate evaluates signed data
func (c *chainACL) Evaluate(signatureSet []*protoutil.SignedData) error {
	policy, ok := c.policyManager.GetPolicy(policies.ChannelWriters)
	if !ok {
		return fmt.Errorf("could not find policy %s", policies.ChannelWriters)
	}

	err := policy.EvaluateSignedData(signatureSet)
	if err != nil {
		c.Logger.Debugf("SigFilter evaluation failed: %s, policyName: %s", err.Error(), policies.ChannelWriters)
		return errors.Wrap(errors.WithStack(msgprocessor.ErrPermissionDenied), err.Error())
	}
	return nil
}
