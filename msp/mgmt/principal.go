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
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

const (
	// Admins is the label for the local MSP admins
	Admins = "Admins"

	// Members is the label for the local MSP members
	Members = "Members"
)

type MSPPrincipalGetter interface {
	// Get returns an MSP principal for the given role
	Get(role string) (*msp.MSPPrincipal, error)
}

func NewLocalMSPPrincipalGetter() MSPPrincipalGetter {
	return &localMSPPrincipalGetter{}
}

type localMSPPrincipalGetter struct{}

func (m *localMSPPrincipalGetter) Get(role string) (*msp.MSPPrincipal, error) {
	mspid, err := GetLocalMSP().GetIdentifier()
	if err != nil {
		return nil, errors.WithMessage(err, "could not extract local msp identifier")
	}

	switch role {
	case Admins:
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: mspid})
		if err != nil {
			return nil, errors.Wrap(err, "marshalling failed")
		}

		return &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes}, nil
	case Members:
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: mspid})
		if err != nil {
			return nil, errors.Wrap(err, "marshalling failed")
		}

		return &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes}, nil
	default:
		return nil, errors.Errorf("MSP Principal role [%s] not recognized", role)
	}
}
