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
}

func (s *supportImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	cc := GetChannelConfig(cid)
	if cc == nil {
		return nil, false
	}

	return cc.ApplicationConfig()
}

func (s *supportImpl) ChaincodeByName(chainname, ccname string) (resourcesconfig.ChaincodeDefinition, bool) {
	// FIXME: implement me properly
	return nil, false
}

type SupportFactoryImpl struct {
}

func (c *SupportFactoryImpl) NewSupport() Support {
	return &supportImpl{}
}

// RegisterSupportFactory should be called to specify
// which factory should be used to serve GetSupport calls
func RegisterSupportFactory(ccfact SupportFactory) {
	supportFactory = ccfact
}

// GetSupport returns a new Support instance by calling the factory
func GetSupport() Support {
	if supportFactory == nil {
		panic("The factory must be set first via RegisterSupportFactory")
	}
	return supportFactory.NewSupport()
}

// init is called when this package is loaded; this way by default,
// all calls to GetSupport will be satisfied by SupportFactoryImpl,
// unless RegisterSupportFactory is called again
func init() {
	RegisterSupportFactory(&SupportFactoryImpl{})
}
