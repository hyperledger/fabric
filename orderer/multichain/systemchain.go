/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multichain

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

// Define some internal interfaces for easier mocking
type chainCreator interface {
	NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error)
	newChain(configTx *cb.Envelope)
	channelsCount() int
}

type limitedSupport interface {
	SharedConfig() config.Orderer
}

type systemChainCommitter struct {
	filter   *systemChainFilter
	configTx *cb.Envelope
}

func (scc *systemChainCommitter) Isolated() bool {
	return true
}

func (scc *systemChainCommitter) Commit() {
	scc.filter.cc.newChain(scc.configTx)
}

type systemChainFilter struct {
	cc      chainCreator
	support limitedSupport
}

func newSystemChainFilter(ls limitedSupport, cc chainCreator) filter.Rule {
	return &systemChainFilter{
		support: ls,
		cc:      cc,
	}
}

func (scf *systemChainFilter) Apply(env *cb.Envelope) (filter.Action, filter.Committer) {
	msgData := &cb.Payload{}

	err := proto.Unmarshal(env.Payload, msgData)
	if err != nil {
		return filter.Forward, nil
	}

	if msgData.Header == nil {
		return filter.Forward, nil
	}

	chdr, err := utils.UnmarshalChannelHeader(msgData.Header.ChannelHeader)
	if err != nil {
		return filter.Forward, nil
	}

	if chdr.Type != int32(cb.HeaderType_ORDERER_TRANSACTION) {
		return filter.Forward, nil
	}

	maxChannels := scf.support.SharedConfig().MaxChannelsCount()
	if maxChannels > 0 {
		// We check for strictly greater than to accommodate the system channel
		if uint64(scf.cc.channelsCount()) > maxChannels {
			logger.Warningf("Rejecting channel creation because the orderer has reached the maximum number of channels, %d", maxChannels)
			return filter.Reject, nil
		}
	}

	configTx := &cb.Envelope{}
	err = proto.Unmarshal(msgData.Data, configTx)
	if err != nil {
		return filter.Reject, nil
	}

	err = scf.authorizeAndInspect(configTx)
	if err != nil {
		logger.Debugf("Rejecting channel creation because: %s", err)
		return filter.Reject, nil
	}

	return filter.Accept, &systemChainCommitter{
		filter:   scf,
		configTx: configTx,
	}
}

func (scf *systemChainFilter) authorize(configEnvelope *cb.ConfigEnvelope) (configtxapi.Manager, error) {
	if configEnvelope.LastUpdate == nil {
		return nil, fmt.Errorf("Must include a config update")
	}

	configManager, err := scf.cc.NewChannelConfig(configEnvelope.LastUpdate)
	if err != nil {
		return nil, fmt.Errorf("Error constructing new channel config from update: %s", err)
	}

	newChannelConfigEnv, err := configManager.ProposeConfigUpdate(configEnvelope.LastUpdate)
	if err != nil {
		return nil, err
	}

	err = configManager.Apply(newChannelConfigEnv)
	if err != nil {
		return nil, err
	}

	return configManager, nil
}

func (scf *systemChainFilter) inspect(proposedManager, configManager configtxapi.Manager) error {
	proposedEnv := proposedManager.ConfigEnvelope()
	actualEnv := configManager.ConfigEnvelope()
	// reflect.DeepEqual will not work here, because it considers nil and empty maps as unequal
	if !proto.Equal(proposedEnv.Config, actualEnv.Config) {
		return fmt.Errorf("config proposed by the channel creation request did not match the config received with the channel creation request")
	}
	return nil
}

func (scf *systemChainFilter) authorizeAndInspect(configTx *cb.Envelope) error {
	payload := &cb.Payload{}
	err := proto.Unmarshal(configTx.Payload, payload)
	if err != nil {
		return fmt.Errorf("Rejecting chain proposal: Error unmarshaling envelope payload: %s", err)
	}

	if payload.Header == nil {
		return fmt.Errorf("Rejecting chain proposal: Not a config transaction")
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return fmt.Errorf("Rejecting chain proposal: Error unmarshaling channel header: %s", err)
	}

	if chdr.Type != int32(cb.HeaderType_CONFIG) {
		return fmt.Errorf("Rejecting chain proposal: Not a config transaction")
	}

	configEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		return fmt.Errorf("Rejecting chain proposal: Error unmarshalling config envelope from payload: %s", err)
	}

	// Make sure that the config was signed by the appropriate authorized entities
	proposedManager, err := scf.authorize(configEnvelope)
	if err != nil {
		return err
	}

	initializer := configtx.NewInitializer()
	configManager, err := configtx.NewManagerImpl(configTx, initializer, nil)
	if err != nil {
		return fmt.Errorf("Failed to create config manager and handlers: %s", err)
	}

	// Make sure that the config does not modify any of the orderer
	return scf.inspect(proposedManager, configManager)
}
