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

func typeSwitch(typeName string) (proto.Message, error) {
	switch typeName {
	case "SimpleMsg":
		return &SimpleMsg{}, nil
	case "NestedMsg":
		return &NestedMsg{}, nil
	case "StaticallyOpaqueMsg":
		return &StaticallyOpaqueMsg{}, nil
	case "VariablyOpaqueMsg":
		return &VariablyOpaqueMsg{}, nil
	default:
		return nil, fmt.Errorf("unknown message type: %s", typeName)
	}
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueFields() []string {
	return []string{"plain_opaque_field"}
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != vom.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return typeSwitch(vom.OpaqueType)
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueMapFields() []string {
	return []string{"map_opaque_field"}
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueMapFieldProto(name string, key string) (proto.Message, error) {
	if name != vom.VariablyOpaqueMapFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return typeSwitch(vom.OpaqueType)
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueSliceFields() []string {
	return []string{"slice_opaque_field"}
}

func (vom *VariablyOpaqueMsg) VariablyOpaqueSliceFieldProto(name string, index int) (proto.Message, error) {
	if name != vom.VariablyOpaqueSliceFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return typeSwitch(vom.OpaqueType)
}

func (cm *ContextlessMsg) VariablyOpaqueFields() []string {
	return []string{"opaque_field"}
}

type DynamicMessageWrapper struct {
	*ContextlessMsg
	typeName string
}

func (dmw *DynamicMessageWrapper) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != dmw.ContextlessMsg.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a statically opaque field: %s", name)
	}

	return typeSwitch(dmw.typeName)
}

func (dmw *DynamicMessageWrapper) Underlying() proto.Message {
	return dmw.ContextlessMsg
}

func wrapContextless(underlying proto.Message, typeName string) (*DynamicMessageWrapper, error) {
	cm, ok := underlying.(*ContextlessMsg)
	if !ok {
		return nil, fmt.Errorf("unknown dynamic message to wrap (%T) requires *ContextlessMsg", underlying)
	}

	return &DynamicMessageWrapper{
		ContextlessMsg: cm,
		typeName:       typeName,
	}, nil
}

func (vom *DynamicMsg) DynamicFields() []string {
	return []string{"plain_dynamic_field"}
}

func (vom *DynamicMsg) DynamicFieldProto(name string, underlying proto.Message) (proto.Message, error) {
	if name != vom.DynamicFields()[0] {
		return nil, fmt.Errorf("not a dynamic field: %s", name)
	}

	return wrapContextless(underlying, vom.DynamicType)
}

func (vom *DynamicMsg) DynamicMapFields() []string {
	return []string{"map_dynamic_field"}
}

func (vom *DynamicMsg) DynamicMapFieldProto(name string, key string, underlying proto.Message) (proto.Message, error) {
	if name != vom.DynamicMapFields()[0] {
		return nil, fmt.Errorf("not a dynamic map field: %s", name)
	}

	return wrapContextless(underlying, vom.DynamicType)
}

func (vom *DynamicMsg) DynamicSliceFields() []string {
	return []string{"slice_dynamic_field"}
}

func (vom *DynamicMsg) DynamicSliceFieldProto(name string, index int, underlying proto.Message) (proto.Message, error) {
	if name != vom.DynamicSliceFields()[0] {
		return nil, fmt.Errorf("not a dynamic slice field: %s", name)
	}

	return wrapContextless(underlying, vom.DynamicType)
}
