/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
)

type DynamicPolicyManagerRegistry struct {
	m      sync.Map
	Logger *flogging.FabricLogger
}

func (dpmr *DynamicPolicyManagerRegistry) Update(bundle *channelconfig.Bundle) {
	chainID := bundle.ConfigtxValidator().ChannelID()
	dpmr.m.Store(chainID, bundle.PolicyManager())
}

func (dpmr *DynamicPolicyManagerRegistry) Registry() func(channel string) policies.Manager {
	return func(channel string) policies.Manager {
		return &dynamicPolicyManager{
			m:       &dpmr.m,
			logger:  dpmr.Logger,
			channel: channel,
		}
	}
}

type dynamicPolicyManager struct {
	channel string
	m       *sync.Map
	logger  *flogging.FabricLogger
}

func (dpm *dynamicPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	o, ok := dpm.m.Load(dpm.channel)
	if !ok {
		return nil, false
	}
	return o.(policies.Manager).GetPolicy(id)
}

func (dpm *dynamicPolicyManager) Manager(path []string) (policies.Manager, bool) {
	o, ok := dpm.m.Load(dpm.channel)
	if !ok {
		return nil, false
	}
	return o.(policies.Manager).Manager(path)
}
