/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/resourcesconfig"
)

var supportFactory SupportFactory

// SupportFactory is a factory of Support interfaces
type SupportFactory interface {
	// NewSupport returns a Support interface
	NewSupport() Support
}

// Support gives access to peer resources and avoids calls to static methods
type Support interface {
	// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
	// and whether the Application config exists
	GetApplicationConfig(cid string) (channelconfig.Application, bool)

	// ChaincodeByName returns the definition (and whether they exist)
	// for a chaincode in a specific channel
	ChaincodeByName(chainname, ccname string) (resourcesconfig.ChaincodeDefinition, bool)
}

type supportImpl struct {
	operations Operations
}

func (s *supportImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	cc := s.operations.GetChannelConfig(cid)
	if cc == nil {
		return nil, false
	}

	return cc.ApplicationConfig()
}

func (s *supportImpl) ChaincodeByName(chainname, ccname string) (resourcesconfig.ChaincodeDefinition, bool) {
	rc := s.operations.GetResourcesConfig(chainname)
	if rc == nil {
		return nil, false
	}

	return rc.ChaincodeRegistry().ChaincodeByName(ccname)
}
