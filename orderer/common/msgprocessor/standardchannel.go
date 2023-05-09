/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

// StandardChannelSupport includes the resources needed for the StandardChannel processor.
type StandardChannelSupport interface {
	// Sequence should return the current configSeq
	Sequence() uint64

	// ChannelID returns the ChannelID
	ChannelID() string

	// Signer returns the signer for this orderer
	Signer() identity.SignerSerializer

	// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
	// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)

	OrdererConfig() (channelconfig.Orderer, bool)
}

// StandardChannel implements the Processor interface for standard extant channels
type StandardChannel struct {
	support           StandardChannelSupport
	filters           *RuleSet // Rules applicable to both normal and config messages
	maintenanceFilter Rule     // Rule applicable only to config messages
}

// NewStandardChannel creates a new standard message processor
func NewStandardChannel(support StandardChannelSupport, filters *RuleSet, bccsp bccsp.BCCSP) *StandardChannel {
	return &StandardChannel{
		filters:           filters,
		support:           support,
		maintenanceFilter: NewMaintenanceFilter(support, bccsp),
	}
}

// CreateStandardChannelFilters creates the set of filters for a normal (non-system) chain.
//
// In maintenance mode, require the signature of /Channel/Orderer/Writer. This will filter out configuration
// changes that are not related to consensus-type migration (e.g on /Channel/Application).
func CreateStandardChannelFilters(filterSupport channelconfig.Resources, config localconfig.TopLevel) *RuleSet {
	rules := []Rule{
		EmptyRejectRule,
		NewSizeFilter(filterSupport),
		NewSigFilter(policies.ChannelWriters, policies.ChannelOrdererWriters, filterSupport),
	}

	if !config.General.Authentication.NoExpirationChecks {
		expirationRule := NewExpirationRejectRule(filterSupport)
		// In case of DoS, expiration is inserted before SigFilter, so it is evaluated first
		rules = append(rules[:2], append([]Rule{expirationRule}, rules[2:]...)...)
	}

	return NewRuleSet(rules)
}

// ClassifyMsg inspects the message to determine which type of processing is necessary
func (s *StandardChannel) ClassifyMsg(chdr *cb.ChannelHeader) Classification {
	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG_UPDATE):
		return ConfigUpdateMsg
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		// Not supported in v3
		return UnsupportedMsg
	case int32(cb.HeaderType_CONFIG):
		// In order to maintain backwards compatibility, we must classify these messages
		return ConfigMsg
	default:
		return NormalMsg
	}
}

// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
// configuration sequence number and nil on success, or an error if the message is not valid
func (s *StandardChannel) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	oc, ok := s.support.OrdererConfig()
	if !ok {
		logger.Panicf("Missing orderer config")
	}
	if oc.Capabilities().ConsensusTypeMigration() {
		if oc.ConsensusState() != orderer.ConsensusType_STATE_NORMAL {
			return 0, errors.WithMessage(
				ErrMaintenanceMode, "normal transactions are rejected")
		}
	}

	configSeq = s.support.Sequence()
	err = s.filters.Apply(env)
	return
}

// ProcessConfigUpdateMsg will attempt to apply the config impetus msg to the current configuration, and if successful
// return the resulting config message and the configSeq the config was computed from.  If the config impetus message
// is invalid, an error is returned.
func (s *StandardChannel) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	logger.Debugf("Processing config update message for existing channel %s", s.support.ChannelID())

	// Call Sequence first.  If seq advances between proposal and acceptance, this is okay, and will cause reprocessing
	// however, if Sequence is called last, then a success could be falsely attributed to a newer configSeq
	seq := s.support.Sequence()
	err = s.filters.Apply(env)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "config update for existing channel did not pass initial checks")
	}

	configEnvelope, err := s.support.ProposeConfigUpdate(env)
	if err != nil {
		return nil, 0, errors.WithMessagef(err, "error applying config update to existing channel '%s'", s.support.ChannelID())
	}

	config, err = protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, s.support.ChannelID(), s.support.Signer(), configEnvelope, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	// We re-apply the filters here, especially for the size filter, to ensure that the transaction we
	// just constructed is not too large for our consenter.  It additionally reapplies the signature
	// check, which although not strictly necessary, is a good sanity check, in case the orderer
	// has not been configured with the right cert material.  The additional overhead of the signature
	// check is negligible, as this is the reconfig path and not the normal path.
	err = s.filters.Apply(config)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "config update for existing channel did not pass final checks")
	}

	err = s.maintenanceFilter.Apply(config)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "config update for existing channel did not pass maintenance checks")
	}

	return config, seq, nil
}

// ProcessConfigMsg takes an envelope of type `HeaderType_CONFIG`, unpacks the `ConfigEnvelope` from it
// extracts the `ConfigUpdate` from `LastUpdate` field, and calls `ProcessConfigUpdateMsg` on it.
func (s *StandardChannel) ProcessConfigMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	logger.Debugf("Processing config message for channel %s", s.support.ChannelID())

	configEnvelope := &cb.ConfigEnvelope{}
	_, err = protoutil.UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, configEnvelope)
	if err != nil {
		return
	}

	return s.ProcessConfigUpdateMsg(configEnvelope.LastUpdate)
}
