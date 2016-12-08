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

package configtx

import (
	"fmt"

	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type configFilter struct {
	configManager Manager
}

// New creates a new configfilter Rule based on the given Manager
func NewFilter(manager Manager) filter.Rule {
	return &configFilter{
		configManager: manager,
	}
}

type configCommitter struct {
	manager        Manager
	configEnvelope *cb.ConfigurationEnvelope
}

func (cc *configCommitter) Commit() {
	err := cc.manager.Apply(cc.configEnvelope)
	if err != nil {
		panic(fmt.Errorf("Could not apply configuration transaction which should have already been validated: %s", err))
	}
}

func (cc *configCommitter) Isolated() bool {
	return true
}

// Apply applies the rule to the given Envelope, replying with the Action to take for the message
func (cf *configFilter) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	msgData := &cb.Payload{}

	err := proto.Unmarshal(message.Payload, msgData)
	if err != nil {
		return filter.Forward, nil
	}

	if msgData.Header == nil || msgData.Header.ChainHeader == nil || msgData.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
		return filter.Forward, nil
	}

	config := &cb.ConfigurationEnvelope{}
	err = proto.Unmarshal(msgData.Data, config)
	if err != nil {
		return filter.Reject, nil
	}

	err = cf.configManager.Validate(config)
	if err != nil {
		return filter.Reject, nil
	}

	return filter.Accept, &configCommitter{
		manager:        cf.configManager,
		configEnvelope: config,
	}
}
