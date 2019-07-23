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

package msp

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

func (mp *MSPPrincipal) VariablyOpaqueFields() []string {
	return []string{"principal"}
}

func (mp *MSPPrincipal) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != mp.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch mp.PrincipalClassification {
	case MSPPrincipal_ROLE:
		return &MSPRole{}, nil
	case MSPPrincipal_ORGANIZATION_UNIT:
		return &OrganizationUnit{}, nil
	case MSPPrincipal_IDENTITY:
		return &SerializedIdentity{}, nil
	default:
		return nil, fmt.Errorf("unable to decode MSP type: %v", mp.PrincipalClassification)
	}
}
