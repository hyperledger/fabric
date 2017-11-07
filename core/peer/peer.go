/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"net"
	"runtime"
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
)

var peerLogger = flogging.MustGetLogger("peer")

var peerServer comm.GRPCServer

// singleton instance to manage credentials for the peer across channel config changes
var credSupport = comm.GetCredentialSupport()

type gossipSupport struct {
	channelconfig.Application
	configtx.Validator
}

type chainSupport struct {
	bundleSource *resourcesconfig.BundleSource
	channelconfig.Resources
	channelconfig.Application
	ledger ledger.PeerLedger
}

var transientStoreFactory = &storeProvider{}

type storeProvider struct {
	transientstore.StoreProvider
	sync.Mutex
}

func (sp *storeProvider) OpenStore(ledgerID string) (transientstore.Store, error) {
	sp.Lock()
	defer sp.Unlock()
	if sp.StoreProvider == nil {
		sp.StoreProvider = transientstore.NewStoreProvider()
	}
	return sp.StoreProvider.OpenStore(ledgerID)
}

func (cs *chainSupport) Apply(configtx *common.ConfigEnvelope) error {
	err := cs.ConfigtxValidator().Validate(configtx)
	if err != nil {
		return err
	}

	// If the chainSupport is being mocked, this field will be nil
	if cs.bundleSource != nil {
		bundle, err := channelconfig.NewBundle(cs.ConfigtxValidator().ChainID(), configtx.Config)
		if err != nil {
			return err
		}

		channelconfig.LogSanityChecks(bundle)

		err = cs.bundleSource.ChannelConfig().ValidateNew(bundle)
		if err != nil {
			return err
		}

		capabilitiesSupportedOrPanic(bundle)

		rBundle, err := cs.bundleSource.NewFromChannelConfig(bundle)
		if err != nil {
			return err
		}

		cs.bundleSource.Update(rBundle)
	}
	return nil
}

func capabilitiesSupportedOrPanic(res channelconfig.Resources) {
	ac, ok := res.ApplicationConfig()
	if !ok {
		peerLogger.Panicf("[channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChainID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible %s", res.ConfigtxValidator(), err)
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible %s", res.ConfigtxValidator(), err)
	}
}

func (cs *chainSupport) Ledger() ledger.PeerLedger {
	return cs.ledger
}

func (cs *chainSupport) GetMSPIDs(cid string) []string {
	return GetMSPIDs(cid)
}

// chain is a local struct to manage objects in a chain
type chain struct {
	cs        *chainSupport
	cb        *common.Block
	committer committer.Committer
}

// chains is a local map of chainID->chainObject
var chains = struct {
	sync.RWMutex
	list map[string]*chain
}{list: make(map[string]*chain)}

//MockInitialize resets chains for test env
func MockInitialize() {
	ledgermgmt.InitializeTestEnv()
	chains.list = nil
	chains.list = make(map[string]*chain)
	chainInitializer = func(string) { return }
}

var chainInitializer func(string)

var mockMSPIDGetter func(string) []string

func MockSetMSPIDGetter(mspIDGetter func(string) []string) {
	mockMSPIDGetter = mspIDGetter
}

// validationWorkersSemaphore is the semaphore used to ensure that
// there are not too many concurrent tx validation goroutines
var validationWorkersSemaphore *semaphore.Weighted

// Initialize sets up any chains that the peer has from the persistence. This
// function should be called at the start up when the ledger and gossip
// ready
func Initialize(init func(string)) {
	nWorkers := viper.GetInt("peer.validatorPoolSize")
	if nWorkers <= 0 {
		nWorkers = runtime.NumCPU()
	}
	validationWorkersSemaphore = semaphore.NewWeighted(int64(nWorkers))

	chainInitializer = init

	var cb *common.Block
	var ledger ledger.PeerLedger
	ledgermgmt.Initialize(nil)
	ledgerIds, err := ledgermgmt.GetLedgerIDs()
	if err != nil {
		panic(fmt.Errorf("Error in initializing ledgermgmt: %s", err))
	}
	for _, cid := range ledgerIds {
		peerLogger.Infof("Loading chain %s", cid)
		if ledger, err = ledgermgmt.OpenLedger(cid); err != nil {
			peerLogger.Warningf("Failed to load ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while loading ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		if cb, err = getCurrConfigBlockFromLedger(ledger); err != nil {
			peerLogger.Warningf("Failed to find config block on ledger %s(%s)", cid, err)
			peerLogger.Debugf("Error while looking for config block on ledger %s with message %s. We continue to the next ledger rather than abort.", cid, err)
			continue
		}
		// Create a chain if we get a valid ledger with config block
		if err = createChain(cid, ledger, cb); err != nil {
			peerLogger.Warningf("Failed to load chain %s(%s)", cid, err)
			peerLogger.Debugf("Error reloading chain %s with message %s. We continue to the next chain rather than abort.", cid, err)
			continue
		}

		InitChain(cid)
	}
}

// InitChain takes care to initialize chain after peer joined, for example deploys system CCs
func InitChain(cid string) {
	if chainInitializer != nil {
		// Initialize chaincode, namely deploy system CC
		peerLogger.Debugf("Init chain %s", cid)
		chainInitializer(cid)
	}
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
	configBlockIndex, err := utils.GetLastConfigIndexFromBlock(lastBlock)
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

// createChain creates a new chain object and insert it into the chains
func createChain(cid string, ledger ledger.PeerLedger, cb *common.Block) error {

	envelopeConfig, err := utils.ExtractEnvelope(cb, 0)
	if err != nil {
		return err
	}

	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig)
	if err != nil {
		return err
	}

	capabilitiesSupportedOrPanic(bundle)

	channelconfig.LogSanityChecks(bundle)

	gossipEventer := service.GetGossipService().NewConfigEventer()

	gossipCallbackWrapper := func(bundle *resourcesconfig.Bundle) {
		ac, ok := bundle.ChannelConfig().ApplicationConfig()
		if !ok {
			// TODO, handle a missing ApplicationConfig more gracefully
			ac = nil
		}
		gossipEventer.ProcessConfigUpdate(&gossipSupport{
			Validator:   bundle.ChannelConfig().ConfigtxValidator(),
			Application: ac,
		})
		service.GetGossipService().SuspectPeers(func(identity api.PeerIdentityType) bool {
			// TODO: this is a place-holder that would somehow make the MSP layer suspect
			// that a given certificate is revoked, or its intermediate CA is revoked.
			// In the meantime, before we have such an ability, we return true in order
			// to suspect ALL identities in order to validate all of them.
			return true
		})
	}

	trustedRootsCallbackWrapper := func(bundle *resourcesconfig.Bundle) {
		updateTrustedRoots(bundle.ChannelConfig())
	}

	mspCallback := func(bundle *resourcesconfig.Bundle) {
		// TODO remove once all references to mspmgmt are gone from peer code
		mspmgmt.XXXSetMSPManager(cid, bundle.ChannelConfig().MSPManager())
	}

	ac, ok := bundle.ApplicationConfig()
	if !ok {
		ac = nil
	}
	cs := &chainSupport{
		Application: ac, // TODO, refactor as this is accessible through Manager
		ledger:      ledger,
	}

	peerSingletonCallback := func(bundle *resourcesconfig.Bundle) {
		ac, ok := bundle.ChannelConfig().ApplicationConfig()
		if !ok {
			ac = nil
		}
		cs.Application = ac
		cs.Resources = bundle.ChannelConfig()
	}

	// TODO, actually use the seed data from the genesis block to bootstrap the resources config
	rBundle, err := resourcesconfig.NewBundle(cid, &common.Config{ChannelGroup: &common.ConfigGroup{}}, bundle)
	if err != nil {
		return err
	}

	cs.bundleSource = resourcesconfig.NewBundleSource(
		rBundle,
		gossipCallbackWrapper,
		trustedRootsCallbackWrapper,
		mspCallback,
		peerSingletonCallback,
	)

	vcs := struct {
		*chainSupport
		*semaphore.Weighted
	}{cs, validationWorkersSemaphore}
	validator := txvalidator.NewTxValidator(vcs)
	c := committer.NewLedgerCommitterReactive(ledger, func(block *common.Block) error {
		chainID, err := utils.GetChainIDFromBlock(block)
		if err != nil {
			return err
		}
		return SetCurrConfigBlock(block, chainID)
	})

	ordererAddresses := bundle.ChannelConfig().OrdererAddresses()
	if len(ordererAddresses) == 0 {
		return errors.New("No ordering service endpoint provided in configuration block")
	}

	// TODO: does someone need to call Close() on the transientStoreFactory at shutdown of the peer?
	store, err := transientStoreFactory.OpenStore(bundle.ConfigtxValidator().ChainID())
	if err != nil {
		return errors.Wrapf(err, "Failed opening transient store for %s", bundle.ConfigtxValidator().ChainID())
	}
	service.GetGossipService().InitializeChannel(bundle.ConfigtxValidator().ChainID(), ordererAddresses, service.Support{
		Validator: validator,
		Committer: c,
		Store:     store,
		Cs:        &privdata.NopCollectionStore{},
	})

	chains.Lock()
	defer chains.Unlock()
	chains.list[cid] = &chain{
		cs:        cs,
		cb:        cb,
		committer: c,
	}

	return nil
}

// CreateChainFromBlock creates a new chain from config block
func CreateChainFromBlock(cb *common.Block) error {
	cid, err := utils.GetChainIDFromBlock(cb)
	if err != nil {
		return err
	}

	var l ledger.PeerLedger
	if l, err = ledgermgmt.CreateLedger(cb); err != nil {
		return fmt.Errorf("Cannot create ledger from genesis block, due to %s", err)
	}

	return createChain(cid, l, cb)
}

// MockCreateChain used for creating a ledger for a chain for tests
// without having to join
func MockCreateChain(cid string) error {
	var ledger ledger.PeerLedger
	var err error

	if ledger = GetLedger(cid); ledger == nil {
		gb, _ := configtxtest.MakeGenesisBlock(cid)
		if ledger, err = ledgermgmt.CreateLedger(gb); err != nil {
			return err
		}
	}

	chains.Lock()
	defer chains.Unlock()

	chains.list[cid] = &chain{
		cs: &chainSupport{
			Resources: &mockchannelconfig.Resources{
				PolicyManagerVal: &mockpolicies.Manager{
					Policy: &mockpolicies.Policy{},
				},
				ConfigtxValidatorVal: &mockconfigtx.Validator{},
			},
			ledger: ledger},
	}

	return nil
}

// GetLedger returns the ledger of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetLedger(cid string) ledger.PeerLedger {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.ledger
	}
	return nil
}

// GetChannelConfig returns the channel configuration of the chain with channel ID. Note that this
// call returns nil if chain cid has not been created.
func GetChannelConfig(cid string) channelconfig.Resources {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs
	}
	return nil
}

// GetPolicyManager returns the policy manager of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetPolicyManager(cid string) policies.Manager {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.PolicyManager()
	}
	return nil
}

// GetCurrConfigBlock returns the cached config block of the specified chain.
// Note that this call returns nil if chain cid has not been created.
func GetCurrConfigBlock(cid string) *common.Block {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cb
	}
	return nil
}

// updates the trusted roots for the peer based on updates to channels
func updateTrustedRoots(cm channelconfig.Resources) {
	// this is triggered on per channel basis so first update the roots for the channel
	peerLogger.Debugf("Updating trusted root authorities for channel %s", cm.ConfigtxValidator().ChainID())
	var secureConfig comm.SecureServerConfig
	var err error
	// only run is TLS is enabled
	secureConfig, err = GetSecureConfig()
	if err == nil && secureConfig.UseTLS {
		buildTrustedRootsForChain(cm)

		// now iterate over all roots for all app and orderer chains
		trustedRoots := [][]byte{}
		credSupport.RLock()
		defer credSupport.RUnlock()
		for _, roots := range credSupport.AppRootCAsByChain {
			trustedRoots = append(trustedRoots, roots...)
		}
		// also need to append statically configured root certs
		if len(secureConfig.ClientRootCAs) > 0 {
			trustedRoots = append(trustedRoots, secureConfig.ClientRootCAs...)
		}
		if len(secureConfig.ServerRootCAs) > 0 {
			trustedRoots = append(trustedRoots, secureConfig.ServerRootCAs...)
		}

		server := GetPeerServer()
		// now update the client roots for the peerServer
		if server != nil {
			err := server.SetClientRootCAs(trustedRoots)
			if err != nil {
				msg := "Failed to update trusted roots for peer from latest config " +
					"block.  This peer may not be able to communicate " +
					"with members of channel %s (%s)"
				peerLogger.Warningf(msg, cm.ConfigtxValidator().ChainID(), err)
			}
		}
	}
}

// populates the appRootCAs and orderRootCAs maps by getting the
// root and intermediate certs for all msps associated with the MSPManager
func buildTrustedRootsForChain(cm channelconfig.Resources) {
	credSupport.Lock()
	defer credSupport.Unlock()

	appRootCAs := [][]byte{}
	ordererRootCAs := [][]byte{}
	appOrgMSPs := make(map[string]struct{})
	ordOrgMSPs := make(map[string]struct{})

	if ac, ok := cm.ApplicationConfig(); ok {
		//loop through app orgs and build map of MSPIDs
		for _, appOrg := range ac.Organizations() {
			appOrgMSPs[appOrg.MSPID()] = struct{}{}
		}
	}

	if ac, ok := cm.OrdererConfig(); ok {
		//loop through orderer orgs and build map of MSPIDs
		for _, ordOrg := range ac.Organizations() {
			ordOrgMSPs[ordOrg.MSPID()] = struct{}{}
		}
	}

	cid := cm.ConfigtxValidator().ChainID()
	peerLogger.Debugf("updating root CAs for channel [%s]", cid)
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		peerLogger.Errorf("Error getting root CAs for channel %s (%s)", cid, err)
	}
	if err == nil {
		for k, v := range msps {
			// check to see if this is a FABRIC MSP
			if v.GetType() == msp.FABRIC {
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
		}
		credSupport.AppRootCAsByChain[cid] = appRootCAs
		credSupport.OrdererRootCAsByChain[cid] = ordererRootCAs
	}
}

// GetMSPIDs returns the ID of each application MSP defined on this chain
func GetMSPIDs(cid string) []string {
	chains.RLock()
	defer chains.RUnlock()

	//if mock is set, use it to return MSPIDs
	//used for tests without a proper join
	if mockMSPIDGetter != nil {
		return mockMSPIDGetter(cid)
	}
	if c, ok := chains.list[cid]; ok {
		if c == nil || c.cs == nil {
			return nil
		}
		ac, ok := c.cs.ApplicationConfig()
		if !ok || ac.Organizations() == nil {
			return nil
		}

		orgs := ac.Organizations()
		toret := make([]string, len(orgs))
		i := 0
		for _, org := range orgs {
			toret[i] = org.MSPID()
			i++
		}

		return toret
	}
	return nil
}

// SetCurrConfigBlock sets the current config block of the specified chain
func SetCurrConfigBlock(block *common.Block, cid string) error {
	chains.Lock()
	defer chains.Unlock()
	if c, ok := chains.list[cid]; ok {
		c.cb = block
		return nil
	}
	return fmt.Errorf("Chain %s doesn't exist on the peer", cid)
}

// NewPeerClientConnection Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnection() (*grpc.ClientConn, error) {
	return NewPeerClientConnectionWithAddress(viper.GetString("peer.address"))
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// NewPeerClientConnectionWithAddress Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnectionWithAddress(peerAddress string) (*grpc.ClientConn, error) {
	if comm.TLSEnabled() {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true, comm.InitTLSForPeer())
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
}

// GetChannelsInfo returns an array with information about all channels for
// this peer
func GetChannelsInfo() []*pb.ChannelInfo {
	// array to store metadata for all channels
	var channelInfoArray []*pb.ChannelInfo

	chains.RLock()
	defer chains.RUnlock()
	for key := range chains.list {
		channelInfo := &pb.ChannelInfo{ChannelId: key}

		// add this specific chaincode's metadata to the array of all chaincodes
		channelInfoArray = append(channelInfoArray, channelInfo)
	}

	return channelInfoArray
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

// CreatePeerServer creates an instance of comm.GRPCServer
// This server is used for peer communications
func CreatePeerServer(listenAddress string,
	secureConfig comm.SecureServerConfig) (comm.GRPCServer, error) {

	var err error
	peerServer, err = comm.NewGRPCServer(listenAddress, secureConfig)
	if err != nil {
		peerLogger.Errorf("Failed to create peer server (%s)", err)
		return nil, err
	}
	return peerServer, nil
}

// GetPeerServer returns the peer server instance
func GetPeerServer() comm.GRPCServer {
	return peerServer
}
