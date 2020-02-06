/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const (
	defaultReplicationBackgroundRefreshInterval = time.Minute * 5
	replicationBackgroundInitialRefreshInterval = time.Second * 10
)

type replicationInitiator struct {
	registerChain     func(chain string)
	verifierRetriever cluster.VerifierRetriever
	channelLister     cluster.ChannelLister
	logger            *flogging.FabricLogger
	secOpts           *comm.SecureOptions
	conf              *localconfig.TopLevel
	lf                cluster.LedgerFactory
	signer            crypto.LocalSigner
}

func (ri *replicationInitiator) replicateIfNeeded(bootstrapBlock *common.Block) {
	if bootstrapBlock.Header.Number == 0 {
		ri.logger.Debug("Booted with a genesis block, replication isn't an option")
		return
	}
	ri.replicateNeededChannels(bootstrapBlock)
}

func (ri *replicationInitiator) createReplicator(bootstrapBlock *common.Block, filter func(string) bool) *cluster.Replicator {
	consenterCert := etcdraft.ConsenterCertificate(ri.secOpts.Certificate)
	systemChannelName, err := utils.GetChainIDFromBlock(bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}
	pullerConfig := cluster.PullerConfigFromTopLevelConfig(systemChannelName, ri.conf, ri.secOpts.Key, ri.secOpts.Certificate, ri.signer)
	puller, err := cluster.BlockPullerFromConfigBlock(pullerConfig, bootstrapBlock, ri.verifierRetriever)
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
	if ri.channelLister != nil {
		replicator.ChannelLister = ri.channelLister
	}

	return replicator
}

func (ri *replicationInitiator) replicateNeededChannels(bootstrapBlock *common.Block) {
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
func (ri *replicationInitiator) ReplicateChains(lastConfigBlock *common.Block, chains []string) []string {
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

// inactiveChainReplicator tracks disabled chains and replicates them upon demand
type inactiveChainReplicator struct {
	registerChain                     func(chain string)
	logger                            *flogging.FabricLogger
	retrieveLastSysChannelConfigBlock func() *common.Block
	replicator                        ChainReplicator
	scheduleChan                      <-chan time.Time
	quitChan                          chan struct{}
	lock                              sync.RWMutex
	chains2CreationCallbacks          map[string]chainCreation
}

func (dc *inactiveChainReplicator) Channels() []cluster.ChannelGenesisBlock {
	dc.lock.RLock()
	defer dc.lock.RUnlock()

	var res []cluster.ChannelGenesisBlock
	for name, chain := range dc.chains2CreationCallbacks {
		res = append(res, cluster.ChannelGenesisBlock{
			ChannelName:  name,
			GenesisBlock: chain.genesisBlock,
		})
	}
	return res
}

func (dc *inactiveChainReplicator) Close() {}

type chainCreation struct {
	create       func()
	genesisBlock *common.Block
}

// TrackChain tracks a chain with the given name, and calls the given callback
// when this chain should be activated.
func (dc *inactiveChainReplicator) TrackChain(chain string, genesisBlock *common.Block, createChainCallback func()) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.logger.Infof("Adding %s to the set of chains to track", chain)
	dc.chains2CreationCallbacks[chain] = chainCreation{
		genesisBlock: genesisBlock,
		create:       createChainCallback,
	}
}

func (dc *inactiveChainReplicator) run() {
	for {
		select {
		case <-dc.scheduleChan:
			dc.replicateDisabledChains()
		case <-dc.quitChan:
			return
		}
	}
}

func (dc *inactiveChainReplicator) replicateDisabledChains() {
	chains := dc.listInactiveChains()
	if len(chains) == 0 {
		dc.logger.Debugf("No inactive chains to try to replicate")
		return
	}

	// For each chain, ensure we registered it into the verifier registry, otherwise
	// we won't be able to verify its blocks.
	for _, chain := range chains {
		dc.registerChain(chain)
	}

	dc.logger.Infof("Found %d inactive chains: %v", len(chains), chains)
	lastSystemChannelConfigBlock := dc.retrieveLastSysChannelConfigBlock()
	replicatedChains := dc.replicator.ReplicateChains(lastSystemChannelConfigBlock, chains)
	dc.logger.Infof("Successfully replicated %d chains: %v", len(replicatedChains), replicatedChains)
	dc.lock.Lock()
	defer dc.lock.Unlock()
	for _, chainName := range replicatedChains {
		chain := dc.chains2CreationCallbacks[chainName]
		delete(dc.chains2CreationCallbacks, chainName)
		chain.create()
	}
}

func (dc *inactiveChainReplicator) stop() {
	close(dc.quitChan)
}

func (dc *inactiveChainReplicator) listInactiveChains() []string {
	dc.lock.RLock()
	defer dc.lock.RUnlock()
	var chains []string
	for chain := range dc.chains2CreationCallbacks {
		chains = append(chains, chain)
	}
	return chains
}

//go:generate mockery -dir . -name Factory -case underscore  -output mocks/

// Factory retrieves or creates new ledgers by chainID
type Factory interface {
	// GetOrCreate gets an existing ledger (if it exists)
	// or creates it if it does not
	GetOrCreate(chainID string) (blockledger.ReadWriter, error)

	// ChainIDs returns the chain IDs the Factory is aware of
	ChainIDs() []string

	// Close releases all resources acquired by the factory
	Close()
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

	for _, chain := range vl.ledgerFactory.ChainIDs() {
		v := vl.loadVerifier(chain)
		if v == nil {
			continue
		}
		res[chain] = v
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
func ValidateBootstrapBlock(block *common.Block) error {
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

	bundle, err := channelconfig.NewBundleFromEnvelope(firstTransaction)
	if err != nil {
		return err
	}

	_, exists := bundle.ConsortiumsConfig()
	if !exists {
		return errors.New("the block isn't a system channel block because it lacks ConsortiumsConfig")
	}
	return nil
}
