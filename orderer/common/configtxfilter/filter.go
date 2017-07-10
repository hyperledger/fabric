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
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type configFilter struct {
	configManager api.Manager
}

// NewFilter creates a new configfilter Rule based on the given Manager
func NewFilter(manager api.Manager) filter.Rule {
	return &configFilter{
		configManager: manager,
	}
}

// Apply applies the rule to the given Envelope, replying with the Action to take for the message
func (cf *configFilter) Apply(message *cb.Envelope) filter.Action {
	msgData, err := utils.UnmarshalPayload(message.Payload)
	if err != nil {
		return filter.Forward
	}

	if msgData.Header == nil /* || msgData.Header.ChannelHeader == nil */ {
		return filter.Forward
	}

	chdr, err := utils.UnmarshalChannelHeader(msgData.Header.ChannelHeader)
	if err != nil {
		return filter.Forward
	}

	if chdr.Type != int32(cb.HeaderType_CONFIG) {
		return filter.Forward
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(msgData.Data)
	if err != nil {
		return filter.Reject
	}

	err = cf.configManager.Validate(configEnvelope)
	if err != nil {
		return filter.Reject
	}

	return filter.Accept
}
