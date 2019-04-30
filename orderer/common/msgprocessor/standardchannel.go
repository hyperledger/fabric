/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"encoding/hex"
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/pkg/errors"
)

// StandardChannelSupport includes the resources needed for the StandardChannel processor.
type StandardChannelSupport interface {
	//GetCert getcert from index
	GetCert(hash []byte) ([]byte, error)

	//CertExists
	CertExists(hash []byte) (bool, error)

	// Sequence should return the current configSeq
	Sequence() uint64

	// ChainID returns the ChannelID
	ChainID() string

	// Signer returns the signer for this orderer
	Signer() crypto.LocalSigner

	// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
	// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)
}

// StandardChannel implements the Processor interface for standard extant channels
type StandardChannel struct {
	support StandardChannelSupport
	filters *RuleSet
}

// NewStandardChannel creates a new standard message processor
func NewStandardChannel(support StandardChannelSupport, filters *RuleSet) *StandardChannel {
	return &StandardChannel{
		filters: filters,
		support: support,
	}
}

// CreateStandardChannelFilters creates the set of filters for a normal (non-system) chain
func CreateStandardChannelFilters(filterSupport channelconfig.Resources) *RuleSet {
	ordererConfig, ok := filterSupport.OrdererConfig()
	if !ok {
		logger.Panicf("Missing orderer config")
	}
	return NewRuleSet([]Rule{
		EmptyRejectRule,
		NewExpirationRejectRule(filterSupport),
		NewSizeFilter(ordererConfig),
		NewSigFilter(policies.ChannelWriters, filterSupport),
	})
}

// ClassifyMsg inspects the message to determine which type of processing is necessary
func (s *StandardChannel) ClassifyMsg(chdr *cb.ChannelHeader) Classification {
	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG_UPDATE):
		return ConfigUpdateMsg
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		// In order to maintain backwards compatibility, we must classify these messages
		return ConfigMsg
	case int32(cb.HeaderType_CONFIG):
		// In order to maintain backwards compatibility, we must classify these messages
		return ConfigMsg
	default:
		return NormalMsg
	}
}

//PruneNormalMsg prune certs in tx
func (s *StandardChannel) PruneNormalMsg(msg *cb.Envelope) (*cb.Envelope, error) {
	pruneMsg := msg
	// get the payload from the envelope
	payload, err := utils.GetPayload(msg)
	if err != nil {
		logger.Errorf("GetPayload returns err %s", err)
		return msg, err
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, err
	}

	//we only prune normal endorse msg
	if cb.HeaderType(chdr.Type) != cb.HeaderType_ENDORSER_TRANSACTION {
		logger.Debugf("skip prune this tx with type: %d", chdr.Type)
		return msg, nil
	}

	shdr, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	if err != nil || shdr.Creator == nil {
		return msg, err
	}
	sID, err := utils.GetIdentity(shdr.Creator)
	if err != nil {
		return msg, err
	}

	if len(sID.IdBytes) == util.CERT_HASH_LEN { //already hash,no need prune
		logger.Debugf("this is a  hash of creator, no need prune")
		return msg, nil
	}
	hash := util.ComputeSHA256(sID.IdBytes)
	exists, err := s.support.CertExists(hash)
	if err != nil {
		return msg, err
	} else if !exists {
		logger.Debugf("this is a  creator, but cert does not exist")
	} else { // cert already exists
		//do replace work
		logger.Infof("replace transaction  with creator hash: %s", hex.EncodeToString(hash))
		newShdr := shdr
		sID.IdBytes = hash
		newShdr.Creator = utils.MarshalOrPanic(sID)
		payloadHeader := utils.MakePayloadHeader(chdr, newShdr)
		newPayload := &cb.Payload{Header: payloadHeader, Data: payload.Data}
		pruneMsg = &cb.Envelope{Payload: utils.MarshalOrPanic(newPayload), Signature: msg.Signature}
	}

	return pruneMsg, nil
}

// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
// configuration sequence number and nil on success, or an error if the message is not valid
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
	// however, if Sequence is called last, then a success could be falsely attributed to a newer configSeq
	seq := s.support.Sequence()
	err = s.filters.Apply(env)
	if err != nil {
		return nil, 0, err
	}

	configEnvelope, err := s.support.ProposeConfigUpdate(env)
	if err != nil {
		return nil, 0, errors.WithMessage(err, fmt.Sprintf("error applying config update to existing channel '%s'", s.support.ChainID()))
	}

	config, err = utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, s.support.ChainID(), s.support.Signer(), configEnvelope, msgVersion, epoch)
	if err != nil {
		return nil, 0, err
	}

	// We re-apply the filters here, especially for the size filter, to ensure that the transaction we
	// just constructed is not too large for our consenter.  It additionally reapplies the signature
	// check, which although not strictly necessary, is a good sanity check, in case the orderer
	// has not been configured with the right cert material.  The additional overhead of the signature
	// check is negligable, as this is the reconfig path and not the normal path.
	err = s.filters.Apply(config)
	if err != nil {
		return nil, 0, err
	}

	return config, seq, nil
}

// ProcessConfigMsg takes an envelope of type `HeaderType_CONFIG`, unpacks the `ConfigEnvelope` from it
// extracts the `ConfigUpdate` from `LastUpdate` field, and calls `ProcessConfigUpdateMsg` on it.
func (s *StandardChannel) ProcessConfigMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	logger.Debugf("Processing config message for channel %s", s.support.ChainID())

	configEnvelope := &cb.ConfigEnvelope{}
	_, err = utils.UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, configEnvelope)
	if err != nil {
		return
	}

	return s.ProcessConfigUpdateMsg(configEnvelope.LastUpdate)
}
