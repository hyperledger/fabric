//
// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package peer

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// Operations exposes an interface to the package level functions that operated
// on singletons in the package. This is a step towards moving from package
// level data for the peer to instance level data.
type Operations interface {
	CreateChainFromBlock(cb *common.Block, sccp sysccprovider.SystemChaincodeProvider, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources) error
	GetChannelConfig(cid string) channelconfig.Resources
	GetApplicationConfig(cid string) (channelconfig.Application, bool)
	GetChannelsInfo() []*pb.ChannelInfo
	GetStableChannelConfig(cid string) channelconfig.Resources
	GetCurrConfigBlock(cid string) *common.Block
	GetLedger(cid string) ledger.PeerLedger
	GetMSPIDs(cid string) []string
	GetPolicyManager(cid string) policies.Manager
	InitChain(cid string)
	Initialize(
		init func(string),
		sccp sysccprovider.SystemChaincodeProvider,
		pm plugin.Mapper,
		pr *platforms.Registry,
		deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
		membershipProvider ledger.MembershipInfoProvider,
		metricsProvider metrics.Provider,
		lr plugindispatcher.LifecycleResources,
		nr plugindispatcher.CollectionAndLifecycleResources,
		ledgerConfig *ledger.Config,
		nWorkers int,
	)
}

type Peer struct{}

// Default provides in implementation of the Peer interface that provides
// access to the package level state.
var Default Operations = &Peer{}

func CreateChainFromBlock(
	cb *common.Block,
	sccp sysccprovider.SystemChaincodeProvider,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	legacyLifecycleValidation plugindispatcher.LifecycleResources,
	newLifecycleValidation plugindispatcher.CollectionAndLifecycleResources,
) error {
	return Default.CreateChainFromBlock(cb, sccp, deployedCCInfoProvider, legacyLifecycleValidation, newLifecycleValidation)
}

func (p *Peer) CreateChainFromBlock(
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

	return createChain(cid, l, cb, sccp, pluginMapper, deployedCCInfoProvider, legacyLifecycleValidation, newLifecycleValidation)
}

// GetChannelConfig returns the channel configuration of the chain with channel ID. Note that this
// call returns nil if chain cid has not been created.
func GetChannelConfig(cid string) channelconfig.Resources { return Default.GetChannelConfig(cid) }
func (p *Peer) GetChannelConfig(cid string) channelconfig.Resources {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs
	}
	return nil
}

// GetChannelsInfo returns an array with information about all channels for
// this peer.
func GetChannelsInfo() []*pb.ChannelInfo { return Default.GetChannelsInfo() }
func (p *Peer) GetChannelsInfo() []*pb.ChannelInfo {
	chains.RLock()
	defer chains.RUnlock()

	var channelInfos []*pb.ChannelInfo
	for key := range chains.list {
		ci := &pb.ChannelInfo{ChannelId: key}
		channelInfos = append(channelInfos, ci)
	}
	return channelInfos
}

// GetStableChannelConfig returns the stable channel configuration of the chain with channel ID.
// Note that this call returns nil if chain cid has not been created.
func GetStableChannelConfig(cid string) channelconfig.Resources {
	return Default.GetStableChannelConfig(cid)
}
func (p *Peer) GetStableChannelConfig(cid string) channelconfig.Resources {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.bundleSource.StableBundle()
	}
	return nil
}

// GetCurrConfigBlock returns the cached config block of the specified chain.
// Note that this call returns nil if chain cid has not been created.
func GetCurrConfigBlock(cid string) *common.Block { return Default.GetCurrConfigBlock(cid) }
func (p *Peer) GetCurrConfigBlock(cid string) *common.Block {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cb
	}
	return nil
}

// GetLedger returns the ledger of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetLedger(cid string) ledger.PeerLedger { return Default.GetLedger(cid) }
func (p *Peer) GetLedger(cid string) ledger.PeerLedger {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.ledger
	}
	return nil
}

// GetMSPIDs returns the ID of each application MSP defined on this chain
func GetMSPIDs(cid string) []string { return Default.GetMSPIDs(cid) }
func (p *Peer) GetMSPIDs(cid string) []string {
	chains.RLock()
	defer chains.RUnlock()

	// if mock is set, use it to return MSPIDs
	// used for tests without a proper join
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

// GetPolicyManager returns the policy manager of the chain with chain ID. Note that this
// call returns nil if chain cid has not been created.
func GetPolicyManager(cid string) policies.Manager { return Default.GetPolicyManager(cid) }
func (p *Peer) GetPolicyManager(cid string) policies.Manager {
	chains.RLock()
	defer chains.RUnlock()
	if c, ok := chains.list[cid]; ok {
		return c.cs.PolicyManager()
	}
	return nil
}

// InitChain takes care to initialize chain after peer joined, for example deploys system CCs
func InitChain(cid string) { Default.InitChain(cid) }
func (p *Peer) InitChain(cid string) {
	if chainInitializer != nil {
		// Initialize chaincode, namely deploy system CC
		peerLogger.Debugf("Initializing channel %s", cid)
		chainInitializer(cid)
	}
}

func (p *Peer) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	cc := p.GetChannelConfig(cid)
	if cc == nil {
		return nil, false
	}

	return cc.ApplicationConfig()
}

func Initialize(
	init func(string),
	sccp sysccprovider.SystemChaincodeProvider,
	mapper plugin.Mapper,
	pr *platforms.Registry,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider,
	membershipProvider ledger.MembershipInfoProvider,
	metricsProvider metrics.Provider,
	lr plugindispatcher.LifecycleResources,
	nr plugindispatcher.CollectionAndLifecycleResources,
	ledgerConfig *ledger.Config,
	nWorkers int,
) {
	Default.Initialize(
		init,
		sccp,
		mapper,
		pr,
		deployedCCInfoProvider,
		membershipProvider,
		metricsProvider,
		lr,
		nr,
		ledgerConfig,
		nWorkers,
	)
}

// Initialize sets up any chains that the peer has from the persistence. This
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
) {
	validationWorkersSemaphore = semaphore.New(nWorkers)

	pluginMapper = pm
	chainInitializer = init

	var cb *common.Block
	var ledger ledger.PeerLedger
	ledgermgmt.Initialize(&ledgermgmt.Initializer{
		CustomTxProcessors:            ConfigTxProcessors,
		PlatformRegistry:              pr,
		DeployedChaincodeInfoProvider: deployedCCInfoProvider,
		MembershipInfoProvider:        membershipProvider,
		MetricsProvider:               metricsProvider,
		Config:                        ledgerConfig,
	})
	ledgerIds, err := ledgermgmt.GetLedgerIDs()
	if err != nil {
		panic(fmt.Errorf("Error in initializing ledgermgmt: %s", err))
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
		if err = createChain(cid, ledger, cb, sccp, pm, deployedCCInfoProvider, legacyLifecycleValidation, newLifecycleValidation); err != nil {
			peerLogger.Errorf("Failed to load chain %s(%s)", cid, err)
			peerLogger.Debugf("Error reloading chain %s with message %s. We continue to the next chain rather than abort.", cid, err)
			continue
		}

		InitChain(cid)
	}
}
