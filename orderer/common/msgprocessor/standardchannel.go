/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/configtxfilter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/filter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/sizefilter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// StandardChannelSupport includes the resources needed for the StandardChannel processor.
type StandardChannelSupport interface {
	// Sequence should return the current configSeq.
	Sequence() uint64

	// ChainID returns the ChannelID.
	ChainID() string

	// Signer returns the signer for this orderer.
	Signer() crypto.LocalSigner

	// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
	// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message.
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)
}

// StandardChannel implements the Processor interface for standard extant channels.
type StandardChannel struct {
	support StandardChannelSupport
	filters *filter.RuleSet
}

// NewStandardChannel creates a new message processor for a standard channel.
func NewStandardChannel(support StandardChannelSupport, filters *filter.RuleSet) *StandardChannel {
	return &StandardChannel{
		filters: filters,
		support: support,
	}
}

// CreateStandardFilters creates the set of filters for a normal (non-system) chain
func CreateStandardFilters(filterSupport configtxapi.Manager) *filter.RuleSet {
	ordererConfig, ok := filterSupport.OrdererConfig()
	if !ok {
		logger.Panicf("Missing orderer config")
	}
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ordererConfig),
		sigfilter.New(policies.ChannelWriters, filterSupport.PolicyManager()),
		configtxfilter.NewFilter(filterSupport),
		filter.AcceptRule,
	})
}

// ClassifyMsg inspects the message to determine which type of processing is necessary
func (s *StandardChannel) ClassifyMsg(chdr *cb.ChannelHeader) (Classification, error) {
	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG_UPDATE):
		return ConfigUpdateMsg, nil
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		// XXX eventually, this should return an error, but for now to allow the old message flow, return ConfigUpdateMsg
		return ConfigUpdateMsg, nil
		// return 0, fmt.Errorf("Transactions of type ORDERER_TRANSACTION cannot be Broadcast")
	case int32(cb.HeaderType_CONFIG):
		// XXX eventually, this should return an error, but for now to allow the old message flow, return ConfigUpdateMsg
		return ConfigUpdateMsg, nil
		// return 0, fmt.Errorf("Transactions of type CONFIG cannot be Broadcast")
	default:
		return NormalMsg, nil
	}
}

// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
// configuration sequence number and nil on success, or an error if the message is not valid.
func (s *StandardChannel) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	configSeq = s.support.Sequence()
	err = s.filters.Apply(env)
	return
}

// ProcessConfigUpdateMsg will attempt to apply the config impetus msg to the current configuration, and if successful
// return the resulting config message and the configSeq the config was computed from.  If the config impetus message
// is invalid, an error is returned.
func (s *StandardChannel) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	logger.Debugf("Processing config update message for channel %s", s.support.ChainID())

	// Call Sequence first.  If seq advances between proposal and acceptance, this is okay, and will cause reprocessing
	// however, if Sequence is called last, then a success could be falsely attributed to a newer configSeq.
	seq := s.support.Sequence()
	err = s.filters.Apply(env)
	if err != nil {
		return nil, 0, err
	}

	configEnvelope, err := s.support.ProposeConfigUpdate(env)
	if err != nil {
		return nil, 0, err
	}

	config, err = utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, s.support.ChainID(), s.support.Signer(), configEnvelope, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	// XXX We should probably run at least the size filter against this new tx one more time.

	return config, seq, nil
}
