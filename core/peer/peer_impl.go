//
// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Operations exposes an interface to the package level functions that operated
// on singletons in the package. This is a step towards moving from package
// level data for the peer to instance level data.
type Operations interface {
	CreateChainFromBlock(cb *common.Block) error
	GetChannelConfig(cid string) channelconfig.Resources
	GetChannelsInfo() []*pb.ChannelInfo
	GetCurrConfigBlock(cid string) *common.Block
	GetLedger(cid string) ledger.PeerLedger
	GetMSPIDs(cid string) []string
	GetPolicyManager(cid string) policies.Manager
	GetResourcesConfig(cid string) resourcesconfig.Resources
	InitChain(cid string)
	Initialize(init func(string))
}

type peerImpl struct {
	createChainFromBlock func(cb *common.Block) error
	getChannelConfig     func(cid string) channelconfig.Resources
	getChannelsInfo      func() []*pb.ChannelInfo
	getCurrConfigBlock   func(cid string) *common.Block
	getLedger            func(cid string) ledger.PeerLedger
	getMSPIDs            func(cid string) []string
	getPolicyManager     func(cid string) policies.Manager
	getResourcesConfig   func(cid string) resourcesconfig.Resources
	initChain            func(cid string)
	initialize           func(init func(string))
}

// Default provides in implementation of the Peer interface that provides
// access to the package level state.
var Default Operations = &peerImpl{
	createChainFromBlock: CreateChainFromBlock,
	getChannelConfig:     GetChannelConfig,
	getChannelsInfo:      GetChannelsInfo,
	getCurrConfigBlock:   GetCurrConfigBlock,
	getLedger:            GetLedger,
	getMSPIDs:            GetMSPIDs,
	getPolicyManager:     GetPolicyManager,
	getResourcesConfig:   GetResourcesConfig,
	initChain:            InitChain,
	initialize:           Initialize,
}

var DefaultSupport Support = &supportImpl{operations: Default}

func (p *peerImpl) CreateChainFromBlock(cb *common.Block) error { return p.createChainFromBlock(cb) }
func (p *peerImpl) GetChannelConfig(cid string) channelconfig.Resources {
	return p.getChannelConfig(cid)
}
func (p *peerImpl) GetChannelsInfo() []*pb.ChannelInfo           { return p.getChannelsInfo() }
func (p *peerImpl) GetCurrConfigBlock(cid string) *common.Block  { return p.getCurrConfigBlock(cid) }
func (p *peerImpl) GetLedger(cid string) ledger.PeerLedger       { return p.getLedger(cid) }
func (p *peerImpl) GetMSPIDs(cid string) []string                { return p.getMSPIDs(cid) }
func (p *peerImpl) GetPolicyManager(cid string) policies.Manager { return p.getPolicyManager(cid) }
func (p *peerImpl) GetResourcesConfig(cid string) resourcesconfig.Resources {
	return p.getResourcesConfig(cid)
}
func (p *peerImpl) InitChain(cid string)         { p.initChain(cid) }
func (p *peerImpl) Initialize(init func(string)) { p.initialize(init) }
