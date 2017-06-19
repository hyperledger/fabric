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
)

const (
	// ConsortiumsGroupKey is the group name for the consortiums config
	ConsortiumsGroupKey = "Consortiums"
)

// ConsortiumsGroup stores the set of Consortiums
type ConsortiumsGroup struct {
	*Proposer
	*ConsortiumsConfig

	mspConfig *msp.MSPConfigHandler
}

// NewConsortiumsGroup creates a new *ConsortiumsGroup
func NewConsortiumsGroup(mspConfig *msp.MSPConfigHandler) *ConsortiumsGroup {
	cg := &ConsortiumsGroup{
		mspConfig: mspConfig,
	}
	cg.Proposer = NewProposer(cg)
	return cg
}

// NewGroup returns a Consortium instance
func (cg *ConsortiumsGroup) NewGroup(name string) (ValueProposer, error) {
	return NewConsortiumGroup(cg.mspConfig), nil
}

// Allocate returns the resources for a new config proposal
func (cg *ConsortiumsGroup) Allocate() Values {
	return NewConsortiumsConfig(cg)
}

// ConsortiumsConfig holds the consoritums configuration information
type ConsortiumsConfig struct {
	*standardValues
	consortiums map[string]Consortium

	consortiumsGroup *ConsortiumsGroup
}

// NewConsortiumsConfig creates a new instance of the consoritums config
func NewConsortiumsConfig(cg *ConsortiumsGroup) *ConsortiumsConfig {
	cc := &ConsortiumsConfig{
		consortiums:      make(map[string]Consortium),
		consortiumsGroup: cg,
	}
	var err error
	cc.standardValues, err = NewStandardValues()
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}
	return cc
}

// Consortiums returns a map of the current consortiums
func (cc *ConsortiumsConfig) Consortiums() map[string]Consortium {
	return cc.consortiums
}

// Commit commits the ConsortiumsConfig
func (cc *ConsortiumsConfig) Commit() {
	cc.consortiumsGroup.ConsortiumsConfig = cc
}

// Validate builds the Consortiums map
func (cc *ConsortiumsConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	var ok bool
	for key, group := range groups {
		cc.consortiums[key], ok = group.(*ConsortiumGroup)
		if !ok {
			return fmt.Errorf("Unexpected group type: %T", group)
		}
	}
	return nil
}
