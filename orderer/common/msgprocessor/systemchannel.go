/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/filter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/sizefilter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/systemchannelfilter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// SystemChannelSupport includes the resources needed for the SystemChannel processor.
type SystemChannelSupport interface {
	// NewChannelConfig creates a new template configuration manager
	NewChannelConfig(env *cb.Envelope) (configtxapi.Manager, error)
}

// SystemChannel implements the Processor interface for the system channel
type SystemChannel struct {
	*StandardChannel
	systemChannelSupport SystemChannelSupport
}

// NewSystemChannel creates a new system channel message processor
func NewSystemChannel(support StandardChannelSupport, systemChannelSupport SystemChannelSupport, filters *filter.RuleSet) *SystemChannel {
	logger.Debugf("Creating system channel msg processor for channel %s", support.ChainID())
	return &SystemChannel{
		StandardChannel:      NewStandardChannel(support, filters),
		systemChannelSupport: systemChannelSupport,
	}
}

// SystemChannelFilterSupport specifies the subset of the full channel support required to create the filter.
type SystemChannelFilterSupport interface {
	SystemChannelSupport
	configtxapi.Manager
}

// CreateSystemChannelFilters creates the set of filters for the ordering system chain
func CreateSystemChannelFilters(chainCreator systemchannelfilter.ChainCreator, ledgerResources configtxapi.Manager) *filter.RuleSet {
	ordererConfig, ok := ledgerResources.OrdererConfig()
	if !ok {
		logger.Panicf("Cannot create system channel filters without orderer config")
	}
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.New(ordererConfig),
		sigfilter.New(policies.ChannelWriters, ledgerResources.PolicyManager()),
		systemchannelfilter.New(ledgerResources, chainCreator),
	})
}

// ProcessNormalMsg handles normal messages, rejecting them if they are not bound for the system channel ID
// with ErrChannelDoesNotExist.
func (s *SystemChannel) ProcessNormalMsg(msg *cb.Envelope) (configSeq uint64, err error) {
	channelID, err := utils.ChannelID(msg)
	if err != nil {
		return 0, err
	}

	// For the StandardChannel message processing, we would not check the channel ID,
	// because the message processor is looked up by channel ID.
	// However, the system channel message processor is the catch all for messages
	// which do not correspond to an extant channel, so we must check it here.
	if channelID != s.support.ChainID() {
		return 0, ErrChannelDoesNotExist
	}

	return s.StandardChannel.ProcessNormalMsg(msg)
}

// ProcessConfigUpdateMsg handles messages of type CONFIG_UPDATE either for the system channel itself
// or, for channel creation.  In the channel creation case, the CONFIG_UPDATE is wrapped into a resulting
// ORDERER_TRANSACTION, and in the standard CONFIG_UPDATE case, a resulting CONFIG message
func (s *SystemChannel) ProcessConfigUpdateMsg(envConfigUpdate *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	channelID, err := utils.ChannelID(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	logger.Debugf("Processing config update tx with system channel message processor for channel ID %s", channelID)

	if channelID == s.support.ChainID() {
		return s.StandardChannel.ProcessConfigUpdateMsg(envConfigUpdate)
	}

	// XXX we should check that the signature on the outer envelope is at least valid for some MSP in the system channel

	logger.Debugf("Processing channel create tx for channel %s on system channel %s", channelID, s.support.ChainID())

	// If the channel ID does not match the system channel, then this must be a channel creation transaction

	ctxm, err := s.systemChannelSupport.NewChannelConfig(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	newChannelConfigEnv, err := ctxm.ProposeConfigUpdate(envConfigUpdate)
	if err != nil {
		return nil, 0, err
	}

	newChannelEnvConfig, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID, s.support.Signer(), newChannelConfigEnv, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	wrappedOrdererTransaction, err := utils.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, s.support.ChainID(), s.support.Signer(), newChannelEnvConfig, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	// XXX we should verify that this still passes the size filter

	return wrappedOrdererTransaction, s.support.Sequence(), nil
}
