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

package config

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
)

type standardValues struct {
	lookup map[string]proto.Message
}

// NewStandardValues accepts a structure which must contain only protobuf message
// types.  The structure may embed other (non-pointer) structures which satisfy
// the same condition.  NewStandard values will instantiate memory for all the proto
// messages and build a lookup map from structure field name to proto message instance
// This is a useful way to easily implement the Values interface
func NewStandardValues(protosStructs ...interface{}) (*standardValues, error) {
	sv := &standardValues{
		lookup: make(map[string]proto.Message),
	}

	for _, protosStruct := range protosStructs {
		logger.Debugf("Initializing protos for %T\n", protosStruct)
		if err := sv.initializeProtosStruct(reflect.ValueOf(protosStruct)); err != nil {
			return nil, err
		}
	}

	return sv, nil
}

// Deserialize looks up the backing Values proto of the given name, unmarshals the given bytes
// to populate the backing message structure, and returns a referenced to the retained deserialized
// message (or an error, either because the key did not exist, or there was an an error unmarshaling
func (sv *standardValues) Deserialize(key string, value []byte) (proto.Message, error) {
	msg, ok := sv.lookup[key]
	if !ok {
		return nil, fmt.Errorf("Unexpected key %s", key)
	}

	err := proto.Unmarshal(value, msg)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (sv *standardValues) initializeProtosStruct(objValue reflect.Value) error {
	objType := objValue.Type()
	if objType.Kind() != reflect.Ptr {
		return fmt.Errorf("Non pointer type")
	}
	if objType.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("Non struct type")
	}

	numFields := objValue.Elem().NumField()
	for i := 0; i < numFields; i++ {
		structField := objType.Elem().Field(i)
		logger.Debugf("Processing field: %s\n", structField.Name)
		switch structField.Type.Kind() {
		case reflect.Ptr:
			fieldPtr := objValue.Elem().Field(i)
			if !fieldPtr.CanSet() {
				return fmt.Errorf("Cannot set structure field %s (unexported?)", structField.Name)
			}
			fieldPtr.Set(reflect.New(structField.Type.Elem()))
		default:
			return fmt.Errorf("Bad type supplied: %s", structField.Type.Kind())
		}

		proto, ok := objValue.Elem().Field(i).Interface().(proto.Message)
		if !ok {
			return fmt.Errorf("Field type %T does not implement proto.Message", objValue.Elem().Field(i))
		}

		_, ok = sv.lookup[structField.Name]
		if ok {
			return fmt.Errorf("Ambiguous field name specified, multiple occurrences of %s", structField.Name)
		}

		sv.lookup[structField.Name] = proto
	}

	return nil
}
