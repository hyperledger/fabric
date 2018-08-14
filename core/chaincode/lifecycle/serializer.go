/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"fmt"
	"reflect"

	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/rw_state.go --fake-name ReadWritableState . ReadWritableState
type ReadWritableState interface {
	ReadableState
	PutState(key string, value []byte) error
	DelState(key string) error
}

type ReadableState interface {
	GetState(key string) (value []byte, err error)
}

type Marshaler func(proto.Message) ([]byte, error)

func (m Marshaler) Marshal(msg proto.Message) ([]byte, error) {
	if m != nil {
		return m(msg)
	}
	return proto.Marshal(msg)
}

// Serializer is used to write structures into the db and to read them back out.
// Although it's unfortunate to write a custom serializer, rather than to use something
// pre-written, like protobuf or JSON, in order to produce precise readwrite sets which
// only perform state updates for keys which are actually updated (and not simply set
// to the same value again) custom serialization is required.
type Serializer struct {
	// Marshaler, when nil uses the standard protobuf impl.
	// Can be overridden for test.
	Marshaler Marshaler
}

// Serialize takes a pointer to a struct, and writes each of its fields as keys
// into a namespace.  It also writes the struct metadata (if it needs updating)
// and,  deletes any keys in the namespace which are not found in the struct.
// Note: If a key already exists for the field, and the value is unchanged, then
// the key is _not_ written to.
func (s *Serializer) Serialize(namespace, name string, structure interface{}, state ReadWritableState) error {
	value := reflect.ValueOf(structure)
	if value.Kind() != reflect.Ptr {
		return errors.Errorf("can only serialize pointers to struct, but got non-pointer %v", value.Kind())
	}

	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return errors.Errorf("can only serialize pointers to struct, but got pointer to %v", value.Kind())
	}

	metadataKey := fmt.Sprintf("%s/metadata/%s", namespace, name)
	metadataBin, err := state.GetState(metadataKey)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("could not query metadata for namespace %s/%s", namespace, name))
	}

	metadata := &lb.StateMetadata{}
	err = proto.Unmarshal(metadataBin, metadata)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("could not decode metadata for namespace %s/%s", namespace, name))
	}

	existingKeys := map[string][]byte{}
	for _, existingField := range metadata.Fields {
		fqKey := fmt.Sprintf("%s/fields/%s/%s", namespace, name, existingField)
		value, err := state.GetState(fqKey)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not get value for key %s", fqKey))
		}
		existingKeys[fqKey] = value
	}

	allFields := make([]string, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		fieldName := value.Type().Field(i).Name
		allFields[i] = fieldName
		keyName := fmt.Sprintf("%s/fields/%s/%s", namespace, name, fieldName)

		fieldValue := value.Field(i)
		stateData := &lb.StateData{}
		switch fieldValue.Kind() {
		case reflect.String:
			stateData.Type = &lb.StateData_String_{String_: fieldValue.String()}
		case reflect.Int64:
			stateData.Type = &lb.StateData_Int64{Int64: fieldValue.Int()}
		case reflect.Uint64:
			stateData.Type = &lb.StateData_Uint64{Uint64: fieldValue.Uint()}
		case reflect.Slice:
			if fieldValue.Type().Elem().Kind() != reflect.Uint8 {
				return errors.Errorf("unsupported slice type %v for field %s", fieldValue.Type().Elem().Kind(), fieldName)
			}
			stateData.Type = &lb.StateData_Bytes{Bytes: fieldValue.Bytes()}
		default:
			return errors.Errorf("unsupported structure field kind %v for serialization for field %s", fieldValue.Kind(), fieldName)
		}

		marshaledFieldValue, err := s.Marshaler.Marshal(stateData)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not marshal value for key %s", keyName))
		}

		if existingValue, ok := existingKeys[keyName]; !ok || !bytes.Equal(existingValue, marshaledFieldValue) {
			err := state.PutState(keyName, marshaledFieldValue)
			if err != nil {
				return errors.WithMessage(err, "could not write key into state")
			}
		}
		delete(existingKeys, keyName)
	}

	typeName := value.Type().Name()
	if len(existingKeys) > 0 || typeName != metadata.Datatype || len(metadata.Fields) != value.NumField() {
		metadata.Datatype = typeName
		metadata.Fields = allFields
		newMetadataBin, err := s.Marshaler.Marshal(metadata)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not marshal metadata for namespace %s/%s", namespace, name))
		}
		err = state.PutState(metadataKey, newMetadataBin)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not store metadata for namespace %s/%s", namespace, name))
		}
	}

	for key := range existingKeys {
		err := state.DelState(key)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not delete unneeded key %s", key))
		}
	}

	return nil
}
