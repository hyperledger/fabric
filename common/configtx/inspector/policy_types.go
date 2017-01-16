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

package inspector

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type policyTypes struct{}

func (ot policyTypes) Value(configItem *cb.ConfigurationItem) Viewable {
	name := "Value"
	value := &cb.Policy{}
	if err := proto.Unmarshal(configItem.Value, value); err != nil {
		return viewableError(name, err)
	}

	values := make([]Viewable, 2)
	values[0] = viewableInt32("Type", value.Type)

	switch value.Type {
	case int32(cb.Policy_SIGNATURE):
		sPolicy := &cb.SignaturePolicyEnvelope{}
		if err := proto.Unmarshal(value.Policy, sPolicy); err != nil {
			values[1] = viewableError("Policy", err)
		}

		values[1] = viewableSignaturePolicyEnvelope("Policy", sPolicy)
	default:
		values[1] = viewableError("Policy", fmt.Errorf("Unknown policy type: %s", value.Type))
	}

	return &field{
		name:   name,
		values: values,
	}
}

func viewableSignaturePolicyEnvelope(name string, signaturePolicyEnvelope *cb.SignaturePolicyEnvelope) Viewable {
	return viewableString(name, "TODO")
}

func init() {
	typeMap[cb.ConfigurationItem_Policy] = policyTypes{}
}
