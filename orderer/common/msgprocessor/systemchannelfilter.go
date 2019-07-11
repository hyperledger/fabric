/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// ChainCreator defines the methods necessary to simulate channel creation.
type ChainCreator interface {
	// NewChannelConfig returns a template config for a new channel.
	NewChannelConfig(envConfigUpdate *cb.Envelope) (channelconfig.Resources, error)

	// CreateBundle parses the config into resources
	CreateBundle(channelID string, config *cb.Config) (channelconfig.Resources, error)

	// ChannelsCount returns the count of channels which currently exist.
	ChannelsCount() int
}

// LimitedSupport defines the subset of the channel resources required by the systemchannel filter.
type LimitedSupport interface {
	OrdererConfig() (channelconfig.Orderer, bool)
}

// SystemChainFilter implements the filter.Rule interface.
type SystemChainFilter struct {
	cc      ChainCreator
	support LimitedSupport
}

// NewSystemChannelFilter returns a new instance of a *SystemChainFilter.
func NewSystemChannelFilter(ls LimitedSupport, cc ChainCreator) *SystemChainFilter {
	return &SystemChainFilter{
		support: ls,
		cc:      cc,
	}
}

// Apply rejects bad messages with an error.
func (scf *SystemChainFilter) Apply(env *cb.Envelope) error {
	msgData := &cb.Payload{}

	err := proto.Unmarshal(env.Payload, msgData)
	if err != nil {
		return errors.Errorf("bad payload: %s", err)
	}

	if msgData.Header == nil {
		return errors.Errorf("missing payload header")
	}

	chdr, err := utils.UnmarshalChannelHeader(msgData.Header.ChannelHeader)
	if err != nil {
		return errors.Errorf("bad channel header: %s", err)
	}

	if chdr.Type != int32(cb.HeaderType_ORDERER_TRANSACTION) {
		return nil
	}

	ordererConfig, ok := scf.support.OrdererConfig()
	if !ok {
		logger.Panicf("System channel does not have orderer config")
	}

	maxChannels := ordererConfig.MaxChannelsCount()
	if maxChannels > 0 {
		// We check for strictly greater than to accommodate the system channel
		if uint64(scf.cc.ChannelsCount()) > maxChannels {
			return errors.Errorf("channel creation would exceed maximimum number of channels: %d", maxChannels)
		}
	}

	if ordererConfig.ConsensusState() != orderer.ConsensusType_STATE_NORMAL {
		return errors.WithMessage(ErrMaintenanceMode, "channel creation is not permitted")
	}

	configTx := &cb.Envelope{}
	err = proto.Unmarshal(msgData.Data, configTx)
	if err != nil {
		return errors.Errorf("payload data error unmarshaling to envelope: %s", err)
	}

	return scf.authorizeAndInspect(configTx)
}

func (scf *SystemChainFilter) authorizeAndInspect(configTx *cb.Envelope) error {
	payload := &cb.Payload{}
	err := proto.Unmarshal(configTx.Payload, payload)
	if err != nil {
		return errors.Errorf("error unmarshaling wrapped configtx envelope payload: %s", err)
	}

	if payload.Header == nil {
		return errors.Errorf("wrapped configtx envelope missing header")
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return errors.Errorf("error unmarshaling wrapped configtx envelope channel header: %s", err)
	}

	if chdr.Type != int32(cb.HeaderType_CONFIG) {
		return errors.Errorf("wrapped configtx envelope not a config transaction")
	}

	configEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		return errors.Errorf("error unmarshalling wrapped configtx config envelope from payload: %s", err)
	}

	if configEnvelope.LastUpdate == nil {
		return errors.Errorf("updated config does not include a config update")
	}

	res, err := scf.cc.NewChannelConfig(configEnvelope.LastUpdate)
	if err != nil {
		return errors.Errorf("error constructing new channel config from update: %s", err)
	}

	// Make sure that the config was signed by the appropriate authorized entities
	newChannelConfigEnv, err := res.ConfigtxValidator().ProposeConfigUpdate(configEnvelope.LastUpdate)
	if err != nil {
		return errors.Errorf("error proposing channel update to new channel config: %s", err)
	}

	// reflect.DeepEqual will not work here, because it considers nil and empty maps as unequal
	if !proto.Equal(newChannelConfigEnv, configEnvelope) {
		return errors.Errorf("config proposed by the channel creation request did not match the config received with the channel creation request")
	}

	bundle, err := scf.cc.CreateBundle(res.ConfigtxValidator().ChainID(), newChannelConfigEnv.Config)
	if err != nil {
		return errors.Wrap(err, "config does not validly parse")
	}

	if err = res.ValidateNew(bundle); err != nil {
		return errors.Wrap(err, "new bundle invalid")
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		return errors.New("config is missing orderer group")
	}

	if err = oc.Capabilities().Supported(); err != nil {
		return errors.Wrap(err, "config update is not compatible")
	}

	if err = bundle.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrap(err, "config update is not compatible")
	}

	return nil
}
