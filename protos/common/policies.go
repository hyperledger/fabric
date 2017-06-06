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

package common

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

func (p *Policy) VariablyOpaqueFields() []string {
	return []string{"value"}
}

func (p *Policy) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != p.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch p.Type {
	case int32(Policy_SIGNATURE):
		return &SignaturePolicyEnvelope{}, nil
	case int32(Policy_IMPLICIT_META):
		return &ImplicitMetaPolicy{}, nil
	default:
		return nil, fmt.Errorf("unable to decode policy type: %v", p.Type)
	}
}
