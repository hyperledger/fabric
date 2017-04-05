/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

// configupdate is an implementation of the broadcast.Proccessor interface
// It facilitates the preprocessing of CONFIG_UPDATE transactions which can
// generate either new CONFIG transactions or new channel creation
// ORDERER_TRANSACTION messages.
package configupdate

import (
	"fmt"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/multichain")

const (
	// These should eventually be derived from the channel support once enabled
	msgVersion = int32(0)
	epoch      = 0
)

// SupportManager provides a way for the Handler to look up the Support for a chain
type SupportManager interface {
	// GetChain gets the chain support for a given ChannelId
	GetChain(chainID string) (Support, bool)
}

// Support enumerates a subset of the full channel support function which is required for this package
type Support interface {
	// ProposeConfigUpdate applies a CONFIG_UPDATE to an existing config to produce a *cb.ConfigEnvelope
	ProposeConfigUpdate(env *cb.Envelope) (*cb.ConfigEnvelope, error)
}

type Processor struct {
	signer          crypto.LocalSigner
	manager         SupportManager
	systemChannelID string
}

func New(systemChannelID string, supportManager SupportManager, signer crypto.LocalSigner) *Processor {
	return &Processor{
		systemChannelID: systemChannelID,
		manager:         supportManager,
		signer:          signer,
	}
}

func channelID(env *cb.Envelope) (string, error) {
	envPayload, err := utils.UnmarshalPayload(env.Payload)
	if err != nil {
		return "", fmt.Errorf("Failing to process config update because of payload unmarshaling error: %s", err)
	}

	if envPayload.Header == nil /* || envPayload.Header.ChannelHeader == nil */ {
		return "", fmt.Errorf("Failing to process config update because no channel ID was set")
	}

	chdr, err := utils.UnmarshalChannelHeader(envPayload.Header.ChannelHeader)
	if err != nil {
		return "", fmt.Errorf("Failing to process config update because of channel header unmarshaling error: %s", err)
	}

	if chdr.ChannelId == "" {
		return "", fmt.Errorf("Failing to process config update because no channel ID was set")
	}

	return chdr.ChannelId, nil
}

// Process takes in an envelope of type CONFIG_UPDATE and proceses it
// to transform it either into to a new channel creation request, or
// into a channel CONFIG transaction (or errors on failure)
func (p *Processor) Process(envConfigUpdate *cb.Envelope) (*cb.Envelope, error) {
	channelID, err := channelID(envConfigUpdate)
	if err != nil {
		return nil, err
	}

	support, ok := p.manager.GetChain(channelID)
	if ok {
		logger.Debugf("Processing channel reconfiguration request for channel %s", channelID)
		return p.existingChannelConfig(envConfigUpdate, channelID, support)
	}

	logger.Debugf("Processing channel creation request for channel %s", channelID)
	return p.newChannelConfig(channelID, envConfigUpdate)
}

func (p *Processor) existingChannelConfig(envConfigUpdate *cb.Envelope, channelID string, support Support) (*cb.Envelope, error) {
	configEnvelope, err := support.ProposeConfigUpdate(envConfigUpdate)
	if err != nil {
		return nil, err
	}

	return utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID, p.signer, configEnvelope, msgVersion, epoch)
}

func createInitialConfig(envConfigUpdate *cb.Envelope) (*cb.ConfigEnvelope, error) {
	// TODO, for now, this assumes the update contains the entire initial config
	// in the future, a subset of config needs to be allowed

	configUpdatePayload, err := utils.UnmarshalPayload(envConfigUpdate.Payload)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of payload unmarshaling error: %s", err)
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(configUpdatePayload.Data)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update envelope unmarshaling error: %s", err)
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update unmarshaling error: %s", err)
	}

	return &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: configUpdate.WriteSet,
		},

		LastUpdate: envConfigUpdate,
	}, nil
}

func (p *Processor) newChannelConfig(channelID string, envConfigUpdate *cb.Envelope) (*cb.Envelope, error) {
	initialConfig, err := createInitialConfig(envConfigUpdate)
	if err != nil {
		return nil, err
	}

	envConfig, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID, p.signer, initialConfig, msgVersion, epoch)
	if err != nil {
		return nil, err
	}

	return utils.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, p.systemChannelID, p.signer, envConfig, msgVersion, epoch)
}
