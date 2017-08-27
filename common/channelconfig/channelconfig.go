/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common/channelconfig")

// RootGroupKey is the key for namespacing the channel config, especially for
// policy evaluation.
const RootGroupKey = "Channel"

// Bundle is a collection of resources which will always have a consistent
// view of the channel configuration.  In particular, for a given bundle reference,
// the config sequence, the policy manager etc. will always return exactly the
// same value.  The Bundle structure is immutable and will always be replaced in its
// entirety, with new backing memory.
type Bundle struct {
	policyManager policies.Manager
	mspManager    msp.MSPManager
}

// PolicyManager returns the policy manager constructed for this config
func (b *Bundle) PolicyManager() policies.Manager {
	return b.policyManager
}

// MSPManager returns the MSP manager constructed for this config
func (b *Bundle) MSPManager() msp.MSPManager {
	return b.mspManager
}

// NewBundle creates a new immutable bundle of configuration
func NewBundle(config *cb.Config) (*Bundle, error) {
	mspManager := msp.NewMSPManager()
	// XXX Truly initialize the MSP manager
	err := mspManager.Setup([]msp.MSP{})
	if err != nil {
		logger.Panicf("Error creating MSP manager")
	}

	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(mspManager)
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	policyManager := policies.NewManagerImpl(RootGroupKey, policyProviderMap)
	err = InitializePolicyManager(policyManager, config.ChannelGroup)
	if err != nil {
		return nil, errors.Wrap(err, "initializing policymanager failed")
	}

	return &Bundle{
		mspManager:    mspManager,
		policyManager: policyManager,
	}, nil
}
