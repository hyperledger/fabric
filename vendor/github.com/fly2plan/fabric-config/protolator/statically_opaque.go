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

package protolator

import (
	"reflect"

	"github.com/golang/protobuf/proto"
)

func opaqueFrom(opaqueType func() (proto.Message, error), value interface{}, destType reflect.Type) (reflect.Value, error) {
	tree := value.(map[string]interface{}) // Safe, already checked
	nMsg, err := opaqueType()
	if err != nil {
		return reflect.Value{}, err
	}
	if err := recursivelyPopulateMessageFromTree(tree, nMsg); err != nil {
		return reflect.Value{}, err
	}
	mMsg, err := MostlyDeterministicMarshal(nMsg)
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(mMsg), nil
}

func opaqueTo(opaqueType func() (proto.Message, error), value reflect.Value) (interface{}, error) {
	nMsg, err := opaqueType()
	if err != nil {
		return nil, err
	}
	mMsg := value.Interface().([]byte) // Safe, already checked
	if err = proto.Unmarshal(mMsg, nMsg); err != nil {
		return nil, err
	}
	return recursivelyCreateTreeFromMessage(nMsg)
}

type staticallyOpaqueFieldFactory struct{}

func (soff staticallyOpaqueFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	opaqueProto, ok := msg.(StaticallyOpaqueFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, opaqueProto.StaticallyOpaqueFields())
}

func (soff staticallyOpaqueFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	opaqueProto := msg.(StaticallyOpaqueFieldProto) // Type checked in Handles

	return &plainField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: bytesType,
			value: fieldValue,
		},
		populateFrom: func(v interface{}, dT reflect.Type) (reflect.Value, error) {
			return opaqueFrom(func() (proto.Message, error) { return opaqueProto.StaticallyOpaqueFieldProto(fieldName) }, v, dT)
		},
		populateTo: func(v reflect.Value) (interface{}, error) {
			return opaqueTo(func() (proto.Message, error) { return opaqueProto.StaticallyOpaqueFieldProto(fieldName) }, v)
		},
	}, nil
}

type staticallyOpaqueMapFieldFactory struct{}

func (soff staticallyOpaqueMapFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	opaqueProto, ok := msg.(StaticallyOpaqueMapFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, opaqueProto.StaticallyOpaqueMapFields())
}

func (soff staticallyOpaqueMapFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	opaqueProto := msg.(StaticallyOpaqueMapFieldProto) // Type checked in Handles

	return &mapField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: fieldType,
			value: fieldValue,
		},
		populateFrom: func(key string, v interface{}, dT reflect.Type) (reflect.Value, error) {
			return opaqueFrom(func() (proto.Message, error) {
				return opaqueProto.StaticallyOpaqueMapFieldProto(fieldName, key)
			}, v, dT)
		},
		populateTo: func(key string, v reflect.Value) (interface{}, error) {
			return opaqueTo(func() (proto.Message, error) {
				return opaqueProto.StaticallyOpaqueMapFieldProto(fieldName, key)
			}, v)
		},
	}, nil
}

type staticallyOpaqueSliceFieldFactory struct{}

func (soff staticallyOpaqueSliceFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	opaqueProto, ok := msg.(StaticallyOpaqueSliceFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, opaqueProto.StaticallyOpaqueSliceFields())
}

func (soff staticallyOpaqueSliceFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	opaqueProto := msg.(StaticallyOpaqueSliceFieldProto) // Type checked in Handles

	return &sliceField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: fieldType,
			value: fieldValue,
		},
		populateFrom: func(index int, v interface{}, dT reflect.Type) (reflect.Value, error) {
			return opaqueFrom(func() (proto.Message, error) {
				return opaqueProto.StaticallyOpaqueSliceFieldProto(fieldName, index)
			}, v, dT)
		},
		populateTo: func(index int, v reflect.Value) (interface{}, error) {
			return opaqueTo(func() (proto.Message, error) {
				return opaqueProto.StaticallyOpaqueSliceFieldProto(fieldName, index)
			}, v)
		},
	}, nil
}
