/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/hyperledger/fabric/common/util"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type ReadWritableState interface {
	ReadableState
	PutState(key string, value []byte) error
	DelState(key string) error
}

type ReadableState interface {
	GetState(key string) (value []byte, err error)
}

type OpaqueState interface {
	GetStateHash(key string) (value []byte, err error)
}

type RangeableState interface {
	GetStateRange(prefix string) (map[string][]byte, error)
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

// SerializableChecks performs some boilerplate checks to make sure the given structure
// is serializable.  It returns the reflected version of the value and a slice of all
// field names, or an error.
func (s *Serializer) SerializableChecks(structure interface{}) (reflect.Value, []string, error) {
	value := reflect.ValueOf(structure)
	if value.Kind() != reflect.Ptr {
		return reflect.Value{}, nil, errors.Errorf("must be pointer to struct, but got non-pointer %v", value.Kind())
	}

	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, nil, errors.Errorf("must be pointers to struct, but got pointer to %v", value.Kind())
	}

	allFields := make([]string, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		fieldName := value.Type().Field(i).Name
		fieldValue := value.Field(i)
		allFields[i] = fieldName
		switch fieldValue.Kind() {
		case reflect.String:
		case reflect.Int64:
		case reflect.Uint64:
		case reflect.Slice:
			if fieldValue.Type().Elem().Kind() != reflect.Uint8 {
				return reflect.Value{}, nil, errors.Errorf("unsupported slice type %v for field %s", fieldValue.Type().Elem().Kind(), fieldName)
			}
		default:
			return reflect.Value{}, nil, errors.Errorf("unsupported structure field kind %v for serialization for field %s", fieldValue.Kind(), fieldName)
		}
	}
	return value, allFields, nil
}

// Serialize takes a pointer to a struct, and writes each of its fields as keys
// into a namespace.  It also writes the struct metadata (if it needs updating)
// and,  deletes any keys in the namespace which are not found in the struct.
// Note: If a key already exists for the field, and the value is unchanged, then
// the key is _not_ written to.
func (s *Serializer) Serialize(namespace, name string, structure interface{}, state ReadWritableState) error {
	value, allFields, err := s.SerializableChecks(structure)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("structure for namespace %s/%s is not serializable", namespace, name))
	}

	metadata, err := s.DeserializeMetadata(namespace, name, state, false)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("could not deserialize metadata for namespace %s/%s", namespace, name))
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

	for i := 0; i < value.NumField(); i++ {
		fieldName := value.Type().Field(i).Name
		fieldValue := value.Field(i)

		keyName := fmt.Sprintf("%s/fields/%s/%s", namespace, name, fieldName)

		stateData := &lb.StateData{}
		switch fieldValue.Kind() {
		case reflect.String:
			stateData.Type = &lb.StateData_String_{String_: fieldValue.String()}
		case reflect.Int64:
			stateData.Type = &lb.StateData_Int64{Int64: fieldValue.Int()}
		case reflect.Uint64:
			stateData.Type = &lb.StateData_Uint64{Uint64: fieldValue.Uint()}
		case reflect.Slice:
			stateData.Type = &lb.StateData_Bytes{Bytes: fieldValue.Bytes()}
			// Note, other field kinds and bad slice types have already been checked by SerializableChecks
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
		err = state.PutState(fmt.Sprintf("%s/metadata/%s", namespace, name), newMetadataBin)
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

// IsSerialized essentially checks if the hashes of a serialized version of a structure matches the hashes
// of the pre-image of some struct serialized into the database.
func (s *Serializer) IsSerialized(namespace, name string, structure interface{}, state OpaqueState) (bool, error) {
	value, allFields, err := s.SerializableChecks(structure)
	if err != nil {
		return false, errors.WithMessage(err, fmt.Sprintf("structure for namespace %s/%s is not serializable", namespace, name))
	}

	fqKeys := make([]string, 0, len(allFields)+1)
	fqKeys = append(fqKeys, fmt.Sprintf("%s/metadata/%s", namespace, name))
	for _, field := range allFields {
		fqKeys = append(fqKeys, fmt.Sprintf("%s/fields/%s/%s", namespace, name, field))
	}

	existingKeys := map[string][]byte{}
	for _, fqKey := range fqKeys {
		value, err := state.GetStateHash(fqKey)
		if err != nil {
			return false, errors.WithMessage(err, fmt.Sprintf("could not get value for key %s", fqKey))
		}
		existingKeys[fqKey] = value
	}

	metadata := &lb.StateMetadata{
		Datatype: value.Type().Name(),
		Fields:   allFields,
	}
	metadataBin, err := s.Marshaler.Marshal(metadata)
	if err != nil {
		return false, errors.WithMessage(err, fmt.Sprintf("could not marshal metadata for namespace %s/%s", namespace, name))
	}

	metadataKeyName := fmt.Sprintf("%s/metadata/%s", namespace, name)
	if !bytes.Equal(util.ComputeSHA256(metadataBin), existingKeys[metadataKeyName]) {
		return false, nil
	}

	for i := 0; i < value.NumField(); i++ {
		fieldName := value.Type().Field(i).Name
		fieldValue := value.Field(i)

		keyName := fmt.Sprintf("%s/fields/%s/%s", namespace, name, fieldName)

		stateData := &lb.StateData{}
		switch fieldValue.Kind() {
		case reflect.String:
			stateData.Type = &lb.StateData_String_{String_: fieldValue.String()}
		case reflect.Int64:
			stateData.Type = &lb.StateData_Int64{Int64: fieldValue.Int()}
		case reflect.Uint64:
			stateData.Type = &lb.StateData_Uint64{Uint64: fieldValue.Uint()}
		case reflect.Slice:
			stateData.Type = &lb.StateData_Bytes{Bytes: fieldValue.Bytes()}
			// Note, other field kinds and bad slice types have already been checked by SerializableChecks
		}

		marshaledFieldValue, err := s.Marshaler.Marshal(stateData)
		if err != nil {
			return false, errors.WithMessage(err, fmt.Sprintf("could not marshal value for key %s", keyName))
		}

		if existingValue, ok := existingKeys[keyName]; !ok || !bytes.Equal(existingValue, util.ComputeSHA256(marshaledFieldValue)) {
			return false, nil
		}
	}

	return true, nil
}

// Deserialize accepts a struct (of a type previously serialized) and populates it with the values from the db.
// Note: The struct names for the serialization and deserialization must match exactly.  Unencoded fields are not
// populated, and the extraneous keys are ignored.
func (s *Serializer) Deserialize(namespace, name string, structure interface{}, state ReadableState) error {
	value, _, err := s.SerializableChecks(structure)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("could not deserialize namespace %s/%s to unserializable type %T", namespace, name, structure))
	}

	metadata, err := s.DeserializeMetadata(namespace, name, state, true)
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
			oneOf, err := s.DeserializeFieldAsBytes(namespace, name, fieldName, state)
			if err != nil {
				return err
			}
			if oneOf != nil {
				fieldValue.SetBytes(oneOf)
			}
			// Note, other field kinds and bad slice types have already been checked by SerializableChecks
		}
	}

	return nil
}

func (s *Serializer) DeserializeMetadata(namespace, name string, state ReadableState, failOnMissing bool) (*lb.StateMetadata, error) {
	metadataBin, err := state.GetState(fmt.Sprintf("%s/metadata/%s", namespace, name))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not query metadata for namespace %s/%s", namespace, name))
	}
	if metadataBin == nil && failOnMissing {
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

func (s *Serializer) DeserializeAllMetadata(namespace string, state RangeableState) (map[string]*lb.StateMetadata, error) {
	prefix := fmt.Sprintf("%s/metadata/", namespace)
	kvs, err := state.GetStateRange(prefix)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not get state range for namespace %s", namespace))
	}
	result := map[string]*lb.StateMetadata{}
	for key, value := range kvs {
		name := key[len(prefix):]
		metadata := &lb.StateMetadata{}
		err = proto.Unmarshal(value, metadata)
		if err != nil {
			return nil, errors.Wrapf(err, "error unmarshaling metadata for key %s", key)
		}
		result[name] = metadata
	}
	return result, nil
}
