/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// StandardChannelSupport includes the resources needed for the StandardChannel processor.
type StandardChannelSupport interface {
	// Sequence should return the current configSeq.
	Sequence() uint64

	// ChainID returns the ChannelID.
	ChainID() string

	// Filters returns the set of filters for the channel.
	Filters() *filter.RuleSet

	// Signer returns the signer for this orderer.
	Signer() crypto.LocalSigner

	// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
	// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message.
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)
}

// StandardChannel implements the Processor interface for standard extant channels.
type StandardChannel struct {
	support StandardChannelSupport
}

// NewStandardChannel creates a new message processor for a standard channel.
func NewStandardChannel(support StandardChannelSupport) *StandardChannel {
	return &StandardChannel{
		support: support,
	}
}

// ClassifyMsg inspects the message to determine which type of processing is necessary.
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
	_, err = s.support.Filters().Apply(env)
	return
}

// ProcessConfigUpdateMsg will attempt to apply the config impetus msg to the current configuration, and if successful
// return the resulting config message and the configSeq the config was computed from.  If the config impetus message
// is invalid, an error is returned.
func (s *StandardChannel) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	// Call Sequence first.  If seq advances between proposal and acceptance, this is okay, and will cause reprocessing
	// however, if Sequence is called last, then a success could be falsely attributed to a newer configSeq.
	seq := s.support.Sequence()
	_, err = s.support.Filters().Apply(env)
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
