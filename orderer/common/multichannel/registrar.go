/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package multichannel tracks the channel resources for the orderer.  It initially
// loads the set of existing channels, and provides an interface for users of these
// channels to retrieve them, or create new ones.
package multichannel

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/filerepo"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	msgVersion = int32(0)
	epoch      = 0
)

var logger = flogging.MustGetLogger("orderer.commmon.multichannel")

// Registrar serves as a point of access and control for the individual channel resources.
type Registrar struct {
	config localconfig.TopLevel

	lock      sync.RWMutex
	chains    map[string]*ChainSupport
	followers map[string]*follower.Chain
	// existence indicates removal is in-progress or failed
	// when failed, the status will indicate failed all other states
	// denote an in-progress removal
	pendingRemoval  map[string]consensus.StaticStatusReporter
	systemChannelID string
	systemChannel   *ChainSupport

	consenters                  map[string]consensus.Consenter
	ledgerFactory               blockledger.Factory
	signer                      identity.SignerSerializer
	blockcutterMetrics          *blockcutter.Metrics
	templator                   msgprocessor.ChannelConfigTemplator
	callbacks                   []channelconfig.BundleActor
	bccsp                       bccsp.BCCSP
	clusterDialer               *cluster.PredicateDialer
	channelParticipationMetrics *Metrics

	joinBlockFileRepo *filerepo.Repo
}

// ConfigBlockOrPanic retrieves the last configuration block from the given ledger.
// Panics on failure.
func ConfigBlockOrPanic(reader blockledger.Reader) *cb.Block {
	lastBlock := blockledger.GetBlock(reader, reader.Height()-1)
	index, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last config in its latest block: %s", err)
	}
	configBlock := blockledger.GetBlock(reader, index)
	if configBlock == nil {
		logger.Panicf("Config block does not exist")
	}

	return configBlock
}

func configTx(reader blockledger.Reader) *cb.Envelope {
	return protoutil.ExtractEnvelopeOrPanic(ConfigBlockOrPanic(reader), 0)
}

// NewRegistrar produces an instance of a *Registrar.
func NewRegistrar(
	config localconfig.TopLevel,
	ledgerFactory blockledger.Factory,
	signer identity.SignerSerializer,
	metricsProvider metrics.Provider,
	bccsp bccsp.BCCSP,
	clusterDialer *cluster.PredicateDialer,
	callbacks ...channelconfig.BundleActor) *Registrar {
	r := &Registrar{
		config:                      config,
		chains:                      make(map[string]*ChainSupport),
		followers:                   make(map[string]*follower.Chain),
		pendingRemoval:              make(map[string]consensus.StaticStatusReporter),
		ledgerFactory:               ledgerFactory,
		signer:                      signer,
		blockcutterMetrics:          blockcutter.NewMetrics(metricsProvider),
		callbacks:                   callbacks,
		bccsp:                       bccsp,
		clusterDialer:               clusterDialer,
		channelParticipationMetrics: NewMetrics(metricsProvider),
	}

	if config.ChannelParticipation.Enabled {
		var err error
		r.joinBlockFileRepo, err = InitJoinBlockFileRepo(&r.config)
		if err != nil {
			logger.Panicf("Error initializing joinblock file repo: %s", err)
		}
	}

	return r
}

// InitJoinBlockFileRepo initialize the channel participation API joinblock file repo. This creates
// the fileRepoDir on the filesystem if it does not already exist.
func InitJoinBlockFileRepo(config *localconfig.TopLevel) (*filerepo.Repo, error) {
	fileRepoDir := filepath.Join(config.FileLedger.Location, "pendingops")
	logger.Infof("Channel Participation API enabled, registrar initializing with file repo %s", fileRepoDir)

	joinBlockFileRepo, err := filerepo.New(fileRepoDir, "join")
	if err != nil {
		return nil, err
	}
	return joinBlockFileRepo, nil
}

func (r *Registrar) Initialize(consenters map[string]consensus.Consenter) {
	r.init(consenters)

	r.lock.Lock()
	defer r.lock.Unlock()
	r.startChannels()
}

func (r *Registrar) init(consenters map[string]consensus.Consenter) {
	r.consenters = consenters

	// Discover and load join-blocks. If there is a join-block, there must be a ledger; if there is none, create it.
	// channelsWithJoinBlock maps channelID to a join-block.
	channelsWithJoinBlock := r.loadJoinBlocks()

	// Discover all ledgers. This should already include all channels with join blocks as well.
	// Make sure there are no empty ledgers without a corresponding join-block.
	existingChannels := r.discoverLedgers(channelsWithJoinBlock)

	// Scan for and initialize the system channel, if it exists.
	// Note that there may be channels with empty ledgers, but always with a join block.
	r.initSystemChannel(existingChannels)

	// Initialize application channels, by creating either a consensus.Chain or a follower.Chain.
	if r.systemChannelID == "" {
		r.initAppChannels(existingChannels, channelsWithJoinBlock)
	} else {
		r.initAppChannelsWhenSystemChannelExists(existingChannels)
	}
}

// startChannels starts internal go-routines in chains and followers.
// Since these go-routines may call-back on the Registrar, this must be protected with a lock.
func (r *Registrar) startChannels() {
	for _, chainSupport := range r.chains {
		chainSupport.start()
	}
	for _, fChain := range r.followers {
		fChain.Start()
	}

	if r.systemChannelID == "" {
		logger.Infof("Registrar initializing without a system channel, number of application channels: %d, with %d consensus.Chain(s) and %d follower.Chain(s)",
			len(r.chains)+len(r.followers), len(r.chains), len(r.followers))
	}
}

func (r *Registrar) discoverLedgers(channelsWithJoinBlock map[string]*cb.Block) []string {
	// Discover all ledgers. This should already include all channels with join blocks as well.
	existingChannels := r.ledgerFactory.ChannelIDs()

	for _, channelID := range existingChannels {
		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}
		// Prune empty ledgers without a join block
		if rl.Height() == 0 {
			if _, ok := channelsWithJoinBlock[channelID]; !ok {
				logger.Warnf("Channel '%s' has an empty ledger without a join-block, removing it", channelID)
				if err := r.ledgerFactory.Remove(channelID); err != nil {
					logger.Panicf("Ledger factory failed to remove empty ledger '%s', error: %s", channelID, err)
				}
			}
		}
	}

	return r.ledgerFactory.ChannelIDs()
}

// initSystemChannel scan for and initialize the system channel, if it exists.
func (r *Registrar) initSystemChannel(existingChannels []string) {
	for _, channelID := range existingChannels {
		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}

		if rl.Height() == 0 {
			// At this point in the initialization flow the system channel cannot be with height==0 and a join-block.
			// Even when the system channel is joined via the channel participation API, on-boarding is performed
			// prior to this point. Therefore, this is an application channel.
			continue // Skip application channels
		}

		configTransaction := configTx(rl)
		if configTransaction == nil {
			logger.Panic("Programming error, configTransaction should never be nil here")
		}
		ledgerResources, err := r.newLedgerResources(configTransaction)
		if err != nil {
			logger.Panicf("Error creating ledger resources: %s", err)
		}
		channelID := ledgerResources.ConfigtxValidator().ChannelID()

		if _, ok := ledgerResources.ConsortiumsConfig(); !ok {
			continue // Skip application channels
		}

		if r.systemChannelID != "" {
			logger.Panicf("There appear to be two system channels %s and %s", r.systemChannelID, channelID)
		}

		chain, err := newChainSupport(r, ledgerResources, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
		if err != nil {
			logger.Panicf("Error creating chain support: %s", err)
		}
		r.templator = msgprocessor.NewDefaultTemplator(chain, r.bccsp)
		chain.Processor = msgprocessor.NewSystemChannel(
			chain,
			r.templator,
			msgprocessor.CreateSystemChannelFilters(r.config, r, chain, chain.MetadataValidator),
			r.bccsp,
		)

		// Retrieve genesis block to log its hash. See FAB-5450 for the purpose
		genesisBlock := ledgerResources.Block(0)
		if genesisBlock == nil {
			logger.Panicf("Error reading genesis block of system channel '%s'", channelID)
		}
		logger.Infof("Starting system channel '%s' with genesis block hash %x and orderer type %s",
			channelID, protoutil.BlockHeaderHash(genesisBlock.Header), chain.SharedConfig().ConsensusType())

		r.chains[channelID] = chain
		r.systemChannelID = channelID
		r.systemChannel = chain
	}
}

// initAppChannels initializes application channels, assuming that the system channel does NOT exist.
// This implies that the orderer is using the channel participation API for joins (channel creation).
func (r *Registrar) initAppChannels(existingChannels []string, channelsWithJoinBlock map[string]*cb.Block) {
	// init app channels with join-blocks
	for channelID, joinBlock := range channelsWithJoinBlock {
		ledgerRes, clusterConsenter, err := r.initLedgerResourcesClusterConsenter(joinBlock)
		if err != nil {
			logger.Panicf("Error: %s, channel: %s", err, channelID)
		}

		isMember, err := clusterConsenter.IsChannelMember(joinBlock)
		if err != nil {
			logger.Panicf("Failed to determine cluster membership from join-block, channel: %s, error: %s", channelID, err)
		}

		if joinBlock.Header.Number == 0 && isMember {
			if _, _, err := r.createAsMember(ledgerRes, joinBlock, channelID); err != nil {
				logger.Panicf("Failed to createAsMember, error: %s", err)
			}
			if err := r.removeJoinBlock(channelID); err != nil {
				logger.Panicf("Failed to remove join-block, channel: %s, error: %s", channelID, err)
			}
		} else {
			if _, _, err = r.createFollower(ledgerRes, clusterConsenter, joinBlock, channelID); err != nil {
				logger.Panicf("Failed to createFollower, error: %s", err)
			}
		}
	}

	// init app channels without join-blocks
	for _, channelID := range existingChannels {
		if _, withJoinBlock := channelsWithJoinBlock[channelID]; withJoinBlock {
			continue // Skip channels with join-blocks, since they were already initialized above.
		}

		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}

		configBlock := ConfigBlockOrPanic(rl)
		configTx := protoutil.ExtractEnvelopeOrPanic(configBlock, 0)
		if configTx == nil {
			logger.Panic("Programming error, configTx should never be nil here")
		}
		ledgerRes, err := r.newLedgerResources(configTx)
		if err != nil {
			logger.Panicf("Error creating ledger resources: %s", err)
		}

		ordererConfig, _ := ledgerRes.OrdererConfig()
		consenter, foundConsenter := r.consenters[ordererConfig.ConsensusType()]
		if !foundConsenter {
			logger.Panicf("Failed to find a consenter for consensus type: %s", ordererConfig.ConsensusType())
		}

		clusterConsenter, ok := consenter.(consensus.ClusterConsenter)
		if !ok {
			logger.Panic("clusterConsenter is not a consensus.ClusterConsenter")
		}

		isMember, err := clusterConsenter.IsChannelMember(configBlock)
		if err != nil {
			logger.Panicf("Failed to determine cluster membership from config-block, error: %s", err)
		}

		if isMember {
			chainSupport, err := newChainSupport(r, ledgerRes, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
			if err != nil {
				logger.Panicf("Failed to create chain support for channel '%s', error: %s", channelID, err)
			}
			r.chains[channelID] = chainSupport
		} else {
			_, _, err := r.createFollower(ledgerRes, clusterConsenter, nil, channelID)
			if err != nil {
				logger.Panicf("Failed to create follower for channel '%s', error: %s", channelID, err)
			}
		}
	}
}

// initAppChannelsWhenSystemChannelExists initializes application channels, assuming that the system channel exists.
// This implies that the channel participation API is not used for joins (channel creation). Therefore, there are no
// join-blocks, and follower.Chain(s) are never created. The call to newChainSupport creates a consensus.Chain of the
// appropriate type.
func (r *Registrar) initAppChannelsWhenSystemChannelExists(existingChannels []string) {
	for _, channelID := range existingChannels {
		if channelID == r.systemChannelID {
			continue // Skip system channel
		}
		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}

		configTxEnv := configTx(rl)
		if configTxEnv == nil {
			logger.Panic("Programming error, configTxEnv should never be nil here")
		}
		ledgerRes, err := r.newLedgerResources(configTxEnv)
		if err != nil {
			logger.Panicf("Error creating ledger resources: %s", err)
		}

		chainSupport, err := newChainSupport(r, ledgerRes, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
		if err != nil {
			logger.Panicf("Failed to create chain support for channel '%s', error: %s", channelID, err)
		}
		r.chains[channelID] = chainSupport
	}
}

func (r *Registrar) initLedgerResourcesClusterConsenter(configBlock *cb.Block) (*ledgerResources, consensus.ClusterConsenter, error) {
	configEnv, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed extracting config envelope from block")
	}

	ledgerRes, err := r.newLedgerResources(configEnv)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed creating ledger resources")
	}

	ordererConfig, _ := ledgerRes.OrdererConfig()
	consenter, foundConsenter := r.consenters[ordererConfig.ConsensusType()]
	if !foundConsenter {
		return nil, nil, errors.Errorf("failed to find a consenter for consensus type: %s", ordererConfig.ConsensusType())
	}

	clusterConsenter, ok := consenter.(consensus.ClusterConsenter)
	if !ok {
		return nil, nil, errors.New("failed cast: clusterConsenter is not a consensus.ClusterConsenter")
	}

	return ledgerRes, clusterConsenter, nil
}

// SystemChannelID returns the ChannelID for the system channel.
func (r *Registrar) SystemChannelID() string {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.systemChannelID
}

// SystemChannel returns the ChainSupport for the system channel.
func (r *Registrar) SystemChannel() *ChainSupport {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.systemChannel
}

// BroadcastChannelSupport returns the message channel header, whether the message is a config update
// and the channel resources for a message or an error if the message is not a message which can
// be processed directly (like CONFIG and ORDERER_TRANSACTION messages)
func (r *Registrar) BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, *ChainSupport, error) {
	chdr, err := protoutil.ChannelHeader(msg)
	if err != nil {
		return nil, false, nil, errors.WithMessage(err, "could not determine channel ID")
	}

	cs := r.GetChain(chdr.ChannelId)
	// New channel creation
	if cs == nil {
		sysChan := r.SystemChannel()
		if sysChan == nil {
			return nil, false, nil, errors.New("channel creation request not allowed because the orderer system channel is not defined")
		}
		cs = sysChan
	}

	isConfig := false
	switch cs.ClassifyMsg(chdr) {
	case msgprocessor.ConfigUpdateMsg:
		isConfig = true
	case msgprocessor.ConfigMsg:
		return chdr, false, nil, errors.New("message is of type that cannot be processed directly")
	default:
	}

	return chdr, isConfig, cs, nil
}

// GetConsensusChain retrieves the consensus.Chain of the channel, if it exists.
func (r *Registrar) GetConsensusChain(chainID string) consensus.Chain {
	r.lock.RLock()
	defer r.lock.RUnlock()

	cs, exists := r.chains[chainID]
	if !exists {
		return nil
	}

	return cs.Chain
}

// GetChain retrieves the chain support for a chain if it exists.
func (r *Registrar) GetChain(chainID string) *ChainSupport {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.chains[chainID]
}

// GetFollower retrieves the follower.Chain if it exists.
func (r *Registrar) GetFollower(chainID string) *follower.Chain {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.followers[chainID]
}

func (r *Registrar) newLedgerResources(configTx *cb.Envelope) (*ledgerResources, error) {
	payload, err := protoutil.UnmarshalPayload(configTx.Payload)
	if err != nil {
		return nil, errors.WithMessage(err, "error umarshaling envelope to payload")
	}

	if payload.Header == nil {
		return nil, errors.New("missing channel header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshaling channel header")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.WithMessage(err, "error umarshaling config envelope from payload data")
	}

	bundle, err := channelconfig.NewBundle(chdr.ChannelId, configEnvelope.Config, r.bccsp)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating channelconfig bundle")
	}

	err = checkResources(bundle)
	if err != nil {
		return nil, errors.WithMessagef(err, "error checking bundle for channel: %s", chdr.ChannelId)
	}

	ledger, err := r.ledgerFactory.GetOrCreate(chdr.ChannelId)
	if err != nil {
		return nil, errors.WithMessagef(err, "error getting ledger for channel: %s", chdr.ChannelId)
	}

	return &ledgerResources{
		configResources: &configResources{
			mutableResources: channelconfig.NewBundleSource(bundle, r.callbacks...),
			bccsp:            r.bccsp,
		},
		ReadWriter: ledger,
	}, nil
}

// CreateChain makes the Registrar create a consensus.Chain with the given name.
func (r *Registrar) CreateChain(chainName string) {
	lf, err := r.ledgerFactory.GetOrCreate(chainName)
	if err != nil {
		logger.Panicf("Failed obtaining ledger factory for %s: %v", chainName, err)
	}
	chain := r.GetChain(chainName)
	if chain != nil {
		logger.Infof("A chain of type %T for channel %s already exists. "+
			"Halting it.", chain.Chain, chainName)
		chain.Halt()
	}
	r.newChain(configTx(lf))
}

func (r *Registrar) newChain(configtx *cb.Envelope) {
	r.lock.Lock()
	defer r.lock.Unlock()

	cs := r.createNewChain(configtx)
	cs.start()
	logger.Infof("Created and started new channel %s", cs.ChannelID())
}

func (r *Registrar) createNewChain(configtx *cb.Envelope) *ChainSupport {
	ledgerResources, err := r.newLedgerResources(configtx)
	if err != nil {
		logger.Panicf("Error creating ledger resources: %s", err)
	}

	// If we have no blocks, we need to create the genesis block ourselves.
	if ledgerResources.Height() == 0 {
		if err := ledgerResources.Append(blockledger.CreateNextBlock(ledgerResources, []*cb.Envelope{configtx})); err != nil {
			logger.Panicf("Error appending genesis block to ledger: %s", err)
		}
	}
	cs, err := newChainSupport(r, ledgerResources, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
	if err != nil {
		logger.Panicf("Error creating chain support: %s", err)
	}

	chainID := ledgerResources.ConfigtxValidator().ChannelID()
	r.chains[chainID] = cs

	return cs
}

// SwitchFollowerToChain creates a consensus.Chain from the tip of the ledger, and removes the follower.
// It is called when a follower detects a config block that indicates cluster membership and halts, transferring
// execution to the consensus.Chain.
func (r *Registrar) SwitchFollowerToChain(channelID string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	lf, err := r.ledgerFactory.GetOrCreate(channelID)
	if err != nil {
		logger.Panicf("Failed obtaining ledger factory for channel %s: %v", channelID, err)
	}

	if _, chainExists := r.chains[channelID]; chainExists {
		logger.Panicf("Programming error, channel already exists: %s", channelID)
	}

	delete(r.followers, channelID)
	logger.Debugf("Removed follower for channel %s", channelID)
	cs := r.createNewChain(configTx(lf))
	if err := r.removeJoinBlock(channelID); err != nil {
		logger.Panicf("Failed removing join-block for channel: %s: %v", channelID, err)
	}
	cs.start()
	logger.Infof("Created and started channel %s", cs.ChannelID())
}

// SwitchChainToFollower creates a follower.Chain from the tip of the ledger and removes the consensus.Chain.
// It is called when an etcdraft.Chain detects it was evicted from the cluster (i.e. removed from the consenters set)
// and halts, transferring execution to the follower.Chain.
func (r *Registrar) SwitchChainToFollower(channelName string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, chainExists := r.chains[channelName]; !chainExists {
		logger.Infof("Channel %s consenter was removed", channelName)
		return
	}

	if _, followerExists := r.followers[channelName]; followerExists {
		logger.Panicf("Programming error, both a follower.Chain and a consensus.Chain exist, channel: %s", channelName)
	}

	rl, err := r.ledgerFactory.GetOrCreate(channelName)
	if err != nil {
		logger.Panicf("Failed obtaining ledger factory for %s: %v", channelName, err)
	}

	configBlock := ConfigBlockOrPanic(rl)
	ledgerRes, clusterConsenter, err := r.initLedgerResourcesClusterConsenter(configBlock)
	if err != nil {
		logger.Panicf("Error initializing ledgerResources & clusterConsenter: %s", err)
	}

	delete(r.chains, channelName)
	logger.Debugf("Removed consensus.Chain for channel %s", channelName)

	fChain, _, err := r.createFollower(ledgerRes, clusterConsenter, nil, channelName)
	if err != nil {
		logger.Panicf("Failed to create follower.Chain for channel '%s', error: %s", channelName, err)
	}
	fChain.Start()

	logger.Infof("Created and started a follower.Chain for channel %s", channelName)
}

// ChannelsCount returns the count of the current total number of channels.
func (r *Registrar) ChannelsCount() int {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return len(r.chains) + len(r.followers)
}

// NewChannelConfig produces a new template channel configuration based on the system channel's current config.
func (r *Registrar) NewChannelConfig(envConfigUpdate *cb.Envelope) (channelconfig.Resources, error) {
	return r.templator.NewChannelConfig(envConfigUpdate)
}

// CreateBundle calls channelconfig.NewBundle
func (r *Registrar) CreateBundle(channelID string, config *cb.Config) (channelconfig.Resources, error) {
	return channelconfig.NewBundle(channelID, config, r.bccsp)
}

// ChannelList returns a slice of ChannelInfoShort containing all application channels (excluding the system
// channel), and ChannelInfoShort of the system channel (nil if does not exist).
// The URL fields are empty, and are to be completed by the caller.
func (r *Registrar) ChannelList() types.ChannelList {
	r.lock.RLock()
	defer r.lock.RUnlock()

	list := types.ChannelList{}

	if r.systemChannelID != "" {
		list.SystemChannel = &types.ChannelInfoShort{Name: r.systemChannelID}
	}
	for name := range r.chains {
		if name == r.systemChannelID {
			continue
		}
		list.Channels = append(list.Channels, types.ChannelInfoShort{Name: name})
	}
	for name := range r.followers {
		list.Channels = append(list.Channels, types.ChannelInfoShort{Name: name})
	}

	for c := range r.pendingRemoval {
		list.Channels = append(list.Channels, types.ChannelInfoShort{
			Name: c,
		})

	}

	return list
}

// ChannelInfo provides extended status information about a channel.
// The URL field is empty, and is to be completed by the caller.
func (r *Registrar) ChannelInfo(channelID string) (types.ChannelInfo, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	info := types.ChannelInfo{Name: channelID}

	if c, ok := r.chains[channelID]; ok {
		info.Height = c.Height()
		info.ConsensusRelation, info.Status = c.StatusReport()
		return info, nil
	}

	if f, ok := r.followers[channelID]; ok {
		info.Height = f.Height()
		info.ConsensusRelation, info.Status = f.StatusReport()
		return info, nil
	}

	status, ok := r.pendingRemoval[channelID]
	if ok {
		return types.ChannelInfo{
			Name:              channelID,
			ConsensusRelation: status.ConsensusRelation,
			Status:            status.Status,
		}, nil
	}

	return types.ChannelInfo{}, types.ErrChannelNotExist
}

// JoinChannel instructs the orderer to create a channel and join it with the provided config block.
// The URL field is empty, and is to be completed by the caller.
func (r *Registrar) JoinChannel(channelID string, configBlock *cb.Block, isAppChannel bool) (info types.ChannelInfo, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if status, ok := r.pendingRemoval[channelID]; ok {
		if status.Status == types.StatusFailed {
			return types.ChannelInfo{}, types.ErrChannelRemovalFailure
		}
		return types.ChannelInfo{}, types.ErrChannelPendingRemoval
	}

	if r.systemChannelID != "" {
		return types.ChannelInfo{}, types.ErrSystemChannelExists
	}

	if _, ok := r.chains[channelID]; ok {
		return types.ChannelInfo{}, types.ErrChannelAlreadyExists
	}

	if _, ok := r.followers[channelID]; ok {
		return types.ChannelInfo{}, types.ErrChannelAlreadyExists
	}

	if !isAppChannel && len(r.chains) > 0 {
		return types.ChannelInfo{}, types.ErrAppChannelsAlreadyExists
	}

	defer func() {
		if err != nil {
			if err2 := r.ledgerFactory.Remove(channelID); err2 != nil {
				logger.Warningf("Failed to cleanup ledger: %v", err2)
			}
		}
	}()
	ledgerRes, clusterConsenter, err := r.initLedgerResourcesClusterConsenter(configBlock)
	if err != nil {
		return types.ChannelInfo{}, err
	}

	blockBytes, err := proto.Marshal(configBlock)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "failed marshaling joinblock")
	}

	if err := r.joinBlockFileRepo.Save(channelID, blockBytes); err != nil {
		return types.ChannelInfo{}, errors.WithMessagef(err, "failed saving joinblock to file repo for channel %s", channelID)
	}
	defer func() {
		if err != nil {
			if err2 := r.removeJoinBlock(channelID); err2 != nil {
				logger.Warningf("Failed to cleanup joinblock for channel %s: %v", channelID, err2)
			}
		}
	}()

	if !isAppChannel {
		info, err := r.joinSystemChannel(ledgerRes, clusterConsenter, configBlock, channelID)
		return info, err
	}

	isMember, err := clusterConsenter.IsChannelMember(configBlock)
	if err != nil {
		return types.ChannelInfo{}, errors.WithMessage(err, "failed to determine cluster membership from join-block")
	}

	if configBlock.Header.Number == 0 && isMember {
		chain, info, err := r.createAsMember(ledgerRes, configBlock, channelID)
		if err == nil {
			if err := r.removeJoinBlock(channelID); err != nil {
				return types.ChannelInfo{}, err
			}
			chain.start()
		}
		return info, err
	}

	fChain, info, err := r.createFollower(ledgerRes, clusterConsenter, configBlock, channelID)
	if err != nil {
		return info, errors.WithMessage(err, "failed to create follower")
	}

	fChain.Start()
	logger.Infof("Joining channel: %v", info)
	return info, err
}

func (r *Registrar) createAsMember(ledgerRes *ledgerResources, configBlock *cb.Block, channelID string) (*ChainSupport, types.ChannelInfo, error) {
	if ledgerRes.Height() == 0 {
		if err := ledgerRes.Append(configBlock); err != nil {
			return nil, types.ChannelInfo{}, errors.WithMessage(err, "failed to append join block to the ledger")
		}
	}
	chain, err := newChainSupport(
		r,
		ledgerRes,
		r.consenters,
		r.signer,
		r.blockcutterMetrics,
		r.bccsp,
	)
	if err != nil {
		return nil, types.ChannelInfo{}, errors.WithMessage(err, "failed to create chain support")
	}

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ConsensusRelation, info.Status = chain.StatusReport()
	r.chains[channelID] = chain

	logger.Infof("Joining channel: %v", info)
	return chain, info, nil
}

// createFollower created a follower.Chain, puts it in the map, but does not start it.
func (r *Registrar) createFollower(
	ledgerRes *ledgerResources,
	clusterConsenter consensus.ClusterConsenter,
	joinBlock *cb.Block,
	channelID string,
) (*follower.Chain, types.ChannelInfo, error) {
	fLog := flogging.MustGetLogger("orderer.commmon.follower")
	blockPullerCreator, err := follower.NewBlockPullerCreator(
		channelID, fLog, r.signer, r.clusterDialer, r.config.General.Cluster, r.bccsp)
	if err != nil {
		return nil, types.ChannelInfo{}, errors.WithMessagef(err, "failed to create BlockPullerFactory for channel %s", channelID)
	}

	fChain, err := follower.NewChain(
		ledgerRes,
		clusterConsenter,
		joinBlock,
		follower.Options{
			Logger: fLog,
		},
		blockPullerCreator,
		r,
		r.bccsp,
		r,
	)

	if err != nil {
		return nil, types.ChannelInfo{}, errors.WithMessagef(err, "failed to create follower for channel %s", channelID)
	}

	clusterRelation, status := fChain.StatusReport()
	info := types.ChannelInfo{
		Name:              channelID,
		URL:               "",
		Height:            ledgerRes.Height(),
		ConsensusRelation: clusterRelation,
		Status:            status,
	}

	r.followers[channelID] = fChain

	logger.Debugf("Created follower.Chain: %v", info)
	return fChain, info, nil
}

// Assumes the system channel join-block is saved to the file repo.
func (r *Registrar) joinSystemChannel(
	ledgerRes *ledgerResources,
	clusterConsenter consensus.ClusterConsenter,
	configBlock *cb.Block,
	channelID string,
) (types.ChannelInfo, error) {
	logger.Infof("Joining system channel '%s', with config block number: %d", channelID, configBlock.Header.Number)

	if configBlock.Header.Number == 0 {
		if err := ledgerRes.Append(configBlock); err != nil {
			return types.ChannelInfo{}, errors.WithMessage(err, "error appending config block to the ledger")
		}
	}

	// This is a degenerate ChainSupport holding an inactive.Chain, that will respond to a GET request with the info
	// returned below. This is an indication to the user/admin that the orderer needs a restart, and prevent
	// conflicting channel participation API actions on the orderer.
	cs, err := newOnBoardingChainSupport(ledgerRes, r.config, r.bccsp)
	if err != nil {
		return types.ChannelInfo{}, errors.WithMessage(err, "error creating onboarding chain support")
	}
	r.chains[channelID] = cs
	r.systemChannel = cs
	r.systemChannelID = channelID

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ConsensusRelation, info.Status = r.systemChannel.StatusReport()

	logger.Infof("System channel creation pending: server requires restart! ChannelInfo: %v", info)

	return info, nil
}

// RemoveChannel instructs the orderer to remove a channel.
func (r *Registrar) RemoveChannel(channelID string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	status, ok := r.pendingRemoval[channelID]
	if ok && status.Status != types.StatusFailed {
		return types.ErrChannelPendingRemoval
	}

	if r.systemChannelID != "" {
		if channelID != r.systemChannelID {
			return types.ErrSystemChannelExists
		}
		return r.removeSystemChannel()
	}

	cs, ok := r.chains[channelID]
	if ok {
		cs.Halt()
		r.removeMember(channelID, cs)
		return nil
	}

	fChain, ok := r.followers[channelID]
	if ok {
		fChain.Halt()
		return r.removeFollower(channelID, fChain)
	}

	return types.ErrChannelNotExist
}

func (r *Registrar) removeMember(channelID string, cs *ChainSupport) {
	relation, status := cs.StatusReport()
	r.pendingRemoval[channelID] = consensus.StaticStatusReporter{ConsensusRelation: relation, Status: status}
	r.removeLedgerAsync(channelID)

	delete(r.chains, channelID)

	logger.Infof("Removed channel: %s", channelID)
}

func (r *Registrar) removeFollower(channelID string, follower *follower.Chain) error {
	// join block may still exist if the follower is:
	// 1) still onboarding
	// 2) active but not yet called registrar.SwitchFollowerToChain()
	// NOTE: if the join block does not exist, os.RemoveAll returns nil
	// so there is no harm attempting to remove a non-existent join block.
	if err := r.removeJoinBlock(channelID); err != nil {
		return err
	}

	relation, status := follower.StatusReport()
	r.pendingRemoval[channelID] = consensus.StaticStatusReporter{ConsensusRelation: relation, Status: status}
	r.removeLedgerAsync(channelID)

	delete(r.followers, channelID)

	logger.Infof("Removed channel: %s", channelID)

	return nil
}

func (r *Registrar) loadJoinBlocks() map[string]*cb.Block {
	channelToBlockMap := make(map[string]*cb.Block)
	if !r.config.ChannelParticipation.Enabled {
		return channelToBlockMap
	}
	channelsList, err := r.joinBlockFileRepo.List()
	if err != nil {
		logger.Panicf("Error listing join block file repo: %s", err)
	}

	logger.Debugf("Loading join-blocks for %d channels: %s", len(channelsList), channelsList)
	for _, fileName := range channelsList {
		channelName := r.joinBlockFileRepo.FileToBaseName(fileName)
		blockBytes, err := r.joinBlockFileRepo.Read(channelName)
		if err != nil {
			logger.Panicf("Error reading join block file: '%s', error: %s", fileName, err)
		}
		block, err := protoutil.UnmarshalBlock(blockBytes)
		if err != nil {
			logger.Panicf("Error unmarshalling join block file: '%s', error: %s", fileName, err)
		}
		channelToBlockMap[channelName] = block
	}

	logger.Debug("Reconciling join-blocks and ledger by creating any missing ledger")
	for channelID := range channelToBlockMap {
		if _, err := r.ledgerFactory.GetOrCreate(channelID); err != nil {
			logger.Panicf("Failed to create a ledger for channel: '%s', error: %s", channelID, err)
		}
	}

	return channelToBlockMap
}

func (r *Registrar) removeJoinBlock(channelID string) error {
	if err := r.joinBlockFileRepo.Remove(channelID); err != nil {
		return errors.WithMessagef(err, "failed removing joinblock for channel %s", channelID)
	}

	return nil
}

func (r *Registrar) removeSystemChannel() error {
	systemChannelID := r.systemChannelID
	consensusType := r.systemChannel.SharedConfig().ConsensusType()
	if consensusType != "etcdraft" {
		return errors.Errorf("cannot remove %s system channel: %s", consensusType, systemChannelID)
	}

	// halt the inactive chain registry
	consenter := r.consenters["etcdraft"].(consensus.ClusterConsenter)
	consenter.RemoveInactiveChainRegistry()

	// halt the system channel and remove it from the chains map
	r.systemChannel.Halt()
	delete(r.chains, systemChannelID)

	// remove system channel resources
	err := r.ledgerFactory.Remove(systemChannelID)
	if err != nil {
		return errors.WithMessagef(err, "failed removing ledger for system channel %s", r.systemChannelID)
	}

	// remove system channel references
	r.systemChannel = nil
	r.systemChannelID = ""
	logger.Infof("removed system channel: %s", systemChannelID)

	failedRemovals := []string{}

	// halt all application channels
	for channel, cs := range r.chains {
		cs.Halt()

		rl, err := r.ledgerFactory.GetOrCreate(channel)
		if err != nil {
			return errors.WithMessagef(err, "could not retrieve ledger for channel: %s", channel)
		}
		configBlock := ConfigBlockOrPanic(rl)
		isChannelMember, err := consenter.IsChannelMember(configBlock)
		if err != nil {
			return errors.WithMessagef(err, "failed to determine channel membership for channel: %s", channel)
		}
		if !isChannelMember {
			logger.Debugf("not a member of channel %s, removing it", channel)
			err := r.ledgerFactory.Remove(channel)
			if err != nil {
				logger.Errorf("failed removing ledger for channel %s, error: %v", channel, err)
				failedRemovals = append(failedRemovals, channel)
				continue
			}

			delete(r.chains, channel)
		}
	}

	if len(failedRemovals) > 0 {
		return fmt.Errorf("failed removing ledger for channel(s): %s", strings.Join(failedRemovals, ", "))
	}

	// reintialize the registrar to recreate every channel
	r.init(r.consenters)

	// restart every channel
	r.startChannels()

	return nil
}

func (r *Registrar) removeLedgerAsync(channelID string) {

	go func() {
		err := r.ledgerFactory.Remove(channelID)
		r.lock.Lock()
		defer r.lock.Unlock()
		if err != nil {
			r.pendingRemoval[channelID] = consensus.StaticStatusReporter{ConsensusRelation: r.pendingRemoval[channelID].ConsensusRelation, Status: types.StatusFailed}
			r.channelParticipationMetrics.reportStatus(channelID, types.StatusFailed)
			logger.Errorf("ledger factory failed to remove empty ledger '%s', error: %s", channelID, err)
			return
		}
		delete(r.pendingRemoval, channelID)
	}()
}

func (r *Registrar) ReportConsensusRelationAndStatusMetrics(channelID string, relation types.ConsensusRelation, status types.Status) {
	r.channelParticipationMetrics.reportConsensusRelation(channelID, relation)
	r.channelParticipationMetrics.reportStatus(channelID, status)
}
