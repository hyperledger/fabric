/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"
	"reflect"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

// DeserializeGroup deserializes the value for all values in a config group
func DeserializeProtoValuesFromGroup(group *cb.ConfigGroup, protosStructs ...interface{}) error {
	sv, err := NewStandardValues(protosStructs...)
	if err != nil {
		logger.Panicf("This is a compile time bug only, the proto structures are somehow invalid: %s", err)
	}

	for key, value := range group.Values {
		if _, err := sv.Deserialize(key, value.Value); err != nil {
			return err
		}
	}
	return nil
}

type StandardValues struct {
	lookup map[string]proto.Message
}

// NewStandardValues accepts a structure which must contain only protobuf message
// types.  The structure may embed other (non-pointer) structures which satisfy
// the same condition.  NewStandard values will instantiate memory for all the proto
// messages and build a lookup map from structure field name to proto message instance
// This is a useful way to easily implement the Values interface
func NewStandardValues(protosStructs ...interface{}) (*StandardValues, error) {
	sv := &StandardValues{
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
func (sv *StandardValues) Deserialize(key string, value []byte) (proto.Message, error) {
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

func (sv *StandardValues) initializeProtosStruct(objValue reflect.Value) error {
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
