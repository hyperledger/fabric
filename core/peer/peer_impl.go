//
// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
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

type Peer struct {
	getPolicyManager func(cid string) policies.Manager
	initChain        func(cid string)
	initialize       func(
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
	)
}

// Default provides in implementation of the Peer interface that provides
// access to the package level state.
var Default Operations = &Peer{
	getPolicyManager: GetPolicyManager,
	initChain:        InitChain,
	initialize:       Initialize,
}

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

func (p *Peer) GetPolicyManager(cid string) policies.Manager { return p.getPolicyManager(cid) }
func (p *Peer) InitChain(cid string)                         { p.initChain(cid) }

func (p *Peer) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	cc := p.GetChannelConfig(cid)
	if cc == nil {
		return nil, false
	}

	return cc.ApplicationConfig()
}

func (p *Peer) Initialize(
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
	p.initialize(
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
