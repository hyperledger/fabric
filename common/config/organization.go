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

	mspconfig "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/msp"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
)

// Org config keys
const (
	// MSPKey is value key for marshaled *mspconfig.MSPConfig
	MSPKey = "MSP"
)

type OrganizationProtos struct {
	MSP *mspprotos.MSPConfig
}

type OrganizationConfig struct {
	*standardValues
	protos *OrganizationProtos

	organizationGroup *OrganizationGroup

	msp   msp.MSP
	mspID string
}

// Config stores common configuration information for organizations
type OrganizationGroup struct {
	*Proposer
	*OrganizationConfig
	name             string
	mspConfigHandler *mspconfig.MSPConfigHandler
}

// NewConfig creates an instnace of the organization Config
func NewOrganizationGroup(name string, mspConfigHandler *mspconfig.MSPConfigHandler) *OrganizationGroup {
	og := &OrganizationGroup{
		name:             name,
		mspConfigHandler: mspConfigHandler,
	}
	og.Proposer = NewProposer(og)
	return og
}

// Name returns the name this org is referred to in config
func (og *OrganizationGroup) Name() string {
	return og.name
}

// MSPID returns the MSP ID associated with this org
func (og *OrganizationGroup) MSPID() string {
	return og.mspID
}

// NewGroup always errors
func (og *OrganizationGroup) NewGroup(name string) (ValueProposer, error) {
	return nil, fmt.Errorf("Organization does not support subgroups")
}

// Allocate creates the proto resources neeeded for a proposal
func (og *OrganizationGroup) Allocate() Values {
	return NewOrganizationConfig(og)
}

func NewOrganizationConfig(og *OrganizationGroup) *OrganizationConfig {
	oc := &OrganizationConfig{
		protos: &OrganizationProtos{},

		organizationGroup: og,
	}

	var err error
	oc.standardValues, err = NewStandardValues(oc.protos)
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}
	return oc
}

// Validate returns whether the configuration is valid
func (oc *OrganizationConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	return oc.validateMSP(tx)
}

func (oc *OrganizationConfig) Commit() {
	oc.organizationGroup.OrganizationConfig = oc
}

func (oc *OrganizationConfig) validateMSP(tx interface{}) error {
	var err error

	logger.Debugf("Setting up MSP for org %s", oc.organizationGroup.name)
	oc.msp, err = oc.organizationGroup.mspConfigHandler.ProposeMSP(tx, oc.protos.MSP)
	if err != nil {
		return err
	}

	oc.mspID, _ = oc.msp.GetIdentifier()

	if oc.mspID == "" {
		return fmt.Errorf("MSP for org %s has empty MSP ID", oc.organizationGroup.name)
	}

	if oc.organizationGroup.OrganizationConfig != nil && oc.organizationGroup.mspID != oc.mspID {
		return fmt.Errorf("Organization %s attempted to change its MSP ID from %s to %s", oc.organizationGroup.name, oc.organizationGroup.mspID, oc.mspID)
	}

	return nil
}
