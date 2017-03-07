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

	configvaluesapi "github.com/hyperledger/fabric/common/configvalues"
	"github.com/hyperledger/fabric/common/configvalues/channel/application"
	"github.com/hyperledger/fabric/common/configvalues/channel/orderer"
	"github.com/hyperledger/fabric/common/configvalues/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Root acts as the object which anchors the rest of the config
// Note, yes, this is a stuttering name, but, the intent is to move
// this up one level at the end of refactoring
type Root struct {
	channel          *ChannelGroup
	mspConfigHandler *msp.MSPConfigHandler
}

// NewRoot creates a new instance of the configvalues Root
func NewRoot(mspConfigHandler *msp.MSPConfigHandler) *Root {
	return &Root{
		channel:          NewChannelGroup(mspConfigHandler),
		mspConfigHandler: mspConfigHandler,
	}
}

// BeginValueProposals is used to start a new config proposal
func (r *Root) BeginValueProposals(groups []string) ([]configvaluesapi.ValueProposer, error) {
	if len(groups) != 1 {
		return nil, fmt.Errorf("Root config only supports having one base group")
	}
	if groups[0] != ChannelGroupKey {
		return nil, fmt.Errorf("Root group must have channel")
	}
	r.mspConfigHandler.BeginConfig()
	return []configvaluesapi.ValueProposer{r.channel}, nil
}

// RollbackConfig is used to abandon a new config proposal
func (r *Root) RollbackProposals() {
	r.mspConfigHandler.RollbackProposals()
}

// PreCommit is used to verify total configuration before commit
func (r *Root) PreCommit() error {
	return r.mspConfigHandler.PreCommit()
}

// CommitConfig is used to commit a new config proposal
func (r *Root) CommitProposals() {
	r.mspConfigHandler.CommitProposals()
}

// ProposeValue should not be invoked on this object
func (r *Root) ProposeValue(key string, value *cb.ConfigValue) error {
	return fmt.Errorf("Programming error, this should never be invoked")
}

// Channel returns the associated Channel level config
func (r *Root) Channel() *ChannelGroup {
	return r.channel
}

// Orderer returns the associated Orderer level config
func (r *Root) Orderer() *orderer.ManagerImpl {
	return r.channel.OrdererConfig()
}

// Application returns the associated Application level config
func (r *Root) Application() *application.SharedConfigImpl {
	return r.channel.ApplicationConfig()
}
