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

func dynamicFrom(dynamicMsg func(underlying proto.Message) (proto.Message, error), value interface{}, destType reflect.Type) (reflect.Value, error) {
	tree := value.(map[string]interface{}) // Safe, already checked
	uMsg := reflect.New(destType.Elem())
	nMsg, err := dynamicMsg(uMsg.Interface().(proto.Message)) // Safe, already checked
	if err != nil {
		return reflect.Value{}, err
	}
	if err := recursivelyPopulateMessageFromTree(tree, nMsg); err != nil {
		return reflect.Value{}, err
	}
	return uMsg, nil
}

func dynamicTo(dynamicMsg func(underlying proto.Message) (proto.Message, error), value reflect.Value) (interface{}, error) {
	nMsg, err := dynamicMsg(value.Interface().(proto.Message)) // Safe, already checked
	if err != nil {
		return nil, err
	}
	return recursivelyCreateTreeFromMessage(nMsg)
}

type dynamicFieldFactory struct{}

func (dff dynamicFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	dynamicProto, ok := msg.(DynamicFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, dynamicProto.DynamicFields())
}

func (dff dynamicFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	dynamicProto, _ := msg.(DynamicFieldProto) // Type checked in Handles

	return &plainField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: fieldType,
			value: fieldValue,
		},
		populateFrom: func(v interface{}, dT reflect.Type) (reflect.Value, error) {
			return dynamicFrom(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicFieldProto(fieldName, underlying)
			}, v, dT)
		},
		populateTo: func(v reflect.Value) (interface{}, error) {
			return dynamicTo(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicFieldProto(fieldName, underlying)
			}, v)
		},
	}, nil
}

type dynamicMapFieldFactory struct{}

func (dmff dynamicMapFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	dynamicProto, ok := msg.(DynamicMapFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, dynamicProto.DynamicMapFields())
}

func (dmff dynamicMapFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	dynamicProto := msg.(DynamicMapFieldProto) // Type checked by Handles

	return &mapField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: fieldType,
			value: fieldValue,
		},
		populateFrom: func(k string, v interface{}, dT reflect.Type) (reflect.Value, error) {
			return dynamicFrom(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicMapFieldProto(fieldName, k, underlying)
			}, v, dT)
		},
		populateTo: func(k string, v reflect.Value) (interface{}, error) {
			return dynamicTo(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicMapFieldProto(fieldName, k, underlying)
			}, v)
		},
	}, nil
}

type dynamicSliceFieldFactory struct{}

func (dmff dynamicSliceFieldFactory) Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool {
	dynamicProto, ok := msg.(DynamicSliceFieldProto)
	if !ok {
		return false
	}

	return stringInSlice(fieldName, dynamicProto.DynamicSliceFields())
}

func (dmff dynamicSliceFieldFactory) NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error) {
	dynamicProto := msg.(DynamicSliceFieldProto) // Type checked by Handles

	return &sliceField{
		baseField: baseField{
			msg:   msg,
			name:  fieldName,
			fType: mapStringInterfaceType,
			vType: fieldType,
			value: fieldValue,
		},
		populateFrom: func(i int, v interface{}, dT reflect.Type) (reflect.Value, error) {
			return dynamicFrom(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicSliceFieldProto(fieldName, i, underlying)
			}, v, dT)
		},
		populateTo: func(i int, v reflect.Value) (interface{}, error) {
			return dynamicTo(func(underlying proto.Message) (proto.Message, error) {
				return dynamicProto.DynamicSliceFieldProto(fieldName, i, underlying)
			}, v)
		},
	}, nil
}
