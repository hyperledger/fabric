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

package testprotos

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

func (som *StaticallyOpaqueMsg) StaticallyOpaqueFields() []string {
	return []string{"plain_opaque_field"}
}

func (som *StaticallyOpaqueMsg) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != som.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return &SimpleMsg{}, nil
}

func (som *StaticallyOpaqueMsg) StaticallyOpaqueMapFields() []string {
	return []string{"map_opaque_field"}
}

func (som *StaticallyOpaqueMsg) StaticallyOpaqueMapFieldProto(name string, key string) (proto.Message, error) {
	if name != som.StaticallyOpaqueMapFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return &SimpleMsg{}, nil
}

func (som *StaticallyOpaqueMsg) StaticallyOpaqueSliceFields() []string {
	return []string{"slice_opaque_field"}
}

func (som *StaticallyOpaqueMsg) StaticallyOpaqueSliceFieldProto(name string, index int) (proto.Message, error) {
	if name != som.StaticallyOpaqueSliceFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return &SimpleMsg{}, nil
}
