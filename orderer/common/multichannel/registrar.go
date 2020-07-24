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
	ab "github.com/hyperledger/fabric-protos-go/orderer"
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

// ConfigBlock retrieves the last configuration block from the given ledger.
// Panics on failure.
func ConfigBlock(reader blockledger.Reader) *cb.Block {
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
	return protoutil.ExtractEnvelopeOrPanic(ConfigBlock(reader), 0)
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
	r.consenters = consenters

	//TODO reconcile existing channel ledgers with join blocks
	existingChannels := r.ledgerFactory.ChannelIDs()

	// TODO first scan for and initialize the system channel, if it exists.
	// TODO then initialize application channels, by creating either consensus.Chain or follower.Chain.
	// TODO start all channels

	for _, channelID := range existingChannels {
		rl, err := r.ledgerFactory.GetOrCreate(channelID)
		if err != nil {
			logger.Panicf("Ledger factory reported channelID %s but could not retrieve it: %s", channelID, err)
		}
		configTx := configTx(rl)
		if configTx == nil {
			logger.Panic("Programming error, configTx should never be nil here")
		}
		ledgerResources, err := r.newLedgerResources(configTx)
		if err != nil {
			logger.Panicf("Error creating ledger resources: %s", err)
		}
		channelID := ledgerResources.ConfigtxValidator().ChannelID()

		if _, ok := ledgerResources.ConsortiumsConfig(); ok {
			if r.systemChannelID != "" {
				logger.Panicf("There appear to be two system channels %s and %s", r.systemChannelID, channelID)
			}

			chain, err := newChainSupport(
				r,
				ledgerResources,
				r.consenters,
				r.signer,
				r.blockcutterMetrics,
				r.bccsp,
			)
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
			iter, pos := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}})
			defer iter.Close()
			if pos != uint64(0) {
				logger.Panicf("Error iterating over system channel: '%s', expected position 0, got %d", channelID, pos)
			}
			genesisBlock, status := iter.Next()
			if status != cb.Status_SUCCESS {
				logger.Panicf("Error reading genesis block of system channel '%s'", channelID)
			}
			logger.Infof("Starting system channel '%s' with genesis block hash %x and orderer type %s",
				channelID, protoutil.BlockHeaderHash(genesisBlock.Header), chain.SharedConfig().ConsensusType())

			r.chains[channelID] = chain
			r.systemChannelID = channelID
			r.systemChannel = chain
			// We delay starting this channel, as it might try to copy and replace the channels map via newChannel before the map is fully built
			defer chain.start()
		} else {
			logger.Debugf("Starting channel: %s", channelID)
			chain, err := newChainSupport(
				r,
				ledgerResources,
				r.consenters,
				r.signer,
				r.blockcutterMetrics,
				r.bccsp,
			)
			if err != nil {
				logger.Panicf("Error creating chain support: %s", err)
			}
			r.chains[channelID] = chain
			chain.start()
		}
	}

	if r.systemChannelID == "" {
		logger.Infof("Registrar initializing without a system channel, number of application channels: %d", len(r.chains))
		if _, etcdRaftFound := r.consenters["etcdraft"]; !etcdRaftFound {
			logger.Panicf("Error initializing without a system channel: failed to find an etcdraft clusterConsenter")
		}
	}
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

	r.createNewChain(configtx)
}

func (r *Registrar) createNewChain(configtx *cb.Envelope) {
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

	logger.Infof("Created and starting new channel %s", chainID)
	cs.start()
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
	r.createNewChain(configTx(lf))
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

	configEnv, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "failed extracting config envelope from block")
	}

	//TODO save the join-block in the file repo to make this action crash tolerant.
	//TODO remove join block & ledger if things go bad below, (using defer that identifies an error?)

	ledgerRes, err := r.newLedgerResources(configEnv)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "failed creating ledger resources")
	}

	ordererConfig, _ := ledgerRes.OrdererConfig()
	consenter, foundConsenter := r.consenters[ordererConfig.ConsensusType()]
	if !foundConsenter {
		return types.ChannelInfo{}, errors.Errorf("failed to find a consenter for consensus type: %s", ordererConfig.ConsensusType())
	}

	clusterConsenter, ok := consenter.(consensus.ClusterConsenter)
	if !ok {
		return types.ChannelInfo{}, errors.New("clusterConsenter is not a consensus.ClusterConsenter")
	}
	isMember, err := clusterConsenter.IsChannelMember(configBlock)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "failed to determine cluster membership from join-block ")
	}

	if configBlock.Header.Number == 0 && isMember {
		return r.joinAsMember(ledgerRes, configBlock, channelID)
	}

	fChain, info, err := r.createFollower(ledgerRes, clusterConsenter, configBlock, channelID)
	if err != nil {
		return info, errors.Wrap(err, "failed to create follower")
	}

	fChain.Start()
	logger.Infof("Joining channel: %v", info)
	return info, err
}

func (r *Registrar) joinAsMember(ledgerRes *ledgerResources, configBlock *cb.Block, channelID string) (types.ChannelInfo, error) {
	err := ledgerRes.Append(configBlock)
	if err != nil {
		return types.ChannelInfo{}, errors.Wrap(err, "error appending join block to the ledger")
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
		return types.ChannelInfo{}, errors.Wrap(err, "error creating chain")
	}

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerRes.Height(),
	}
	info.ClusterRelation, info.Status = chain.StatusReport()

	r.chains[channelID] = chain
	chain.start()

	logger.Infof("Joining channel: %v", info)
	return info, nil
}

// createFollower created a follower.Chain, puts it in the map, but does not start it.
func (r *Registrar) createFollower(
	ledgerRes *ledgerResources,
	clusterConsenter consensus.ClusterConsenter,
	joinBlock *cb.Block,
	channelID string,
) (*follower.Chain, types.ChannelInfo, error) {
	logger := flogging.MustGetLogger("orderer.commmon.follower").With("channel", channelID)
	blockPullerCreator, err := follower.NewBlockPullerCreator(
		channelID, logger, r.signer, r.clusterDialer, r.config.General.Cluster, r.bccsp)
	if err != nil {
		return nil, types.ChannelInfo{}, errors.Wrapf(err, "failed to create BlockPullerFactory for channel %s", channelID)
	}

	fChain, err := follower.NewChain(
		ledgerRes,
		clusterConsenter,
		joinBlock,
		follower.Options{
			Logger: logger,
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

// RemoveChannel instructs the orderer to remove a channel.
// Depending on the removeStorage parameter, the storage resources are either removed or archived.
func (r *Registrar) RemoveChannel(channelID string, removeStorage bool) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.systemChannelID != "" {
		if channelID != r.systemChannelID {
			return types.ErrSystemChannelExists
		}
		// TODO FAB-17965
		return errors.New("not yet implemented")
	}

	cs, ok := r.chains[channelID]
	if ok {
		return r.removeMember(channelID, cs)
	}

	follower, ok := r.followers[channelID]
	if ok {
		return r.removeFollower(channelID, follower)
	}

	return types.ErrChannelNotExist
}

func (r *Registrar) removeMember(channelID string, cs *ChainSupport) error {
	cs.Halt()

	err := r.ledgerFactory.Remove(channelID)
	if err != nil {
		return errors.Errorf("error removing ledger for channel %s", channelID)
	}

	delete(r.chains, channelID)

	logger.Infof("Removed channel: %s", channelID)

	return nil
}

func (r *Registrar) removeFollower(channelID string, follower *follower.Chain) error {
	follower.Halt()

	err := r.ledgerFactory.Remove(channelID)
	if err != nil {
		return errors.Errorf("error removing ledger for channel %s", channelID)
	}

	delete(r.followers, channelID)

	logger.Infof("Removed channel: %s", channelID)

	return nil
}
