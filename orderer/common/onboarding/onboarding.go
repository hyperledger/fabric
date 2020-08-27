/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package onboarding

import (
	"os"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	DefaultReplicationBackgroundRefreshInterval = time.Minute * 5
	replicationBackgroundInitialRefreshInterval = time.Second * 10
)

var logger = flogging.MustGetLogger("orderer.common.onboarding")

type ReplicationInitiator struct {
	RegisterChain func(chain string)
	ChannelLister cluster.ChannelLister

	verifierRetriever cluster.VerifierRetriever
	logger            *flogging.FabricLogger
	secOpts           comm.SecureOptions
	conf              *localconfig.TopLevel
	lf                cluster.LedgerFactory
	signer            identity.SignerSerializer
	cryptoProvider    bccsp.BCCSP
}

func NewReplicationInitiator(
	lf blockledger.Factory,
	bootstrapBlock *common.Block,
	conf *localconfig.TopLevel,
	secOpts comm.SecureOptions,
	signer identity.SignerSerializer,
	bccsp bccsp.BCCSP,
) *ReplicationInitiator {
	logger := flogging.MustGetLogger("orderer.common.cluster")

	vl := &verifierLoader{
		verifierFactory: &cluster.BlockVerifierAssembler{Logger: logger, BCCSP: bccsp},
		onFailure: func(block *common.Block) {
			protolator.DeepMarshalJSON(os.Stdout, block)
		},
		ledgerFactory: lf,
		logger:        logger,
	}

	systemChannelName, err := protoutil.GetChannelIDFromBlock(bootstrapBlock)
	if err != nil {
		logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}

	// System channel is not verified because we trust the bootstrap block
	// and use backward hash chain verification.
	verifiersByChannel := vl.loadVerifiers()
	verifiersByChannel[systemChannelName] = &cluster.NoopBlockVerifier{}

	vr := &cluster.VerificationRegistry{
		LoadVerifier:       vl.loadVerifier,
		Logger:             logger,
		VerifiersByChannel: verifiersByChannel,
		VerifierFactory:    &cluster.BlockVerifierAssembler{Logger: logger, BCCSP: bccsp},
	}

	ledgerFactory := &ledgerFactory{
		Factory:       lf,
		onBlockCommit: vr.BlockCommitted,
	}
	return &ReplicationInitiator{
		RegisterChain:     vr.RegisterVerifier,
		verifierRetriever: vr,
		logger:            logger,
		secOpts:           secOpts,
		conf:              conf,
		lf:                ledgerFactory,
		signer:            signer,
		cryptoProvider:    bccsp,
	}
}

func (ri *ReplicationInitiator) ReplicateIfNeeded(bootstrapBlock *common.Block) {
	if bootstrapBlock.Header.Number == 0 {
		ri.logger.Debug("Booted with a genesis block, replication isn't an option")
		return
	}
	ri.replicateNeededChannels(bootstrapBlock)
}

func (ri *ReplicationInitiator) createReplicator(bootstrapBlock *common.Block, filter func(string) bool) *cluster.Replicator {
	consenterCert := &etcdraft.ConsenterCertificate{
		Logger:               ri.logger,
		ConsenterCertificate: ri.secOpts.Certificate,
		CryptoProvider:       ri.cryptoProvider,
	}

	systemChannelName, err := protoutil.GetChannelIDFromBlock(bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}
	pullerConfig := cluster.PullerConfigFromTopLevelConfig(systemChannelName, ri.conf, ri.secOpts.Key, ri.secOpts.Certificate, ri.signer)
	puller, err := cluster.BlockPullerFromConfigBlock(pullerConfig, bootstrapBlock, ri.verifierRetriever, ri.cryptoProvider)
	if err != nil {
		ri.logger.Panicf("Failed creating puller config from bootstrap block: %v", err)
	}
	puller.MaxPullBlockRetries = uint64(ri.conf.General.Cluster.ReplicationMaxRetries)
	puller.RetryTimeout = ri.conf.General.Cluster.ReplicationRetryTimeout

	replicator := &cluster.Replicator{
		Filter:           filter,
		LedgerFactory:    ri.lf,
		SystemChannel:    systemChannelName,
		BootBlock:        bootstrapBlock,
		Logger:           ri.logger,
		AmIPartOfChannel: consenterCert.IsConsenterOfChannel,
		Puller:           puller,
		ChannelLister: &cluster.ChainInspector{
			Logger:          ri.logger,
			Puller:          puller,
			LastConfigBlock: bootstrapBlock,
		},
	}

	// If a custom channel lister is requested, use it
	if ri.ChannelLister != nil {
		replicator.ChannelLister = ri.ChannelLister
	}

	return replicator
}

func (ri *ReplicationInitiator) replicateNeededChannels(bootstrapBlock *common.Block) {
	replicator := ri.createReplicator(bootstrapBlock, cluster.AnyChannel)
	defer replicator.Puller.Close()
	replicationNeeded, err := replicator.IsReplicationNeeded()
	if err != nil {
		ri.logger.Panicf("Failed determining whether replication is needed: %v", err)
	}

	if !replicationNeeded {
		ri.logger.Info("Replication isn't needed")
		return
	}

	ri.logger.Info("Will now replicate chains")
	replicator.ReplicateChains()
}

// ReplicateChains replicates the given chains with the assistance of the given last system channel config block,
// and returns the names of the chains that were successfully replicated.
func (ri *ReplicationInitiator) ReplicateChains(lastConfigBlock *common.Block, chains []string) []string {
	ri.logger.Info("Will now replicate chains", chains)
	wantedChannels := make(map[string]struct{})
	for _, chain := range chains {
		wantedChannels[chain] = struct{}{}
	}
	filter := func(channelName string) bool {
		_, exists := wantedChannels[channelName]
		return exists
	}
	replicator := ri.createReplicator(lastConfigBlock, filter)
	replicator.DoNotPanicIfClusterNotReachable = true
	defer replicator.Puller.Close()
	return replicator.ReplicateChains()
}

type ledgerFactory struct {
	blockledger.Factory
	onBlockCommit cluster.BlockCommitFunc
}

func (lf *ledgerFactory) GetOrCreate(chainID string) (cluster.LedgerWriter, error) {
	ledger, err := lf.Factory.GetOrCreate(chainID)
	if err != nil {
		return nil, err
	}
	interceptedLedger := &cluster.LedgerInterceptor{
		LedgerWriter:         ledger,
		Channel:              chainID,
		InterceptBlockCommit: lf.onBlockCommit,
	}
	return interceptedLedger, nil
}

//go:generate mockery -dir . -name ChainReplicator -case underscore -output mocks

// ChainReplicator replicates chains
type ChainReplicator interface {
	// ReplicateChains replicates the given chains using the given last system channel config block.
	// It returns the names of the chains that were successfully replicated.
	ReplicateChains(lastConfigBlock *common.Block, chains []string) []string
}

// InactiveChainReplicator tracks disabled chains and replicates them upon demand
type InactiveChainReplicator struct {
	registerChain                     func(chain string)
	logger                            *flogging.FabricLogger
	retrieveLastSysChannelConfigBlock func() *common.Block
	replicator                        ChainReplicator
	scheduleChan                      <-chan time.Time
	quitChan                          chan struct{}
	doneChan                          chan struct{}
	lock                              sync.RWMutex
	chains2CreationCallbacks          map[string]chainCreation
}

func NewInactiveChainReplicator(
	chainReplicator ChainReplicator,
	getSysChannelConfigBlockFunc func() *common.Block,
	registerChainFunc func(chain string),
	replicationRefreshInterval time.Duration,
) *InactiveChainReplicator {
	if replicationRefreshInterval == 0 {
		replicationRefreshInterval = DefaultReplicationBackgroundRefreshInterval
	}
	exponentialSleep := exponentialDurationSeries(replicationBackgroundInitialRefreshInterval, replicationRefreshInterval)
	ticker := newTicker(exponentialSleep)

	icr := &InactiveChainReplicator{
		logger:                            logger,
		scheduleChan:                      ticker.C,
		quitChan:                          make(chan struct{}),
		doneChan:                          make(chan struct{}),
		replicator:                        chainReplicator,
		chains2CreationCallbacks:          make(map[string]chainCreation),
		retrieveLastSysChannelConfigBlock: getSysChannelConfigBlockFunc,
		registerChain:                     registerChainFunc,
	}

	return icr
}

func (i *InactiveChainReplicator) Channels() []cluster.ChannelGenesisBlock {
	i.lock.RLock()
	defer i.lock.RUnlock()

	var res []cluster.ChannelGenesisBlock
	for name, chain := range i.chains2CreationCallbacks {
		res = append(res, cluster.ChannelGenesisBlock{
			ChannelName:  name,
			GenesisBlock: chain.genesisBlock,
		})
	}
	return res
}

func (i *InactiveChainReplicator) Close() {}

type chainCreation struct {
	create       func()
	genesisBlock *common.Block
}

// TrackChain tracks a chain with the given name, and calls the given callback
// when this chain should be activated.
func (i *InactiveChainReplicator) TrackChain(chain string, genesisBlock *common.Block, createChainCallback func()) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.logger.Infof("Adding %s to the set of chains to track", chain)
	i.chains2CreationCallbacks[chain] = chainCreation{
		genesisBlock: genesisBlock,
		create:       createChainCallback,
	}
}

func (i *InactiveChainReplicator) Run() {
	for {
		select {
		case <-i.scheduleChan:
			i.replicateDisabledChains()
		case <-i.quitChan:
			close(i.doneChan)
			return
		}
	}
}

func (i *InactiveChainReplicator) replicateDisabledChains() {
	chains := i.listInactiveChains()
	if len(chains) == 0 {
		i.logger.Debugf("No inactive chains to try to replicate")
		return
	}

	// For each chain, ensure we registered it into the verifier registry, otherwise
	// we won't be able to verify its blocks.
	for _, chain := range chains {
		i.registerChain(chain)
	}

	i.logger.Infof("Found %d inactive chains: %v", len(chains), chains)
	lastSystemChannelConfigBlock := i.retrieveLastSysChannelConfigBlock()
	replicatedChains := i.replicator.ReplicateChains(lastSystemChannelConfigBlock, chains)
	i.logger.Infof("Successfully replicated %d chains: %v", len(replicatedChains), replicatedChains)
	i.lock.Lock()
	defer i.lock.Unlock()
	for _, chainName := range replicatedChains {
		chain := i.chains2CreationCallbacks[chainName]
		delete(i.chains2CreationCallbacks, chainName)
		chain.create()
	}
}

// Stop stops the inactive chain replicator. This is used when removing the
// system channel.
func (i *InactiveChainReplicator) Stop() {
	close(i.quitChan)
	<-i.doneChan
}

func (i *InactiveChainReplicator) listInactiveChains() []string {
	i.lock.RLock()
	defer i.lock.RUnlock()
	var chains []string
	for chain := range i.chains2CreationCallbacks {
		chains = append(chains, chain)
	}
	return chains
}

type blockGetter struct {
	ledger blockledger.Reader
}

func (bg *blockGetter) Block(number uint64) *common.Block {
	return blockledger.GetBlock(bg.ledger, number)
}

type verifierLoader struct {
	ledgerFactory   blockledger.Factory
	verifierFactory cluster.VerifierFactory
	logger          *flogging.FabricLogger
	onFailure       func(block *common.Block)
}

type verifiersByChannel map[string]cluster.BlockVerifier

func (vl *verifierLoader) loadVerifiers() verifiersByChannel {
	res := make(verifiersByChannel)

	for _, channel := range vl.ledgerFactory.ChannelIDs() {
		v := vl.loadVerifier(channel)
		if v == nil {
			continue
		}
		res[channel] = v
	}

	return res
}

func (vl *verifierLoader) loadVerifier(chain string) cluster.BlockVerifier {
	ledger, err := vl.ledgerFactory.GetOrCreate(chain)
	if err != nil {
		vl.logger.Panicf("Failed obtaining ledger for channel %s", chain)
	}

	blockRetriever := &blockGetter{ledger: ledger}
	height := ledger.Height()
	if height == 0 {
		vl.logger.Errorf("Channel %s has no blocks, skipping it", chain)
		return nil
	}
	lastBlockIndex := height - 1
	lastBlock := blockRetriever.Block(lastBlockIndex)
	if lastBlock == nil {
		vl.logger.Panicf("Failed retrieving block [%d] for channel %s", lastBlockIndex, chain)
	}

	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, blockRetriever)
	if err != nil {
		vl.logger.Panicf("Failed retrieving config block [%d] for channel %s", lastBlockIndex, chain)
	}
	conf, err := cluster.ConfigFromBlock(lastConfigBlock)
	if err != nil {
		vl.onFailure(lastConfigBlock)
		vl.logger.Panicf("Failed extracting configuration for channel %s from block [%d]: %v",
			chain, lastConfigBlock.Header.Number, err)
	}

	verifier, err := vl.verifierFactory.VerifierFromConfig(conf, chain)
	if err != nil {
		vl.onFailure(lastConfigBlock)
		vl.logger.Panicf("Failed creating verifier for channel %s from block [%d]: %v", chain, lastBlockIndex, err)
	}
	vl.logger.Infof("Loaded verifier for channel %s from config block at index %d", chain, lastBlockIndex)
	return verifier
}

// ValidateBootstrapBlock returns whether this block can be used as a bootstrap block.
// A bootstrap block is a block of a system channel, and needs to have a ConsortiumsConfig.
func ValidateBootstrapBlock(block *common.Block, bccsp bccsp.BCCSP) error {
	if block == nil {
		return errors.New("nil block")
	}

	if block.Data == nil || len(block.Data.Data) == 0 {
		return errors.New("empty block data")
	}

	firstTransaction := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], firstTransaction); err != nil {
		return errors.Wrap(err, "failed extracting envelope from block")
	}

	bundle, err := channelconfig.NewBundleFromEnvelope(firstTransaction, bccsp)
	if err != nil {
		return err
	}

	_, exists := bundle.ConsortiumsConfig()
	if !exists {
		return errors.New("the block isn't a system channel block because it lacks ConsortiumsConfig")
	}
	return nil
}
