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

package config

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config/msp"

	"github.com/golang/protobuf/proto"
)

// Root acts as the object which anchors the rest of the config
// Note, yes, this is a stuttering name, but, the intent is to move
// this up one level at the end of refactoring
type Root struct {
	channel          *ChannelGroup
	mspConfigHandler *msp.MSPConfigHandler
}

// NewRoot creates a new instance of the Root
func NewRoot(mspConfigHandler *msp.MSPConfigHandler) *Root {
	return &Root{
		channel:          NewChannelGroup(mspConfigHandler),
		mspConfigHandler: mspConfigHandler,
	}
}

type failDeserializer struct{}

func (fd failDeserializer) Deserialize(key string, value []byte) (proto.Message, error) {
	return nil, fmt.Errorf("Programming error, this should never be invoked")
}

// BeginValueProposals is used to start a new config proposal
func (r *Root) BeginValueProposals(tx interface{}, groups []string) (ValueDeserializer, []ValueProposer, error) {
	if len(groups) != 1 {
		return nil, nil, fmt.Errorf("Root config only supports having one base group")
	}
	if groups[0] != ChannelGroupKey {
		return nil, nil, fmt.Errorf("Root group must have channel")
	}
	r.mspConfigHandler.BeginConfig(tx)
	return failDeserializer{}, []ValueProposer{r.channel}, nil
}

// RollbackConfig is used to abandon a new config proposal
func (r *Root) RollbackProposals(tx interface{}) {
	r.mspConfigHandler.RollbackProposals(tx)
}

// PreCommit is used to verify total configuration before commit
func (r *Root) PreCommit(tx interface{}) error {
	return r.mspConfigHandler.PreCommit(tx)
}

// CommitConfig is used to commit a new config proposal
func (r *Root) CommitProposals(tx interface{}) {
	r.mspConfigHandler.CommitProposals(tx)
}

// Channel returns the associated Channel level config
func (r *Root) Channel() *ChannelGroup {
	return r.channel
}

// Orderer returns the associated Orderer level config
func (r *Root) Orderer() *OrdererGroup {
	return r.channel.OrdererConfig()
}

// Application returns the associated Application level config
func (r *Root) Application() *ApplicationGroup {
	return r.channel.ApplicationConfig()
}

func (r *Root) Consortiums() *ConsortiumsGroup {
	return r.channel.ConsortiumsConfig()
}
