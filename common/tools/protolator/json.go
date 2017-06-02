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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

type protoFieldFactory interface {
	// Handles should return whether or not this particular protoFieldFactory instance
	// is responsible for the given proto's field
	Handles(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) bool

	// NewProtoField should create a backing protoField implementor
	// Note that the fieldValue may represent nil, so the fieldType is also
	// included (as reflecting the type of a nil value causes a panic)
	NewProtoField(msg proto.Message, fieldName string, fieldType reflect.Type, fieldValue reflect.Value) (protoField, error)
}

type protoField interface {
	// Name returns the proto name of the field
	Name() string

	// PopulateFrom mutates the underlying object, by taking the intermediate JSON representation
	// and converting it into the proto representation, then assigning it to the backing value
	// via reflection
	PopulateFrom(source interface{}) error

	// PopulateTo does not mutate the underlying object, but instead converts it
	// into the intermediate JSON representation (ie a struct -> map[string]interface{}
	// or a slice of structs to []map[string]interface{}
	PopulateTo() (interface{}, error)
}

var (
	protoMsgType           = reflect.TypeOf((*proto.Message)(nil)).Elem()
	mapStringInterfaceType = reflect.TypeOf(map[string]interface{}{})
	bytesType              = reflect.TypeOf([]byte{})
)

type baseField struct {
	msg   proto.Message
	name  string
	fType reflect.Type
	vType reflect.Type
	value reflect.Value
}

func (bf *baseField) Name() string {
	return bf.name
}

type plainField struct {
	baseField
	populateFrom func(source interface{}, destType reflect.Type) (reflect.Value, error)
	populateTo   func(source reflect.Value) (interface{}, error)
}

func (pf *plainField) PopulateFrom(source interface{}) error {
	if !reflect.TypeOf(source).AssignableTo(pf.fType) {
		return fmt.Errorf("expected field %s for message %T to be assignable from %v but was not.  Is %T", pf.name, pf.msg, pf.fType, source)
	}
	value, err := pf.populateFrom(source, pf.vType)
	if err != nil {
		return fmt.Errorf("error in PopulateFrom for field %s for message %T: %s", pf.name, pf.msg, err)
	}
	pf.value.Set(value)
	return nil
}

func (pf *plainField) PopulateTo() (interface{}, error) {
	if !pf.value.Type().AssignableTo(pf.vType) {
		return nil, fmt.Errorf("expected field %s for message %T to be assignable to %v but was not. Got %T.", pf.name, pf.msg, pf.fType, pf.value)
	}
	value, err := pf.populateTo(pf.value)
	if err != nil {
		return nil, fmt.Errorf("error in PopulateTo for field %s for message %T: %s", pf.name, pf.msg, err)
	}
	return value, nil
}

type mapField struct {
	baseField
	populateFrom func(key string, value interface{}, destType reflect.Type) (reflect.Value, error)
	populateTo   func(key string, value reflect.Value) (interface{}, error)
}

func (mf *mapField) PopulateFrom(source interface{}) error {
	tree, ok := source.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected map field %s for message %T to be assignable from map[string]interface{} but was not. Got %T", mf.name, mf.msg, source)
	}

	result := reflect.MakeMap(mf.vType)

	for k, v := range tree {
		if !reflect.TypeOf(v).AssignableTo(mf.fType) {
			return fmt.Errorf("expected map field %s value for %s for message %T to be assignable from %v but was not.  Is %T", mf.name, k, mf.msg, mf.fType, v)
		}
		newValue, err := mf.populateFrom(k, v, mf.vType.Elem())
		if err != nil {
			return fmt.Errorf("error in PopulateFrom for map field %s with key %s for message %T: %s", mf.name, k, mf.msg, err)
		}
		result.SetMapIndex(reflect.ValueOf(k), newValue)
	}

	mf.value.Set(result)
	return nil
}

func (mf *mapField) PopulateTo() (interface{}, error) {
	result := make(map[string]interface{})
	keys := mf.value.MapKeys()
	for _, key := range keys {
		k, ok := key.Interface().(string)
		if !ok {
			return nil, fmt.Errorf("expected map field %s for message %T to have string keys, but did not.", mf.name, mf.msg)
		}

		subValue := mf.value.MapIndex(key)

		if !subValue.Type().AssignableTo(mf.vType.Elem()) {
			return nil, fmt.Errorf("expected map field %s with key %s for message %T to be assignable to %v but was not. Got %v.", mf.name, k, mf.msg, mf.vType.Elem(), subValue.Type())
		}

		value, err := mf.populateTo(k, subValue)
		if err != nil {
			return nil, fmt.Errorf("error in PopulateTo for map field %s and key %s for message %T: %s", mf.name, k, mf.msg, err)
		}
		result[k] = value
	}

	return result, nil
}

type sliceField struct {
	baseField
	populateTo   func(i int, source reflect.Value) (interface{}, error)
	populateFrom func(i int, source interface{}, destType reflect.Type) (reflect.Value, error)
}

func (sf *sliceField) PopulateFrom(source interface{}) error {
	slice, ok := source.([]interface{})
	if !ok {
		return fmt.Errorf("expected slice field %s for message %T to be assignable from []interface{} but was not. Got %T", sf.name, sf.msg, source)
	}

	result := reflect.MakeSlice(sf.vType, len(slice), len(slice))

	for i, v := range slice {
		if !reflect.TypeOf(v).AssignableTo(sf.fType) {
			return fmt.Errorf("expected slice field %s value at index %d for message %T to be assignable from %v but was not.  Is %T", sf.name, i, sf.msg, sf.fType, v)
		}
		subValue, err := sf.populateFrom(i, v, sf.vType.Elem())
		if err != nil {
			return fmt.Errorf("error in PopulateFrom for slice field %s at index %d for message %T: %s", sf.name, i, sf.msg, err)
		}
		result.Index(i).Set(subValue)
	}

	sf.value.Set(result)
	return nil
}

func (sf *sliceField) PopulateTo() (interface{}, error) {
	result := make([]interface{}, sf.value.Len())
	for i := range result {
		subValue := sf.value.Index(i)
		if !subValue.Type().AssignableTo(sf.vType.Elem()) {
			return nil, fmt.Errorf("expected slice field %s at index %d for message %T to be assignable to %v but was not. Got %v.", sf.name, i, sf.msg, sf.vType.Elem(), subValue.Type())
		}

		value, err := sf.populateTo(i, subValue)
		if err != nil {
			return nil, fmt.Errorf("error in PopulateTo for slice field %s at index %d for message %T: %s", sf.name, i, sf.msg, err)
		}
		result[i] = value
	}

	return result, nil
}

func stringInSlice(target string, slice []string) bool {
	for _, name := range slice {
		if name == target {
			return true
		}
	}
	return false
}

// protoToJSON is a simple shortcut wrapper around the proto JSON marshaler
func protoToJSON(msg proto.Message) ([]byte, error) {
	var b bytes.Buffer
	m := jsonpb.Marshaler{
		EnumsAsInts:  false,
		EmitDefaults: false,
		Indent:       "    ",
		OrigName:     true,
	}
	err := m.Marshal(&b, msg)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func mapToProto(tree map[string]interface{}, msg proto.Message) error {
	jsonOut, err := json.Marshal(tree)
	if err != nil {
		return err
	}

	return jsonpb.UnmarshalString(string(jsonOut), msg)
}

// jsonToMap allocates a map[string]interface{}, unmarshals a JSON document into it
// and returns it, or error
func jsonToMap(marshaled []byte) (map[string]interface{}, error) {
	tree := make(map[string]interface{})
	d := json.NewDecoder(bytes.NewReader(marshaled))
	d.UseNumber()
	err := d.Decode(&tree)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling intermediate JSON: %s", err)
	}
	return tree, nil
}

// The factory implementations, listed in order of most greedy to least.
// Factories listed lower, may depend on factories listed higher being
// evaluated first.
var fieldFactories = []protoFieldFactory{
	dynamicSliceFieldFactory{},
	dynamicMapFieldFactory{},
	dynamicFieldFactory{},
	variablyOpaqueSliceFieldFactory{},
	variablyOpaqueMapFieldFactory{},
	variablyOpaqueFieldFactory{},
	staticallyOpaqueSliceFieldFactory{},
	staticallyOpaqueMapFieldFactory{},
	staticallyOpaqueFieldFactory{},
	nestedSliceFieldFactory{},
	nestedMapFieldFactory{},
	nestedFieldFactory{},
}

func protoFields(msg proto.Message, uMsg proto.Message) ([]protoField, error) {
	var result []protoField

	pmVal := reflect.ValueOf(uMsg)
	if pmVal.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("expected proto.Message %T to be pointer kind", msg)
	}

	if pmVal.IsNil() {
		return nil, nil
	}

	mVal := pmVal.Elem()
	if mVal.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected proto.Message %T ptr value to be struct, was %v", uMsg, mVal.Kind())
	}

	iResult := make([][]protoField, len(fieldFactories))

	protoProps := proto.GetProperties(mVal.Type())
	// TODO, this will skip oneof fields, this should be handled
	// correctly at some point
	for _, prop := range protoProps.Prop {
		fieldName := prop.OrigName
		fieldValue := mVal.FieldByName(prop.Name)
		fieldTypeStruct, ok := mVal.Type().FieldByName(prop.Name)
		if !ok {
			return nil, fmt.Errorf("programming error: proto does not have field advertised by proto package")
		}
		fieldType := fieldTypeStruct.Type

		for i, factory := range fieldFactories {
			if !factory.Handles(msg, fieldName, fieldType, fieldValue) {
				continue
			}

			field, err := factory.NewProtoField(msg, fieldName, fieldType, fieldValue)
			if err != nil {
				return nil, err
			}
			iResult[i] = append(iResult[i], field)
			break
		}
	}

	// Loop over the collected fields in reverse order to collect them in
	// correct dependency order as specified in fieldFactories
	for i := len(iResult) - 1; i >= 0; i-- {
		result = append(result, iResult[i]...)
	}

	return result, nil
}

func recursivelyCreateTreeFromMessage(msg proto.Message) (tree map[string]interface{}, err error) {
	defer func() {
		// Because this function is recursive, it's difficult to determine which level
		// of the proto the error originated from, this wrapper leaves breadcrumbs for debugging
		if err != nil {
			err = fmt.Errorf("%T: %s", msg, err)
		}
	}()

	uMsg := msg
	decorated, ok := msg.(DecoratedProto)
	if ok {
		uMsg = decorated.Underlying()
	}

	fields, err := protoFields(msg, uMsg)
	if err != nil {
		return nil, err
	}

	jsonBytes, err := protoToJSON(uMsg)
	if err != nil {
		return nil, err
	}

	tree, err = jsonToMap(jsonBytes)
	if err != nil {
		return nil, err
	}

	for _, field := range fields {
		if _, ok := tree[field.Name()]; !ok {
			continue
		}
		delete(tree, field.Name())
		tree[field.Name()], err = field.PopulateTo()
		if err != nil {
			return nil, err
		}
	}

	return tree, nil
}

// DeepMarshalJSON marshals msg to w as JSON, but instead of marshaling bytes fields which contain nested
// marshaled messages as base64 (like the standard proto encoding), these nested messages are remarshaled
// as the JSON representation of those messages.  This is done so that the JSON representation is as non-binary
// and human readable as possible.
func DeepMarshalJSON(w io.Writer, msg proto.Message) error {
	root, err := recursivelyCreateTreeFromMessage(msg)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "\t")
	return encoder.Encode(root)
}

func recursivelyPopulateMessageFromTree(tree map[string]interface{}, msg proto.Message) (err error) {
	defer func() {
		// Because this function is recursive, it's difficult to determine which level
		// of the proto the error orginated from, this wrapper leaves breadcrumbs for debugging
		if err != nil {
			err = fmt.Errorf("%T: %s", msg, err)
		}
	}()

	uMsg := msg
	decorated, ok := msg.(DecoratedProto)
	if ok {
		uMsg = decorated.Underlying()
	}

	fields, err := protoFields(msg, uMsg)
	if err != nil {
		return err
	}

	specialFieldsMap := make(map[string]interface{})

	for _, field := range fields {
		specialField, ok := tree[field.Name()]
		if !ok {
			continue
		}
		specialFieldsMap[field.Name()] = specialField
		delete(tree, field.Name())
	}

	if err = mapToProto(tree, uMsg); err != nil {
		return err
	}

	for _, field := range fields {
		specialField, ok := specialFieldsMap[field.Name()]
		if !ok {
			continue
		}
		if err := field.PopulateFrom(specialField); err != nil {
			return err
		}
	}

	return nil
}

// DeepUnmarshalJSON takes JSON output as generated by DeepMarshalJSON and decodes it into msg
// This includes re-marshaling the expanded nested elements to binary form
func DeepUnmarshalJSON(r io.Reader, msg proto.Message) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	root, err := jsonToMap(b)
	if err != nil {
		return err
	}

	return recursivelyPopulateMessageFromTree(root, msg)
}
