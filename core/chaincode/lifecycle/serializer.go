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

// Deserialize accepts a struct (of a type previously serialized) and populates it with the values from the db.
// Note: The struct names for the serialization and deserialization must match exactly.  Unencoded fields are not
// populated, and the extraneous keys are ignored.
func (s *Serializer) Deserialize(namespace, name string, structure interface{}, state ReadableState) error {
	value := reflect.ValueOf(structure)
	if value.Kind() != reflect.Ptr {
		return errors.Errorf("can only deserialize pointers to struct, but got non-pointer %v", value.Kind())
	}

	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return errors.Errorf("can only deserialize pointers to struct, but got pointer to %v", value.Kind())
	}

	metadataBin, err := state.GetState(fmt.Sprintf("%s/metadata/%s", namespace, name))
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("could not query metadata for namespace %s/%s", namespace, name))
	}
	if metadataBin == nil {
		return errors.Errorf("no existing serialized message found")
	}

	metadata := &lb.StateMetadata{}
	err = proto.Unmarshal(metadataBin, metadata)
	if err != nil {
		return errors.Wrapf(err, "could not unmarshal metadata for namespace %s/%s", namespace, name)
	}

	typeName := value.Type().Name()
	if typeName != metadata.Datatype {
		return errors.Errorf("type name mismatch '%s' != '%s'", typeName, metadata.Datatype)
	}

	for i := 0; i < value.NumField(); i++ {
		fieldName := value.Type().Field(i).Name
		fieldValue := value.Field(i)
		switch fieldValue.Kind() {
		case reflect.String:
			oneOf, err := s.DeserializeFieldAsString(namespace, name, fieldName, state)
			if err != nil {
				return err
			}
			fieldValue.SetString(oneOf)
		case reflect.Int64:
			oneOf, err := s.DeserializeFieldAsInt64(namespace, name, fieldName, state)
			if err != nil {
				return err
			}
			fieldValue.SetInt(oneOf)
		case reflect.Uint64:
			oneOf, err := s.DeserializeFieldAsUint64(namespace, name, fieldName, state)
			if err != nil {
				return err
			}
			fieldValue.SetUint(oneOf)
		case reflect.Slice:
			if fieldValue.Type().Elem().Kind() != reflect.Uint8 {
				return errors.Errorf("unsupported slice type %v for field %s", fieldValue.Type().Elem().Kind(), fieldName)
			}
			oneOf, err := s.DeserializeFieldAsBytes(namespace, name, fieldName, state)
			if err != nil {
				return err
			}
			fieldValue.SetBytes(oneOf)
		default:
			return errors.Errorf("unsupported structure field kind %v for deserialization for field %s", fieldValue.Kind(), fieldName)
		}
	}

	return nil
}

func (s *Serializer) DeserializeMetadata(namespace, name string, state ReadableState) (*lb.StateMetadata, error) {
	metadataBin, err := state.GetState(fmt.Sprintf("%s/metadata/%s", namespace, name))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not query metadata for namespace %s/%s", namespace, name))
	}
	if metadataBin == nil {
		return nil, errors.Errorf("no existing serialized message found")
	}

	metadata := &lb.StateMetadata{}
	err = proto.Unmarshal(metadataBin, metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal metadata for namespace %s/%s", namespace, name)
	}

	return metadata, nil
}

func (s *Serializer) DeserializeField(namespace, name, field string, state ReadableState) (*lb.StateData, error) {
	keyName := fmt.Sprintf("%s/fields/%s/%s", namespace, name, field)
	value, err := state.GetState(keyName)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not get state for key %s", keyName))
	}

	stateData := &lb.StateData{}
	err = proto.Unmarshal(value, stateData)
	if err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal state for key %s", keyName)
	}

	return stateData, nil
}

func (s *Serializer) DeserializeFieldAsString(namespace, name, field string, state ReadableState) (string, error) {
	value, err := s.DeserializeField(namespace, name, field, state)
	if err != nil {
		return "", err
	}
	if value.Type == nil {
		return "", nil
	}
	oneOf, ok := value.Type.(*lb.StateData_String_)
	if !ok {
		return "", errors.Errorf("expected key %s/fields/%s/%s to encode a value of type String, but was %T", namespace, name, field, value.Type)
	}
	return oneOf.String_, nil
}

func (s *Serializer) DeserializeFieldAsBytes(namespace, name, field string, state ReadableState) ([]byte, error) {
	value, err := s.DeserializeField(namespace, name, field, state)
	if err != nil {
		return nil, err
	}
	if value.Type == nil {
		return nil, nil
	}
	oneOf, ok := value.Type.(*lb.StateData_Bytes)
	if !ok {
		return nil, errors.Errorf("expected key %s/fields/%s/%s to encode a value of type []byte, but was %T", namespace, name, field, value.Type)
	}
	return oneOf.Bytes, nil
}

func (s *Serializer) DeserializeFieldAsInt64(namespace, name, field string, state ReadableState) (int64, error) {
	value, err := s.DeserializeField(namespace, name, field, state)
	if err != nil {
		return 0, err
	}
	if value.Type == nil {
		return 0, nil
	}
	oneOf, ok := value.Type.(*lb.StateData_Int64)
	if !ok {
		return 0, errors.Errorf("expected key %s/fields/%s/%s to encode a value of type Int64, but was %T", namespace, name, field, value.Type)
	}
	return oneOf.Int64, nil
}

func (s *Serializer) DeserializeFieldAsUint64(namespace, name, field string, state ReadableState) (uint64, error) {
	value, err := s.DeserializeField(namespace, name, field, state)
	if err != nil {
		return 0, err
	}
	if value.Type == nil {
		return 0, nil
	}
	oneOf, ok := value.Type.(*lb.StateData_Uint64)
	if !ok {
		return 0, errors.Errorf("expected key %s/fields/%s/%s to encode a value of type Uint64, but was %T", namespace, name, field, value.Type)
	}
	return oneOf.Uint64, nil
}
