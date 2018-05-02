/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
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
