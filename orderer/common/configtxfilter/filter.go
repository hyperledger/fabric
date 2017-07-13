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

package configtxfilter

import (
	"fmt"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type configFilter struct {
	configManager api.Manager
}

// New creates a new configfilter Rule based on the given Manager
func NewFilter(manager api.Manager) filter.Rule {
	return &configFilter{
		configManager: manager,
	}
}

type configCommitter struct {
	manager        api.Manager
	configEnvelope *cb.ConfigEnvelope
}

func (cc *configCommitter) Commit() {
	err := cc.manager.Apply(cc.configEnvelope)
	if err != nil {
		panic(fmt.Errorf("Could not apply config transaction which should have already been validated: %s", err))
	}
}

func (cc *configCommitter) Isolated() bool {
	return true
}

// Apply applies the rule to the given Envelope, replying with the Action to take for the message
func (cf *configFilter) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	msgData, err := utils.UnmarshalPayload(message.Payload)
	if err != nil {
		return filter.Forward, nil
	}

	if msgData.Header == nil /* || msgData.Header.ChannelHeader == nil */ {
		return filter.Forward, nil
	}

	chdr, err := utils.UnmarshalChannelHeader(msgData.Header.ChannelHeader)
	if err != nil {
		return filter.Forward, nil
	}

	if chdr.Type != int32(cb.HeaderType_CONFIG) {
		return filter.Forward, nil
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(msgData.Data)
	if err != nil {
		return filter.Reject, nil
	}

	err = cf.configManager.Validate(configEnvelope)
	if err != nil {
		return filter.Reject, nil
	}

	return filter.Accept, &configCommitter{
		manager:        cf.configManager,
		configEnvelope: configEnvelope,
	}
}
