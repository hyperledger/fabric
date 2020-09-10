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
	"sync"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/filerepo"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/onboarding"
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

	lock            sync.RWMutex
	chains          map[string]*ChainSupport
	followers       map[string]*follower.Chain
	systemChannelID string
	systemChannel   *ChainSupport

	consenters         map[string]consensus.Consenter
	ledgerFactory      blockledger.Factory
	signer             identity.SignerSerializer
	blockcutterMetrics *blockcutter.Metrics
	templator          msgprocessor.ChannelConfigTemplator
	callbacks          []channelconfig.BundleActor
	bccsp              bccsp.BCCSP
	clusterDialer      *cluster.PredicateDialer

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
		config:             config,
		chains:             make(map[string]*ChainSupport),
		followers:          make(map[string]*follower.Chain),
		ledgerFactory:      ledgerFactory,
		signer:             signer,
		blockcutterMetrics: blockcutter.NewMetrics(metricsProvider),
		callbacks:          callbacks,
		bccsp:              bccsp,
		clusterDialer:      clusterDialer,
	}

	if config.ChannelParticipation.Enabled {
		r.initializeJoinBlockFileRepo()
	}

	return r
}

// initialize the channel participation API joinblock file repo. This creates
// the fileRepoDir on the filesystem if it does not already exist.
func (r *Registrar) initializeJoinBlockFileRepo() {
	fileRepoDir := filepath.Join(r.config.FileLedger.Location, "filerepo")
	logger.Infof("Channel Participation API enabled, registrar initializing with file repo %s", fileRepoDir)

	joinBlockFileRepo, err := filerepo.New(fileRepoDir, "joinblock")
	if err != nil {
		logger.Panicf("Error initializing join block file repo: %s", err)
	}

	r.joinBlockFileRepo = joinBlockFileRepo
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
	existingChannels := r.ledgerFactory.ChannelIDs()

	// Scan for and initialize the system channel, if it exists.
	// Make sure there are no empty ledgers without a corresponding join-block.
	// Note that there may be channels with empty ledgers, but always with a join block.
	r.initSystemChannel(existingChannels, channelsWithJoinBlock)

	// Initialize application channels, by creating either a consensus.Chain or a follower.Chain.
	r.initAppChannels(existingChannels, channelsWithJoinBlock)
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

func (r *Registrar) initSystemChannel(existingChannels []string, channelsWithJoinBlock map[string]*cb.Block) {
	for _, channelID := range existingChannels {
		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}

		if rl.Height() == 0 {
			if _, ok := channelsWithJoinBlock[channelID]; !ok {
				logger.Warnf("Channel '%s' has an empty ledger without a join-block, removing it", channelID)
				if err := r.ledgerFactory.Remove(channelID); err != nil {
					logger.Panicf("Ledger factory failed to remove empty ledger '%s', error: %s", channelID, err)
				}
			}
			//TODO currently the system channel cannot be a follower, i.e. start with height==0 and a join-block.
			// This may change in FAB-17911, when we allow a system channel to join via the channel participation API.
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

func (r *Registrar) initAppChannels(existingChannels []string, channelsWithJoinBlock map[string]*cb.Block) {
	var appChannelsWithoutJoinBlock []string
	for _, channelID := range existingChannels {
		if _, withJoinBlock := channelsWithJoinBlock[channelID]; channelID == r.systemChannelID || withJoinBlock {
			continue
		}
		appChannelsWithoutJoinBlock = append(appChannelsWithoutJoinBlock, channelID)
	}

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
			// TODO remove the join block
		} else {
			if _, _, err = r.createFollower(ledgerRes, clusterConsenter, joinBlock, channelID); err != nil {
				logger.Panicf("Failed to createFollower, error: %s", err)
			}
		}
	}

	for _, channelID := range appChannelsWithoutJoinBlock {
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

		if r.systemChannel != nil {
			chainSupport, err := newChainSupport(r, ledgerRes, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
			if err != nil {
				logger.Panicf("Failed to create chain support for channel '%s', error: %s", channelID, err)
			}
			r.chains[channelID] = chainSupport
			continue
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

func (r *Registrar) initLedgerResourcesClusterConsenter(configBlock *cb.Block) (*ledgerResources, consensus.ClusterConsenter, error) {
	configEnv, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed extracting config envelope from block")
	}

	ledgerRes, err := r.newLedgerResources(configEnv)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating ledger resources")
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
		return nil, false, nil, fmt.Errorf("could not determine channel ID: %s", err)
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
		return nil, errors.Wrap(err, "error umarshaling envelope to payload")
	}

	if payload.Header == nil {
		return nil, errors.New("missing channel header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling channel header")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrap(err, "error umarshaling config envelope from payload data")
	}

	bundle, err := channelconfig.NewBundle(chdr.ChannelId, configEnvelope.Config, r.bccsp)
	if err != nil {
		return nil, errors.Wrap(err, "error creating channelconfig bundle")
	}

	err = checkResources(bundle)
	if err != nil {
		return nil, errors.Wrapf(err, "error checking bundle for channel: %s", chdr.ChannelId)
	}

	ledger, err := r.ledgerFactory.GetOrCreate(chdr.ChannelId)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting ledger for channel: %s", chdr.ChannelId)
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
func (r *Registrar) SwitchFollowerToChain(chainName string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	lf, err := r.ledgerFactory.GetOrCreate(chainName)
	if err != nil {
		logger.Panicf("Failed obtaining ledger factory for %s: %v", chainName, err)
	}

	if _, chainExists := r.chains[chainName]; chainExists {
		logger.Panicf("Programming error, chain already exists: %s", chainName)
	}

	delete(r.followers, chainName)
	logger.Debugf("Removed follower for channel %s", chainName)
	cs := r.createNewChain(configTx(lf))
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
		info.ClusterRelation, info.Status = c.StatusReport()
		return info, nil
	}

	if f, ok := r.followers[channelID]; ok {
		info.Height = f.Height()
		info.ClusterRelation, info.Status = f.StatusReport()
		return info, nil
	}

	return types.ChannelInfo{}, types.ErrChannelNotExist
}

// JoinChannel instructs the orderer to create a channel and join it with the provided config block.
// The URL field is empty, and is to be completed by the caller.
func (r *Registrar) JoinChannel(channelID string, configBlock *cb.Block, isAppChannel bool) (types.ChannelInfo, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

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

	ledgerRes, clusterConsenter, err := r.initLedgerResourcesClusterConsenter(configBlock)
	if err != nil {
		return types.ChannelInfo{}, err
	}

	if !isAppChannel {
		info, err := r.joinSystemChannel(ledgerRes, clusterConsenter, configBlock, channelID)
		return info, err
	}

	//TODO save the join-block in the file repo to make this action crash tolerant.
	//TODO remove join block & ledger if things go bad below, (using defer that identifies an error?)

	isMember, err := clusterConsenter.IsChannelMember(configBlock)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "failed to determine cluster membership from join-block ")
	}

	if configBlock.Header.Number == 0 && isMember {
		chain, info, err := r.createAsMember(ledgerRes, configBlock, channelID)
		if err == nil {
			//TODO remove the join block
			chain.start()
		}
		return info, err
	}

	fChain, info, err := r.createFollower(ledgerRes, clusterConsenter, configBlock, channelID)
	if err != nil {
		return info, errors.Wrap(err, "failed to create follower")
	}

	fChain.Start()
	logger.Infof("Joining channel: %v", info)
	return info, err
}

func (r *Registrar) createAsMember(ledgerRes *ledgerResources, configBlock *cb.Block, channelID string) (*ChainSupport, types.ChannelInfo, error) {
	if ledgerRes.Height() == 0 {
		if err := ledgerRes.Append(configBlock); err != nil {
			return nil, types.ChannelInfo{}, errors.Wrap(err, "error appending join block to the ledger")
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
		return nil, types.ChannelInfo{}, errors.Wrap(err, "error creating chain support")
	}

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ClusterRelation, info.Status = chain.StatusReport()
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
		return nil, types.ChannelInfo{}, errors.Wrapf(err, "failed to create BlockPullerFactory for channel %s", channelID)
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
	)

	if err != nil {
		return nil, types.ChannelInfo{}, errors.Wrapf(err, "failed to create follower for channel %s", channelID)
	}

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ClusterRelation, info.Status = fChain.StatusReport()

	r.followers[channelID] = fChain

	logger.Debugf("Created follower.Chain: %v", info)
	return fChain, info, nil
}

func (r *Registrar) joinSystemChannel(
	ledgerRes *ledgerResources,
	clusterConsenter consensus.ClusterConsenter,
	configBlock *cb.Block,
	channelID string,
) (types.ChannelInfo, error) {
	logger.Infof("Joining system channel '%s', with config block number: %d", channelID, configBlock.Header.Number)

	if configBlock.Header.Number == 0 {
		if err := ledgerRes.Append(configBlock); err != nil {
			return types.ChannelInfo{}, errors.Wrap(err, "error appending config block to the ledger")
		}
	} else {
		// Save the config block to the bootstrap file, such that if the orderer crashes before on-boarding of the
		// system channel is complete, it could be resumed by restarting the orderer with BootstrapMethod == "file".
		if r.config.General.BootstrapFile == "" {
			return types.ChannelInfo{}, errors.New("missing boostrap file path")
		}
		bootstrapHelper := file.New(r.config.General.BootstrapFile)
		if err := bootstrapHelper.SaveBlock(configBlock); err != nil {
			return types.ChannelInfo{}, errors.Wrap(err, "error saving config block to boostrap file")
		}
	}

	var repInitiator *onboarding.ReplicationInitiator
	repInitiator = onboarding.NewReplicationInitiator(r.ledgerFactory, configBlock, &r.config, r.clusterDialer.Config.SecOpts, r.signer, r.bccsp)
	// Create a new InactiveChainReplicator
	getConfigBlock := func() *cb.Block {
		return ConfigBlockOrPanic(ledgerRes)
	}
	icr := onboarding.NewInactiveChainReplicator(
		repInitiator,
		getConfigBlock,
		repInitiator.RegisterChain,
		r.config.General.Cluster.ReplicationBackgroundRefreshInterval,
	)

	// This closure will complete the initialization of the registrar after the asynch onboarding or immediately.
	completeInit := func() {
		repInitiator.ChannelLister = icr
		clusterConsenter.SetInactiveChainRegistry(icr)
		go icr.Run()

		logger.Infof("Completed onboarding for system channel: %s", channelID)

		r.init(r.consenters)
		r.startChannels()

		logger.Infof("Re-initialized registrar after joining system channel: '%s', number of channels: %d", channelID, len(r.chains))
	}

	if configBlock.Header.Number > 0 {
		logger.Infof("Going to replicate system channel: '%s', and the chains it refers to", channelID)
		// This is a degenerate ChainSupport holding an inactive.Chain, that will respond to a GET request but nothing more.
		cs, err := newOnBoardingChainSupport(r, ledgerRes, r.signer, r.bccsp)
		if err != nil {
			return types.ChannelInfo{}, errors.Wrap(err, "error creating onboarding chain support")
		}
		r.chains[channelID] = cs
		r.systemChannel = cs
		r.systemChannelID = channelID
		info := types.ChannelInfo{
			Name:            channelID,
			URL:             "",
			Height:          ledgerRes.Height(),
			ClusterRelation: types.ClusterRelationMember,
			Status:          types.StatusOnBoarding,
		}
		// Replicate and complete the initialization asynchronously
		go func() {
			//This call is blocking and may take a long time, so we execute it asynchronously.
			repInitiator.ReplicateIfNeeded(configBlock)

			r.lock.Lock()
			defer r.lock.Unlock()

			completeInit()
		}()

		return info, nil
	}

	completeInit()

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ClusterRelation, info.Status = r.systemChannel.StatusReport()

	logger.Infof("Created system channel: %v", info)

	return info, nil
}

// RemoveChannel instructs the orderer to remove a channel.
func (r *Registrar) RemoveChannel(channelID string) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.systemChannelID != "" {
		if channelID != r.systemChannelID {
			return types.ErrSystemChannelExists
		}
		return r.removeSystemChannel()
	}

	cs, ok := r.chains[channelID]
	if ok {
		cs.Halt()
		return r.removeMember(channelID, cs)
	}

	fChain, ok := r.followers[channelID]
	if ok {
		fChain.Halt()
		return r.removeFollower(channelID, fChain)
	}

	return types.ErrChannelNotExist
}

func (r *Registrar) removeMember(channelID string, cs *ChainSupport) error {
	err := r.ledgerFactory.Remove(channelID)
	if err != nil {
		return errors.Errorf("error removing ledger for channel %s", channelID)
	}

	delete(r.chains, channelID)

	logger.Infof("Removed channel: %s", channelID)

	return nil
}

func (r *Registrar) removeFollower(channelID string, follower *follower.Chain) error {
	// TODO if follower is onboarding, remove the joinblock from file repo

	err := r.ledgerFactory.Remove(channelID)
	if err != nil {
		return errors.Errorf("error removing ledger for channel %s", channelID)
	}

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

	// halt all application channels
	for channel, cs := range r.chains {
		cs.Halt()

		rl, err := r.ledgerFactory.GetOrCreate(channel)
		if err != nil {
			return errors.Wrapf(err, "could not retrieve ledger for channel: %s", channel)
		}
		configBlock := ConfigBlockOrPanic(rl)
		isChannelMember, err := consenter.IsChannelMember(configBlock)
		if err != nil {
			return errors.Wrapf(err, "failed to determine channel membership for channel: %s", channel)
		}
		if !isChannelMember {
			logger.Debugf("Not a member of channel %s, removing it", channel)
			err := r.removeMember(channel, cs)
			if err != nil {
				return errors.Wrapf(err, "failed to remove channel: %s", channel)
			}
		}
	}

	// remove system channel resources
	err := r.ledgerFactory.Remove(systemChannelID)
	if err != nil {
		return errors.Errorf("error removing ledger for system channel %s", r.systemChannelID)
	}

	// remove system channel references
	r.systemChannel = nil
	r.systemChannelID = ""

	// reintialize the registrar to recreate every channel
	r.init(r.consenters)

	// restart every channel
	r.startChannels()

	logger.Infof("Removed system channel: %s", systemChannelID)

	return nil
}
