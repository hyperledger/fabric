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
	mspconfig "github.com/hyperledger/fabric/common/config/msp"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

// Application org config keys
const (
	// AnchorPeersKey is the key name for the AnchorPeers ConfigValue
	AnchorPeersKey = "AnchorPeers"
)

type ApplicationOrgProtos struct {
	AnchorPeers *pb.AnchorPeers
}

type ApplicationOrgConfig struct {
	*OrganizationConfig
	protos *ApplicationOrgProtos

	applicationOrgGroup *ApplicationOrgGroup
}

// ApplicationOrgGroup defines the configuration for an application org
type ApplicationOrgGroup struct {
	*Proposer
	*OrganizationGroup
	*ApplicationOrgConfig
}

// NewApplicationOrgGroup creates a new ApplicationOrgGroup
func NewApplicationOrgGroup(id string, mspConfig *mspconfig.MSPConfigHandler) *ApplicationOrgGroup {
	aog := &ApplicationOrgGroup{
		OrganizationGroup: NewOrganizationGroup(id, mspConfig),
	}
	aog.Proposer = NewProposer(aog)
	return aog
}

// AnchorPeers returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (aog *ApplicationOrgConfig) AnchorPeers() []*pb.AnchorPeer {
	return aog.protos.AnchorPeers.AnchorPeers
}

func (aog *ApplicationOrgGroup) Allocate() Values {
	return NewApplicationOrgConfig(aog)
}

func (aoc *ApplicationOrgConfig) Commit() {
	aoc.applicationOrgGroup.ApplicationOrgConfig = aoc
	aoc.OrganizationConfig.Commit()
}

func NewApplicationOrgConfig(aog *ApplicationOrgGroup) *ApplicationOrgConfig {
	aoc := &ApplicationOrgConfig{
		protos:             &ApplicationOrgProtos{},
		OrganizationConfig: NewOrganizationConfig(aog.OrganizationGroup),

		applicationOrgGroup: aog,
	}
	var err error
	aoc.standardValues, err = NewStandardValues(aoc.protos, aoc.OrganizationConfig.protos)
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}

	return aoc
}

func (aoc *ApplicationOrgConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Anchor peers for org %s are %v", aoc.applicationOrgGroup.name, aoc.protos.AnchorPeers)
	}
	return aoc.OrganizationConfig.Validate(tx, groups)
}
