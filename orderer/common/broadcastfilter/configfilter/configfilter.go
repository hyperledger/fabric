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

package configfilter

import (
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type configFilter struct {
	configManager configtx.Manager
}

func New(manager configtx.Manager) broadcastfilter.Rule {
	return &configFilter{
		configManager: manager,
	}
}

// Apply applies the rule to the given Envelope, replying with the Action to take for the message
func (cf *configFilter) Apply(message *cb.Envelope) broadcastfilter.Action {
	msgData := &cb.Payload{}

	err := proto.Unmarshal(message.Payload, msgData)
	if err != nil {
		return broadcastfilter.Forward
	}

	if msgData.Header == nil || msgData.Header.ChainHeader == nil || msgData.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
		return broadcastfilter.Forward
	}

	config := &cb.ConfigurationEnvelope{}
	err = proto.Unmarshal(msgData.Data, config)
	if err != nil {
		return broadcastfilter.Reject
	}

	err = cf.configManager.Validate(config)
	if err != nil {
		return broadcastfilter.Reject
	}

	return broadcastfilter.Reconfigure
}
