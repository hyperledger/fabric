/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/peer"
)

type MockSupportImpl struct {
	GetApplicationConfigRv     channelconfig.Application
	GetApplicationConfigBoolRv bool
	ChaincodeByNameRv          resourcesconfig.ChaincodeDefinition
	ChaincodeByNameBoolRv      bool
}

func (s *MockSupportImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return s.GetApplicationConfigRv, s.GetApplicationConfigBoolRv
}

func (s *MockSupportImpl) ChaincodeByName(chainname, ccname string) (resourcesconfig.ChaincodeDefinition, bool) {
	return s.ChaincodeByNameRv, s.ChaincodeByNameBoolRv
}

type MockSupportFactoryImpl struct {
	NewSupportRv *MockSupportImpl
}

func (c *MockSupportFactoryImpl) NewSupport() peer.Support {
	return c.NewSupportRv
}
