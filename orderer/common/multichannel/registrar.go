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

// checkResources makes sure that the channel config is compatible with this binary and logs sanity checks
func checkResources(res channelconfig.Resources) error {
	channelconfig.LogSanityChecks(res)
	oc, ok := res.OrdererConfig()
	if !ok {
		return errors.New("config does not contain orderer config")
	}
	if err := oc.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "config requires unsupported orderer capabilities: %s", err)
	}
	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "config requires unsupported channel capabilities: %s", err)
	}
	return nil
}

// checkResourcesOrPanic invokes checkResources and panics if an error is returned
func checkResourcesOrPanic(res channelconfig.Resources) {
	if err := checkResources(res); err != nil {
		logger.Panicf("[channel %s] %s", res.ConfigtxValidator().ChannelID(), err)
	}
}

type mutableResources interface {
	channelconfig.Resources
	Update(*channelconfig.Bundle)
}

type configResources struct {
	mutableResources
	bccsp bccsp.BCCSP
}

func (cr *configResources) CreateBundle(channelID string, config *cb.Config) (*channelconfig.Bundle, error) {
	return channelconfig.NewBundle(channelID, config, cr.bccsp)
}

func (cr *configResources) Update(bndl *channelconfig.Bundle) {
	checkResourcesOrPanic(bndl)
	cr.mutableResources.Update(bndl)
}

func (cr *configResources) SharedConfig() channelconfig.Orderer {
	oc, ok := cr.OrdererConfig()
	if !ok {
		logger.Panicf("[channel %s] has no orderer configuration", cr.ConfigtxValidator().ChannelID())
	}
	return oc
}

type ledgerResources struct {
	*configResources
	blockledger.ReadWriter
}

// Registrar serves as a point of access and control for the individual channel resources.
type Registrar struct {
	config localconfig.TopLevel
	lock   sync.RWMutex
	chains map[string]*ChainSupport

	consenters         map[string]consensus.Consenter
	ledgerFactory      blockledger.Factory
	signer             identity.SignerSerializer
	blockcutterMetrics *blockcutter.Metrics
	systemChannelID    string
	systemChannel      *ChainSupport
	templator          msgprocessor.ChannelConfigTemplator
	callbacks          []channelconfig.BundleActor
	bccsp              bccsp.BCCSP
}

// ConfigBlock retrieves the last configuration block from the given ledger.
// Panics on failure.
func ConfigBlock(reader blockledger.Reader) *cb.Block {
	lastBlock, err := blockledger.GetBlockByNumber(reader, reader.Height()-1)
	if err != nil {
		logger.Panicw("Failed to retrieve block", "blockNum", reader.Height()-1, "error", err)
	}
	index, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicw("Chain did not have appropriately encoded last config in its latest block", "error", err)
	}
	configBlock, err := blockledger.GetBlockByNumber(reader, index)
	if err != nil {
		logger.Panicw("Failed to retrieve config block", "blockNum", index, "error", err)
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
	callbacks ...channelconfig.BundleActor,
) *Registrar {
	r := &Registrar{
		config:             config,
		chains:             make(map[string]*ChainSupport),
		ledgerFactory:      ledgerFactory,
		signer:             signer,
		blockcutterMetrics: blockcutter.NewMetrics(metricsProvider),
		callbacks:          callbacks,
		bccsp:              bccsp,
	}

	return r
}

func (r *Registrar) Initialize(consenters map[string]consensus.Consenter) {
	r.consenters = consenters
	existingChannels := r.ledgerFactory.ChannelIDs()

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
			logger.Panicf("Error initializing without a system channel: failed to find an etcdraft consenter")
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

// CreateChain makes the Registrar create a chain with the given name.
func (r *Registrar) CreateChain(chainName string) {
	lf, err := r.ledgerFactory.GetOrCreate(chainName)
	if err != nil {
		logger.Panicf("Failed obtaining ledger factory for %s: %v", chainName, err)
	}
	chain := r.GetChain(chainName)
	if chain != nil {
		logger.Infof("A chain of type %T for channel %s already exists. "+
			"Halting it.", chain.Chain, chainName)
		r.lock.Lock()
		chain.Halt()
		delete(r.chains, chainName)
		r.lock.Unlock()
	}
	r.newChain(configTx(lf))
}

func (r *Registrar) newChain(configtx *cb.Envelope) {
	r.lock.Lock()
	defer r.lock.Unlock()

	channelName, err := channelNameFromConfigTx(configtx)
	if err != nil {
		logger.Warnf("Failed extracting channel name: %v", err)
		return
	}

	// fixes https://github.com/hyperledger/fabric/issues/2931
	if existingChain, exists := r.chains[channelName]; exists {
		if raftChain, isRaftChain := existingChain.Chain.(RaftChain); isRaftChain && raftChain.IsRaft() {
			logger.Infof("Channel %s already created, skipping its creation", channelName)
			return
		}
	}

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

// ChannelsCount returns the count of the current total number of channels.
func (r *Registrar) ChannelsCount() int {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return len(r.chains)
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

	if len(r.chains) == 0 {
		return list
	}

	if r.systemChannelID != "" {
		list.SystemChannel = &types.ChannelInfoShort{Name: r.systemChannelID}
	}
	for name := range r.chains {
		if name == r.systemChannelID {
			continue
		}
		list.Channels = append(list.Channels, types.ChannelInfoShort{Name: name})
	}

	return list
}

func (r *Registrar) ChannelInfo(channelID string) (types.ChannelInfo, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	info := types.ChannelInfo{}
	cs, ok := r.chains[channelID]
	if !ok {
		return info, types.ErrChannelNotExist
	}

	info.Name = channelID
	info.Height = cs.Height()
	info.ClusterRelation, info.Status = cs.StatusReport()

	return info, nil
}

func (r *Registrar) JoinChannel(channelID string, configBlock *cb.Block, isAppChannel bool) (types.ChannelInfo, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.systemChannelID != "" {
		return types.ChannelInfo{}, types.ErrSystemChannelExists
	}

	_, ok := r.chains[channelID]
	if ok {
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

	ledgerResources, err := r.newLedgerResources(configEnv)
	if err != nil {
		//TODO remove join block
		return types.ChannelInfo{}, errors.Wrap(err, "failed creating ledger resources")
	}

	joinSupport, err := newChainSupportForJoin(configBlock, r, ledgerResources, r.consenters, r.signer, r.blockcutterMetrics, r.bccsp)
	if err != nil {
		//TODO remove join block, clean ledger resources
		return types.ChannelInfo{}, errors.Wrap(err, "failed creating chain support for join")
	}

	info := types.ChannelInfo{
		Name:   channelID,
		URL:    "",
		Height: ledgerResources.Height(),
	}
	info.ClusterRelation, info.Status = joinSupport.StatusReport()

	logger.Infof("Joining new channel %s", channelID)

	r.chains[channelID] = joinSupport
	joinSupport.start()

	return info, nil
}

func (r *Registrar) RemoveChannel(channelID string, removeStorage bool) error {
	//TODO
	return errors.New("Not implemented yet")
}

type RaftChain interface {
	IsRaft() bool
}

func channelNameFromConfigTx(configtx *cb.Envelope) (string, error) {
	payload, err := protoutil.UnmarshalPayload(configtx.Payload)
	if err != nil {
		return "", errors.WithMessage(err, "error umarshaling envelope to payload")
	}

	if payload.Header == nil {
		return "", errors.New("missing channel header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", errors.WithMessage(err, "error unmarshalling channel header")
	}

	return chdr.ChannelId, nil
}
