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
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Operations exposes an interface to the package level functions that operated
// on singletons in the package. This is a step towards moving from package
// level data for the peer to instance level data.
type Operations interface {
	CreateChainFromBlock(cb *common.Block, sccp sysccprovider.SystemChaincodeProvider, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources) error
	GetChannelConfig(cid string) channelconfig.Resources
	GetChannelsInfo() []*pb.ChannelInfo
	GetStableChannelConfig(cid string) channelconfig.Resources
	GetCurrConfigBlock(cid string) *common.Block
	GetLedger(cid string) ledger.PeerLedger
	GetMSPIDs(cid string) []string
	GetPolicyManager(cid string) policies.Manager
	InitChain(cid string)
	Initialize(init func(string), sccp sysccprovider.SystemChaincodeProvider, pm plugin.Mapper, pr *platforms.Registry, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, membershipProvider ledger.MembershipInfoProvider, metricsProvider metrics.Provider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources)
}

type peerImpl struct {
	createChainFromBlock   func(cb *common.Block, sccp sysccprovider.SystemChaincodeProvider, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources) error
	getChannelConfig       func(cid string) channelconfig.Resources
	getChannelsInfo        func() []*pb.ChannelInfo
	getStableChannelConfig func(cid string) channelconfig.Resources
	getCurrConfigBlock     func(cid string) *common.Block
	getLedger              func(cid string) ledger.PeerLedger
	getMSPIDs              func(cid string) []string
	getPolicyManager       func(cid string) policies.Manager
	initChain              func(cid string)
	initialize             func(init func(string), sccp sysccprovider.SystemChaincodeProvider, mapper plugin.Mapper, pr *platforms.Registry, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, membershipProvider ledger.MembershipInfoProvider, metricsProvider metrics.Provider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources)
}

// Default provides in implementation of the Peer interface that provides
// access to the package level state.
var Default Operations = &peerImpl{
	createChainFromBlock:   CreateChainFromBlock,
	getChannelConfig:       GetChannelConfig,
	getChannelsInfo:        GetChannelsInfo,
	getStableChannelConfig: GetStableChannelConfig,
	getCurrConfigBlock:     GetCurrConfigBlock,
	getLedger:              GetLedger,
	getMSPIDs:              GetMSPIDs,
	getPolicyManager:       GetPolicyManager,
	initChain:              InitChain,
	initialize:             Initialize,
}

var DefaultSupport Support = &supportImpl{operations: Default}

func (p *peerImpl) CreateChainFromBlock(cb *common.Block, sccp sysccprovider.SystemChaincodeProvider, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources) error {
	return p.createChainFromBlock(cb, sccp, deployedCCInfoProvider, lr, nr)
}
func (p *peerImpl) GetChannelConfig(cid string) channelconfig.Resources {
	return p.getChannelConfig(cid)
}
func (p *peerImpl) GetChannelsInfo() []*pb.ChannelInfo { return p.getChannelsInfo() }
func (p *peerImpl) GetStableChannelConfig(cid string) channelconfig.Resources {
	return p.getStableChannelConfig(cid)
}
func (p *peerImpl) GetCurrConfigBlock(cid string) *common.Block  { return p.getCurrConfigBlock(cid) }
func (p *peerImpl) GetLedger(cid string) ledger.PeerLedger       { return p.getLedger(cid) }
func (p *peerImpl) GetMSPIDs(cid string) []string                { return p.getMSPIDs(cid) }
func (p *peerImpl) GetPolicyManager(cid string) policies.Manager { return p.getPolicyManager(cid) }
func (p *peerImpl) InitChain(cid string)                         { p.initChain(cid) }
func (p *peerImpl) Initialize(init func(string), sccp sysccprovider.SystemChaincodeProvider, mapper plugin.Mapper, pr *platforms.Registry, deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider, membershipProvider ledger.MembershipInfoProvider, metricsProvider metrics.Provider, lr plugindispatcher.LifecycleResources, nr plugindispatcher.CollectionAndLifecycleResources) {
	p.initialize(init, sccp, mapper, pr, deployedCCInfoProvider, membershipProvider, metricsProvider, lr, nr)
}
