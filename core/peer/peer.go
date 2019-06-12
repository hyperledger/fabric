/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	cc "github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	fileledger "github.com/hyperledger/fabric/common/ledger/blockledger/file"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	vir "github.com/hyperledger/fabric/core/committer/txvalidator/v20/valinforetriever"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/api"
	gossipprivdata "github.com/hyperledger/fabric/gossip/privdata"
	gossipservice "github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var peerLogger = flogging.MustGetLogger("peer")

var peerServer *comm.GRPCServer

// singleton instance to manage credentials for the peer across channel config changes
var credSupport = comm.GetCredentialSupport()

type CollectionInfoShim struct {
	plugindispatcher.CollectionAndLifecycleResources
	ChannelID string
}

func (cis *CollectionInfoShim) CollectionValidationInfo(chaincodeName, collectionName string, validationState validation.State) ([]byte, error, error) {
	return cis.CollectionAndLifecycleResources.CollectionValidationInfo(cis.ChannelID, chaincodeName, collectionName, validationState)
}

type gossipSupport struct {
	channelconfig.Application
	configtx.Validator
	channelconfig.Channel
}

func capabilitiesSupportedOrPanic(res channelconfig.Resources) {
	ac, ok := res.ApplicationConfig()
	if !ok {
		peerLogger.Panicf("[channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChainID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChainID(), err)
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChainID(), err)
	}
}

// Channel is a local struct to manage objects in a Channel
type Channel struct {
	cb           *common.Block
	committer    committer.Committer
	ledger       ledger.PeerLedger
	bundleSource *channelconfig.BundleSource
	resources    channelconfig.Resources
}

// bundleUpdate is called by the bundleSource when the channel configuration
// changes.
func (c *Channel) bundleUpdate(b *channelconfig.Bundle) {
	c.resources = b
}

func (c *Channel) Ledger() ledger.PeerLedger {
	return c.ledger
}

func (c *Channel) Reader() blockledger.Reader {
	return fileledger.NewFileLedger(fileLedgerBlockStore{c.ledger})
}

// Errored returns a channel that can be used to determine
// if a backing resource has errored. At this point in time,
// the peer does not have any error conditions that lead to
// this function signaling that an error has occurred.
func (c *Channel) Errored() <-chan struct{} {
	// If this is ever updated to return a real channel, the error message
	// in deliver.go around this channel closing should be updated.
	return nil
}

func (c *Channel) PolicyManager() policies.Manager {
	return c.resources.PolicyManager()
}

// Sequence passes through to the underlying configtx.Validator
func (c *Channel) Sequence() uint64 {
	return c.resources.ConfigtxValidator().Sequence()
}

func (c *Channel) Apply(configtx *common.ConfigEnvelope) error {
	configTxValidator := c.resources.ConfigtxValidator()
	err := configTxValidator.Validate(configtx)
	if err != nil {
		return err
	}

	bundle, err := channelconfig.NewBundle(configTxValidator.ChainID(), configtx.Config)
	if err != nil {
		return err
	}

	channelconfig.LogSanityChecks(bundle)
	err = c.bundleSource.ValidateNew(bundle)
	if err != nil {
		return err
	}

	capabilitiesSupportedOrPanic(bundle)

	c.bundleSource.Update(bundle)
	return nil
}

func (c *Channel) Capabilities() channelconfig.ApplicationCapabilities {
	ac, ok := c.resources.ApplicationConfig()
	if !ok {
		return nil
	}
	return ac.Capabilities()
}

func (c *Channel) GetMSPIDs() []string {
	ac, ok := c.resources.ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	var mspIDs []string
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

func (c *Channel) MSPManager() msp.MSPManager {
	return c.resources.MSPManager()
}

func getCurrConfigBlockFromLedger(ledger ledger.PeerLedger) (*common.Block, error) {
	peerLogger.Debugf("Getting config block")

	// get last block.  Last block number is Height-1
	blockchainInfo, err := ledger.GetBlockchainInfo()
	if err != nil {
		return nil, err
	}
	lastBlock, err := ledger.GetBlockByNumber(blockchainInfo.Height - 1)
	if err != nil {
		return nil, err
	}

	// get most recent config block location from last block metadata
	configBlockIndex, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return nil, err
	}

	// get most recent config block
	configBlock, err := ledger.GetBlockByNumber(configBlockIndex)
	if err != nil {
		return nil, err
	}

	peerLogger.Debugf("Got config block[%d]", configBlockIndex)
	return configBlock, nil
}

// updates the trusted roots for the peer based on updates to channels
func updateTrustedRoots(cm channelconfig.Resources) {
	// this is triggered on per channel basis so first update the roots for the channel
	peerLogger.Debugf("Updating trusted root authorities for channel %s", cm.ConfigtxValidator().ChainID())

	// only run is TLS is enabled
	serverConfig, err := GetServerConfig()
	if err != nil || !serverConfig.SecOpts.UseTLS {
		// TODO: stop making calls to get the server configuratino
		return
	}

	buildTrustedRootsForChain(cm)

	// now iterate over all roots for all app and orderer channels
	credSupport.RLock()
	defer credSupport.RUnlock()

	var trustedRoots [][]byte
	for _, roots := range credSupport.AppRootCAsByChain {
		trustedRoots = append(trustedRoots, roots...)
	}
	trustedRoots = append(trustedRoots, serverConfig.SecOpts.ClientRootCAs...)
	trustedRoots = append(trustedRoots, serverConfig.SecOpts.ServerRootCAs...)

	server := peerServer
	if server == nil {
		return
	}

	// now update the client roots for the peerServer
	err = server.SetClientRootCAs(trustedRoots)
	if err != nil {
		msg := "Failed to update trusted roots from latest config block. " +
			"This peer may not be able to communicate with members of channel %s (%s)"
		peerLogger.Warningf(msg, cm.ConfigtxValidator().ChainID(), err)
	}
}

// populates the appRootCAs and orderRootCAs maps by getting the
// root and intermediate certs for all msps associated with the MSPManager
func buildTrustedRootsForChain(cm channelconfig.Resources) {
	credSupport.Lock()
	defer credSupport.Unlock()

	appOrgMSPs := make(map[string]struct{})
	if ac, ok := cm.ApplicationConfig(); ok {
		for _, appOrg := range ac.Organizations() {
			appOrgMSPs[appOrg.MSPID()] = struct{}{}
		}
	}

	ordOrgMSPs := make(map[string]struct{})
	if ac, ok := cm.OrdererConfig(); ok {
		for _, ordOrg := range ac.Organizations() {
			ordOrgMSPs[ordOrg.MSPID()] = struct{}{}
		}
	}

	cid := cm.ConfigtxValidator().ChainID()
	peerLogger.Debugf("updating root CAs for channel [%s]", cid)
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		peerLogger.Errorf("Error getting root CAs for channel %s (%s)", cid, err)
		return
	}

	var appRootCAs [][]byte
	var ordererRootCAs [][]byte
	for k, v := range msps {
		// we only support the fabric MSP
		if v.GetType() != msp.FABRIC {
			continue
		}

		for _, root := range v.GetTLSRootCerts() {
			// check to see of this is an app org MSP
			if _, ok := appOrgMSPs[k]; ok {
				peerLogger.Debugf("adding app root CAs for MSP [%s]", k)
				appRootCAs = append(appRootCAs, root)
			}
			// check to see of this is an orderer org MSP
			if _, ok := ordOrgMSPs[k]; ok {
				peerLogger.Debugf("adding orderer root CAs for MSP [%s]", k)
				ordererRootCAs = append(ordererRootCAs, root)
			}
		}
		for _, intermediate := range v.GetTLSIntermediateCerts() {
			// check to see of this is an app org MSP
			if _, ok := appOrgMSPs[k]; ok {
				peerLogger.Debugf("adding app root CAs for MSP [%s]", k)
				appRootCAs = append(appRootCAs, intermediate)
			}
			// check to see of this is an orderer org MSP
			if _, ok := ordOrgMSPs[k]; ok {
				peerLogger.Debugf("adding orderer root CAs for MSP [%s]", k)
				ordererRootCAs = append(ordererRootCAs, intermediate)
			}
		}
	}
	credSupport.AppRootCAsByChain[cid] = appRootCAs
	credSupport.OrdererRootCAsByChain[cid] = ordererRootCAs
}

// setCurrConfigBlock sets the current config block of the specified channel
func (p *Peer) setCurrConfigBlock(block *common.Block, cid string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if c, ok := p.channels[cid]; ok {
		c.cb = block
		return nil
	}
	return errors.Errorf("[channel %s] channel not associated with this peer", cid)
}

// NewChannelPolicyManagerGetter returns a new instance of ChannelPolicyManagerGetter
func NewChannelPolicyManagerGetter() policies.ChannelPolicyManagerGetter {
	return &channelPolicyManagerGetter{}
}

type channelPolicyManagerGetter struct{}

func (c *channelPolicyManagerGetter) Manager(channelID string) (policies.Manager, bool) {
	policyManager := GetPolicyManager(channelID)
	return policyManager, policyManager != nil
}

// NewPeerServer creates an instance of comm.GRPCServer
// This server is used for peer communications
func NewPeerServer(listenAddress string, serverConfig comm.ServerConfig) (*comm.GRPCServer, error) {
	var err error
	peerServer, err = comm.NewGRPCServer(listenAddress, serverConfig)
	if err != nil {
		peerLogger.Errorf("Failed to create peer server (%s)", err)
		return nil, err
	}
	return peerServer, nil
}

//
//  Deliver service support structs for the peer
//

// DeliverChainManager provides access to a channel for performing deliver
type DeliverChainManager struct{}

func (DeliverChainManager) GetChain(chainID string) deliver.Chain {
	Default.mutex.RLock()
	defer Default.mutex.RUnlock()
	channel, ok := Default.channels[chainID]
	if !ok {
		return nil
	}
	return channel
}

// fileLedgerBlockStore implements the interface expected by
// common/ledger/blockledger/file to interact with a file ledger for deliver
type fileLedgerBlockStore struct {
	ledger.PeerLedger
}

func (flbs fileLedgerBlockStore) AddBlock(*common.Block) error {
	return nil
}

func (flbs fileLedgerBlockStore) RetrieveBlocks(startBlockNumber uint64) (commonledger.ResultsIterator, error) {
	return flbs.GetBlocksIterator(startBlockNumber)
}

// NewConfigSupport returns
func NewConfigSupport() cc.Manager {
	return &configSupport{}
}

type configSupport struct{}

// GetChannelConfig returns an instance of a object that represents
// current channel configuration tree of the specified channel. The
// ConfigProto method of the returned object can be used to get the
// proto representing the channel configuration.
func (*configSupport) GetChannelConfig(cid string) cc.Config {
	Default.mutex.RLock()
	defer Default.mutex.RUnlock()
	channel := Default.channels[cid]
	if channel == nil {
		peerLogger.Errorf("[channel %s] channel not associated with this peer", cid)
		return nil
	}
	return channel.bundleSource.ConfigtxValidator()
}

// Operations exposes an interface to the package level functions that operated
// on singletons in the package. This is a step towards moving from package
// level data for the peer to instance level data.
type Operations interface {
	GetApplicationConfig(cid string) (channelconfig.Application, bool)
	GetLedger(cid string) ledger.PeerLedger
	GetPolicyManager(cid string) policies.Manager
}

type Peer struct {
	StoreProvider transientstore.StoreProvider
	storesMutex   sync.RWMutex
	stores        map[string]transientstore.Store

	GossipService *gossipservice.GossipService

	// validationWorkersSemaphore is used to limit the number of concurrent validation
	// go routines.
	validationWorkersSemaphore semaphore.Semaphore

	pluginMapper       plugin.Mapper
	channelInitializer func(cid string)

	// channels is a map of channelID to channel
	mutex    sync.RWMutex
	channels map[string]*Channel
}

// Default provides in implementation of the Peer that provides
// access to the package level state.
var Default *Peer = &Peer{}

func (p *Peer) StoreForChannel(cid string) transientstore.Store {
	p.storesMutex.RLock()
	defer p.storesMutex.RUnlock()
	return p.stores[cid]
}

func (p *Peer) openStore(cid string) (transientstore.Store, error) {
	p.storesMutex.Lock()
	defer p.storesMutex.Unlock()

	store, err := p.StoreProvider.OpenStore(cid)
	if err != nil {
		return nil, err
	}

	if p.stores == nil {
		p.stores = map[string]transientstore.Store{}
	}

	p.stores[cid] = store
	return store, nil
}

func (p *Peer) CreateChannel(
	cb *common.Block,
	sccp sysccprovider.SystemChaincodeProvider,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	legacyLifecycleValidation plugindispatcher.LifecycleResources,
	newLifecycleValidation plugindispatcher.CollectionAndLifecycleResources,
) error {
	cid, err := protoutil.GetChainIDFromBlock(cb)
	if err != nil {
		return err
	}

	l, err := ledgermgmt.CreateLedger(cb)
	if err != nil {
		return errors.WithMessage(err, "cannot create ledger from genesis block")
	}

	if err := p.createChannel(cid, l, cb, sccp, p.pluginMapper, deployedCCInfoProvider, legacyLifecycleValidation, newLifecycleValidation); err != nil {
		return err
	}

	p.initChannel(cid)
	return nil
}

// createChannel creates a new channel object and insert it into the channels slice.
func (p *Peer) createChannel(
	cid string,
	l ledger.PeerLedger,
	cb *common.Block,
	sccp sysccprovider.SystemChaincodeProvider,
	pluginMapper plugin.Mapper,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	legacyLifecycleValidation plugindispatcher.LifecycleResources,
	newLifecycleValidation plugindispatcher.CollectionAndLifecycleResources,
) error {
	chanConf, err := retrievePersistedChannelConfig(l)
	if err != nil {
		return err
	}

	var bundle *channelconfig.Bundle
	if chanConf != nil {
		bundle, err = channelconfig.NewBundle(cid, chanConf)
		if err != nil {
			return err
		}
	} else {
		// Config was only stored in the statedb starting with v1.1 binaries
		// so if the config is not found there, extract it manually from the config block
		envelopeConfig, err := protoutil.ExtractEnvelope(cb, 0)
		if err != nil {
			return err
		}

		bundle, err = channelconfig.NewBundleFromEnvelope(envelopeConfig)
		if err != nil {
			return err
		}
	}

	capabilitiesSupportedOrPanic(bundle)

	channelconfig.LogSanityChecks(bundle)

	gossipEventer := p.GossipService.NewConfigEventer()

	gossipCallbackWrapper := func(bundle *channelconfig.Bundle) {
		ac, ok := bundle.ApplicationConfig()
		if !ok {
			// TODO, handle a missing ApplicationConfig more gracefully
			ac = nil
		}
		gossipEventer.ProcessConfigUpdate(&gossipSupport{
			Validator:   bundle.ConfigtxValidator(),
			Application: ac,
			Channel:     bundle.ChannelConfig(),
		})
		p.GossipService.SuspectPeers(func(identity api.PeerIdentityType) bool {
			// TODO: this is a place-holder that would somehow make the MSP layer suspect
			// that a given certificate is revoked, or its intermediate CA is revoked.
			// In the meantime, before we have such an ability, we return true in order
			// to suspect ALL identities in order to validate all of them.
			return true
		})
	}

	trustedRootsCallbackWrapper := func(bundle *channelconfig.Bundle) {
		updateTrustedRoots(bundle)
	}

	mspCallback := func(bundle *channelconfig.Bundle) {
		// TODO remove once all references to mspmgmt are gone from peer code
		mspmgmt.XXXSetMSPManager(cid, bundle.MSPManager())
	}

	committer := committer.NewLedgerCommitterReactive(l, func(block *common.Block) error {
		chainID, err := protoutil.GetChainIDFromBlock(block)
		if err != nil {
			return err
		}
		return p.setCurrConfigBlock(block, chainID)
	})

	c := &Channel{
		cb:        cb,
		committer: committer,
		ledger:    l,
		resources: bundle,
	}

	c.bundleSource = channelconfig.NewBundleSource(
		bundle,
		gossipCallbackWrapper,
		trustedRootsCallbackWrapper,
		mspCallback,
		c.bundleUpdate,
	)

	validator := txvalidator.NewTxValidator(
		cid,
		p.validationWorkersSemaphore,
		c,
		&vir.ValidationInfoRetrieveShim{
			New:    newLifecycleValidation,
			Legacy: legacyLifecycleValidation,
		},
		&CollectionInfoShim{
			CollectionAndLifecycleResources: newLifecycleValidation,
			ChannelID:                       bundle.ConfigtxValidator().ChainID(),
		},
		sccp,
		p.pluginMapper,
		NewChannelPolicyManagerGetter(),
	)

	ordererAddresses := bundle.ChannelConfig().OrdererAddresses()
	if len(ordererAddresses) == 0 {
		return errors.New("no ordering service endpoint provided in configuration block")
	}

	// TODO: does someone need to call Close() on the transientStoreFactory at shutdown of the peer?
	store, err := p.openStore(bundle.ConfigtxValidator().ChainID())
	if err != nil {
		return errors.Wrapf(err, "[channel %s] failed opening transient store", bundle.ConfigtxValidator().ChainID())
	}

	simpleCollectionStore := privdata.NewSimpleCollectionStore(l, deployedCCInfoProvider)
	p.GossipService.InitializeChannel(bundle.ConfigtxValidator().ChainID(), ordererAddresses, gossipservice.Support{
		Validator: validator,
		Committer: committer,
		Store:     store,
		Cs:        simpleCollectionStore,
		IdDeserializeFactory: gossipprivdata.IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
			return mspmgmt.GetManagerForChain(chainID)
		}),
	})

	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.channels == nil {
		p.channels = map[string]*Channel{}
	}
	p.channels[cid] = c

	return nil
}

// GetChannelConfig returns the channel configuration of the channel with channel ID. Note that this
// call returns nil if channel cid has not been created.
func (p *Peer) GetChannelConfig(cid string) channelconfig.Resources {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if c, ok := p.channels[cid]; ok {
		return c.resources
	}
	return nil
}

// GetChannelsInfo returns an array with information about all channels for
// this peer.
func (p *Peer) GetChannelsInfo() []*pb.ChannelInfo {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	var channelInfos []*pb.ChannelInfo
	for key := range p.channels {
		ci := &pb.ChannelInfo{ChannelId: key}
		channelInfos = append(channelInfos, ci)
	}
	return channelInfos
}

// GetStableChannelConfig returns the stable channel configuration of the channel with channel ID.
// Note that this call returns nil if channel cid has not been created.
func (p *Peer) GetStableChannelConfig(cid string) channelconfig.Resources {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if c, ok := p.channels[cid]; ok {
		return c.bundleSource.StableBundle()
	}
	return nil
}

// GetCurrConfigBlock returns the cached config block of the specified channel.
// Note that this call returns nil if channel cid has not been created.
func (p *Peer) GetCurrConfigBlock(cid string) *common.Block {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if c, ok := p.channels[cid]; ok {
		return c.cb
	}
	return nil
}

// GetLedger returns the ledger of the channel with channel ID. Note that this
// call returns nil if channel cid has not been created.
func GetLedger(cid string) ledger.PeerLedger { return Default.GetLedger(cid) }
func (p *Peer) GetLedger(cid string) ledger.PeerLedger {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if c, ok := p.channels[cid]; ok {
		return c.ledger
	}
	return nil
}

// GetMSPIDs returns the ID of each application MSP defined on this channel
func GetMSPIDs(cid string) []string { return Default.GetMSPIDs(cid) }
func (p *Peer) GetMSPIDs(cid string) []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	c, ok := p.channels[cid]
	if !ok {
		return nil
	}

	return c.GetMSPIDs()
}

// GetPolicyManager returns the policy manager of the channel with channel ID. Note that this
// call returns nil if channel cid has not been created.
func GetPolicyManager(cid string) policies.Manager { return Default.GetPolicyManager(cid) }
func (p *Peer) GetPolicyManager(cid string) policies.Manager {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if c, ok := p.channels[cid]; ok {
		return c.resources.PolicyManager()
	}
	return nil
}

// initChannel takes care to initialize channel after peer joined, for example deploys system CCs
func (p *Peer) initChannel(cid string) {
	if p.channelInitializer != nil {
		// Initialize chaincode, namely deploy system CC
		peerLogger.Debugf("Initializing channel %s", cid)
		p.channelInitializer(cid)
	}
}

func (p *Peer) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	cc := p.GetChannelConfig(cid)
	if cc == nil {
		return nil, false
	}

	return cc.ApplicationConfig()
}

// Initialize sets up any channels that the peer has from the persistence. This
// function should be called at the start up when the ledger and gossip
// ready
func (p *Peer) Initialize(
	init func(string),
	sccp sysccprovider.SystemChaincodeProvider,
	pm plugin.Mapper,
	pr *platforms.Registry,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	membershipProvider ledger.MembershipInfoProvider,
	metricsProvider metrics.Provider,
	legacyLifecycleValidation plugindispatcher.LifecycleResources,
	newLifecycleValidation plugindispatcher.CollectionAndLifecycleResources,
	ledgerConfig *ledger.Config,
	nWorkers int,
	txProcessors customtx.Processors,
) {
	// TODO: exported dep fields or constructor
	p.validationWorkersSemaphore = semaphore.New(nWorkers)
	p.pluginMapper = pm
	p.channelInitializer = init

	var cb *common.Block
	var ledger ledger.PeerLedger
	ledgermgmt.Initialize(&ledgermgmt.Initializer{
		CustomTxProcessors:            txProcessors,
		PlatformRegistry:              pr,
		DeployedChaincodeInfoProvider: deployedCCInfoProvider,
		MembershipInfoProvider:        membershipProvider,
		MetricsProvider:               metricsProvider,
		Config:                        ledgerConfig,
	})
	ledgerIds, err := ledgermgmt.GetLedgerIDs()
	if err != nil {
		panic(fmt.Errorf("error in initializing ledgermgmt: %s", err))
	}
	for _, cid := range ledgerIds {
		peerLogger.Infof("Loading chain %s", cid)
		if ledger, err = ledgermgmt.OpenLedger(cid); err != nil {
			peerLogger.Errorf("Failed to load ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while loading ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		if cb, err = getCurrConfigBlockFromLedger(ledger); err != nil {
			peerLogger.Errorf("Failed to find config block on ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while looking for config block on ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		// Create a chain if we get a valid ledger with config block
		if err = p.createChannel(cid, ledger, cb, sccp, pm, deployedCCInfoProvider, legacyLifecycleValidation, newLifecycleValidation); err != nil {
			peerLogger.Errorf("Failed to load chain %s(%s)", cid, err)
			peerLogger.Debugf("Error reloading chain %s with message %s. We continue to the next chain rather than abort.", cid, err)
			continue
		}

		p.initChannel(cid)
	}
}
