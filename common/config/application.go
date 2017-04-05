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
	// ApplicationGroupKey is the group name for the Application config
	ApplicationGroupKey = "Application"
)

// ApplicationGroup represents the application config group
type ApplicationGroup struct {
	*Proposer
	*ApplicationConfig
	mspConfig *msp.MSPConfigHandler
}

type ApplicationConfig struct {
	*standardValues

	applicationGroup *ApplicationGroup
	applicationOrgs  map[string]ApplicationOrg
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewApplicationGroup(mspConfig *msp.MSPConfigHandler) *ApplicationGroup {
	ag := &ApplicationGroup{
		mspConfig: mspConfig,
	}
	ag.Proposer = NewProposer(ag)

	return ag
}

func (ag *ApplicationGroup) NewGroup(name string) (ValueProposer, error) {
	return NewApplicationOrgGroup(name, ag.mspConfig), nil
}

// Allocate returns a new instance of the ApplicationConfig
func (ag *ApplicationGroup) Allocate() Values {
	return NewApplicationConfig(ag)
}

func NewApplicationConfig(ag *ApplicationGroup) *ApplicationConfig {
	sv, err := NewStandardValues(&(struct{}{}))
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}

	return &ApplicationConfig{
		applicationGroup: ag,

		// Currently there are no config values
		standardValues: sv,
	}
}

func (ac *ApplicationConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	ac.applicationOrgs = make(map[string]ApplicationOrg)
	var ok bool
	for key, value := range groups {
		ac.applicationOrgs[key], ok = value.(*ApplicationOrgGroup)
		if !ok {
			return fmt.Errorf("Application sub-group %s was not an ApplicationOrgGroup, actually %T", key, value)
		}
	}
	return nil
}

func (ac *ApplicationConfig) Commit() {
	ac.applicationGroup.ApplicationConfig = ac
}

// Organizations returns a map of org ID to ApplicationOrg
func (ac *ApplicationConfig) Organizations() map[string]ApplicationOrg {
	return ac.applicationOrgs
}
