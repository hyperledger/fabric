/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

// ChainCreator defines the methods necessary to simulate channel creation.
type ChainCreator interface {
	// NewChannelConfig returns a template config for a new channel.
	NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error)

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
		return fmt.Errorf("bad payload: %s", err)
	}

	if msgData.Header == nil {
		return fmt.Errorf("missing payload header")
	}

	chdr, err := utils.UnmarshalChannelHeader(msgData.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("bad channel header: %s", err)
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
			return fmt.Errorf("channel creation would exceed maximimum number of channels: %d", maxChannels)
		}
	}

	configTx := &cb.Envelope{}
	err = proto.Unmarshal(msgData.Data, configTx)
	if err != nil {
		return fmt.Errorf("payload data error unmarshaling to envelope: %s", err)
	}

	return scf.authorizeAndInspect(configTx)
}

func (scf *SystemChainFilter) authorize(configEnvelope *cb.ConfigEnvelope) (*cb.ConfigEnvelope, error) {
	if configEnvelope.LastUpdate == nil {
		return nil, fmt.Errorf("updated config does not include a config update")
	}

	configManager, err := scf.cc.NewChannelConfig(configEnvelope.LastUpdate)
	if err != nil {
		return nil, fmt.Errorf("error constructing new channel config from update: %s", err)
	}

	newChannelConfigEnv, err := configManager.ProposeConfigUpdate(configEnvelope.LastUpdate)
	if err != nil {
		return nil, fmt.Errorf("error proposing channel update to new channel config: %s", err)
	}

	err = configManager.Validate(newChannelConfigEnv)
	if err != nil {
		return nil, fmt.Errorf("error applying channel update to new channel config: %s", err)
	}

	return newChannelConfigEnv, nil
}

func (scf *SystemChainFilter) authorizeAndInspect(configTx *cb.Envelope) error {
	payload := &cb.Payload{}
	err := proto.Unmarshal(configTx.Payload, payload)
	if err != nil {
		return fmt.Errorf("error unmarshaling wrapped configtx envelope payload: %s", err)
	}

	if payload.Header == nil {
		return fmt.Errorf("wrapped configtx envelope missing header")
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("error unmarshaling wrapped configtx envelope channel header: %s", err)
	}

	if chdr.Type != int32(cb.HeaderType_CONFIG) {
		return fmt.Errorf("wrapped configtx envelope not a config transaction")
	}

	configEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		return fmt.Errorf("error unmarshalling wrapped configtx config envelope from payload: %s", err)
	}

	// Make sure that the config was signed by the appropriate authorized entities
	proposedEnv, err := scf.authorize(configEnvelope)
	if err != nil {
		return err
	}

	// reflect.DeepEqual will not work here, because it considers nil and empty maps as unequal
	if !proto.Equal(proposedEnv.Config, configEnvelope.Config) {
		return fmt.Errorf("config proposed by the channel creation request did not match the config received with the channel creation request")
	}

	// Make sure the config can be parsed into a bundle
	_, err = channelconfig.NewBundle(chdr.ChannelId, configEnvelope.Config)
	if err != nil {
		return fmt.Errorf("failed to create config bundle: %s", err)
	}

	return nil
}
