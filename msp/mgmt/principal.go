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

package mgmt

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
)

type MSPPrincipalGetter interface {
	// Get returns an MSP principal for the given role
	Get(role string) (*common.MSPPrincipal, error)
}

func NewLocalMSPPrincipalGetter() MSPPrincipalGetter {
	return &localMSPPrincipalGetter{}
}

type localMSPPrincipalGetter struct{}

func (m *localMSPPrincipalGetter) Get(role string) (*common.MSPPrincipal, error) {
	mspid, err := GetLocalMSP().GetIdentifier()
	if err != nil {
		return nil, fmt.Errorf("Could not extract local msp identifier [%s]", err)
	}

	// TODO: put the constants in some more appropriate place
	switch role {
	case "admin":
		principalBytes, err := proto.Marshal(&common.MSPRole{Role: common.MSPRole_ADMIN, MspIdentifier: mspid})
		if err != nil {
			return nil, err
		}

		return &common.MSPPrincipal{
			PrincipalClassification: common.MSPPrincipal_ROLE,
			Principal:               principalBytes}, nil
	case "member":
		principalBytes, err := proto.Marshal(&common.MSPRole{Role: common.MSPRole_MEMBER, MspIdentifier: mspid})
		if err != nil {
			return nil, err
		}

		return &common.MSPPrincipal{
			PrincipalClassification: common.MSPPrincipal_ROLE,
			Principal:               principalBytes}, nil
	default:
		return nil, fmt.Errorf("MSP Principal role [%s] not recognized.", role)
	}
}
