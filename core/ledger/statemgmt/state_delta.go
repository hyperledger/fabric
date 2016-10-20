/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package statemgmt

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
)

// StateDelta holds the changes to existing state. This struct is used for holding the uncommitted changes during execution of a tx-batch
// Also, to be used for transferring the state to another peer in chunks
type StateDelta struct {
	ChaincodeStateDeltas map[string]*ChaincodeStateDelta
	// RollBackwards allows one to contol whether this delta will roll the state
	// forwards or backwards.
	RollBackwards bool
}

// NewStateDelta constructs an empty StateDelta struct
func NewStateDelta() *StateDelta {
	return &StateDelta{make(map[string]*ChaincodeStateDelta), false}
}

// Get get the state from delta if exists
func (stateDelta *StateDelta) Get(chaincodeID string, key string) *UpdatedValue {
	// TODO Cache?
	chaincodeStateDelta, ok := stateDelta.ChaincodeStateDeltas[chaincodeID]
	if ok {
		return chaincodeStateDelta.get(key)
	}
	return nil
}

// Set sets state value for a key
func (stateDelta *StateDelta) Set(chaincodeID string, key string, value, previousValue []byte) {
	chaincodeStateDelta := stateDelta.getOrCreateChaincodeStateDelta(chaincodeID)
	chaincodeStateDelta.set(key, value, previousValue)
	return
}

// Delete deletes a key from the state
func (stateDelta *StateDelta) Delete(chaincodeID string, key string, previousValue []byte) {
	chaincodeStateDelta := stateDelta.getOrCreateChaincodeStateDelta(chaincodeID)
	chaincodeStateDelta.remove(key, previousValue)
	return
}

// IsUpdatedValueSet returns true if a update value is already set for
// the given chaincode ID and key.
func (stateDelta *StateDelta) IsUpdatedValueSet(chaincodeID, key string) bool {
	chaincodeStateDelta, ok := stateDelta.ChaincodeStateDeltas[chaincodeID]
	if !ok {
		return false
	}
	if _, ok := chaincodeStateDelta.UpdatedKVs[key]; ok {
		return true
	}
	return false
}

// ApplyChanges merges another delta - if a key is present in both, the value of the existing key is overwritten
func (stateDelta *StateDelta) ApplyChanges(anotherStateDelta *StateDelta) {
	for chaincodeID, chaincodeStateDelta := range anotherStateDelta.ChaincodeStateDeltas {
		existingChaincodeStateDelta, existingChaincode := stateDelta.ChaincodeStateDeltas[chaincodeID]
		for key, valueHolder := range chaincodeStateDelta.UpdatedKVs {
			var previousValue []byte
			if existingChaincode {
				existingUpdateValue, existingUpdate := existingChaincodeStateDelta.UpdatedKVs[key]
				if existingUpdate {
					// The existing state delta already has an updated value for this key.
					previousValue = existingUpdateValue.PreviousValue
				} else {
					// Use the previous value set in the new state delta
					previousValue = valueHolder.PreviousValue
				}
			} else {
				// Use the previous value set in the new state delta
				previousValue = valueHolder.PreviousValue
			}

			if valueHolder.IsDeleted() {
				stateDelta.Delete(chaincodeID, key, previousValue)
			} else {
				stateDelta.Set(chaincodeID, key, valueHolder.Value, previousValue)
			}
		}
	}
}

// IsEmpty checks whether StateDelta contains any data
func (stateDelta *StateDelta) IsEmpty() bool {
	return len(stateDelta.ChaincodeStateDeltas) == 0
}

// GetUpdatedChaincodeIds return the chaincodeIDs that are prepsent in the delta
// If sorted is true, the method return chaincodeIDs in lexicographical sorted order
func (stateDelta *StateDelta) GetUpdatedChaincodeIds(sorted bool) []string {
	updatedChaincodeIds := make([]string, len(stateDelta.ChaincodeStateDeltas))
	i := 0
	for k := range stateDelta.ChaincodeStateDeltas {
		updatedChaincodeIds[i] = k
		i++
	}
	if sorted {
		sort.Strings(updatedChaincodeIds)
	}
	return updatedChaincodeIds
}

// GetUpdates returns changes associated with given chaincodeId
func (stateDelta *StateDelta) GetUpdates(chaincodeID string) map[string]*UpdatedValue {
	chaincodeStateDelta := stateDelta.ChaincodeStateDeltas[chaincodeID]
	if chaincodeStateDelta == nil {
		return nil
	}
	return chaincodeStateDelta.UpdatedKVs
}

func (stateDelta *StateDelta) getOrCreateChaincodeStateDelta(chaincodeID string) *ChaincodeStateDelta {
	chaincodeStateDelta, ok := stateDelta.ChaincodeStateDeltas[chaincodeID]
	if !ok {
		chaincodeStateDelta = newChaincodeStateDelta(chaincodeID)
		stateDelta.ChaincodeStateDeltas[chaincodeID] = chaincodeStateDelta
	}
	return chaincodeStateDelta
}

// ComputeCryptoHash computes crypto-hash for the data held
// returns nil if no data is present
func (stateDelta *StateDelta) ComputeCryptoHash() []byte {
	if stateDelta.IsEmpty() {
		return nil
	}
	var buffer bytes.Buffer
	sortedChaincodeIds := stateDelta.GetUpdatedChaincodeIds(true)
	for _, chaincodeID := range sortedChaincodeIds {
		buffer.WriteString(chaincodeID)
		chaincodeStateDelta := stateDelta.ChaincodeStateDeltas[chaincodeID]
		sortedKeys := chaincodeStateDelta.getSortedKeys()
		for _, key := range sortedKeys {
			buffer.WriteString(key)
			updatedValue := chaincodeStateDelta.get(key)
			if !updatedValue.IsDeleted() {
				buffer.Write(updatedValue.Value)
			}
		}
	}
	hashingContent := buffer.Bytes()
	logger.Debugf("computing hash on %#v", hashingContent)
	return util.ComputeCryptoHash(hashingContent)
}

//ChaincodeStateDelta maintains state for a chaincode
type ChaincodeStateDelta struct {
	ChaincodeID string
	UpdatedKVs  map[string]*UpdatedValue
}

func newChaincodeStateDelta(chaincodeID string) *ChaincodeStateDelta {
	return &ChaincodeStateDelta{chaincodeID, make(map[string]*UpdatedValue)}
}

func (chaincodeStateDelta *ChaincodeStateDelta) get(key string) *UpdatedValue {
	// TODO Cache?
	return chaincodeStateDelta.UpdatedKVs[key]
}

func (chaincodeStateDelta *ChaincodeStateDelta) set(key string, updatedValue, previousValue []byte) {
	updatedKV, ok := chaincodeStateDelta.UpdatedKVs[key]
	if ok {
		// Key already exists, just set the updated value
		updatedKV.Value = updatedValue
	} else {
		// New key. Create a new entry in the map
		chaincodeStateDelta.UpdatedKVs[key] = &UpdatedValue{updatedValue, previousValue}
	}
}

func (chaincodeStateDelta *ChaincodeStateDelta) remove(key string, previousValue []byte) {
	updatedKV, ok := chaincodeStateDelta.UpdatedKVs[key]
	if ok {
		// Key already exists, just set the value
		updatedKV.Value = nil
	} else {
		// New key. Create a new entry in the map
		chaincodeStateDelta.UpdatedKVs[key] = &UpdatedValue{nil, previousValue}
	}
}

func (chaincodeStateDelta *ChaincodeStateDelta) hasChanges() bool {
	return len(chaincodeStateDelta.UpdatedKVs) > 0
}

func (chaincodeStateDelta *ChaincodeStateDelta) getSortedKeys() []string {
	updatedKeys := []string{}
	for k := range chaincodeStateDelta.UpdatedKVs {
		updatedKeys = append(updatedKeys, k)
	}
	sort.Strings(updatedKeys)
	logger.Debugf("Sorted keys = %#v", updatedKeys)
	return updatedKeys
}

// UpdatedValue holds the value for a key
type UpdatedValue struct {
	Value         []byte
	PreviousValue []byte
}

// IsDeleted checks whether the key was deleted
func (updatedValue *UpdatedValue) IsDeleted() bool {
	return updatedValue.Value == nil
}

// GetValue returns the value
func (updatedValue *UpdatedValue) GetValue() []byte {
	return updatedValue.Value
}

// GetPreviousValue returns the previous value
func (updatedValue *UpdatedValue) GetPreviousValue() []byte {
	return updatedValue.PreviousValue
}

// marshalling / Unmarshalling code
// We need to revisit the following when we define proto messages
// for state related structures for transporting. May be we can
// completely get rid of custom marshalling / Unmarshalling of a state delta

// Marshal serializes the StateDelta
func (stateDelta *StateDelta) Marshal() (b []byte) {
	buffer := proto.NewBuffer([]byte{})
	err := buffer.EncodeVarint(uint64(len(stateDelta.ChaincodeStateDeltas)))
	if err != nil {
		// in protobuf code the error return is always nil
		panic(fmt.Errorf("This error should not occure: %s", err))
	}
	for chaincodeID, chaincodeStateDelta := range stateDelta.ChaincodeStateDeltas {
		buffer.EncodeStringBytes(chaincodeID)
		chaincodeStateDelta.marshal(buffer)
	}
	b = buffer.Bytes()
	return
}

func (chaincodeStateDelta *ChaincodeStateDelta) marshal(buffer *proto.Buffer) {
	err := buffer.EncodeVarint(uint64(len(chaincodeStateDelta.UpdatedKVs)))
	if err != nil {
		panic(fmt.Errorf("This error should not occur: %s", err))
	}
	for key, valueHolder := range chaincodeStateDelta.UpdatedKVs {
		err = buffer.EncodeStringBytes(key)
		if err != nil {
			panic(fmt.Errorf("This error should not occur: %s", err))
		}
		chaincodeStateDelta.marshalValueWithMarker(buffer, valueHolder.Value)
		chaincodeStateDelta.marshalValueWithMarker(buffer, valueHolder.PreviousValue)
	}
	return
}

func (chaincodeStateDelta *ChaincodeStateDelta) marshalValueWithMarker(buffer *proto.Buffer, value []byte) {
	if value == nil {
		// Just add a marker that the value is nil
		err := buffer.EncodeVarint(uint64(0))
		if err != nil {
			panic(fmt.Errorf("This error should not occur: %s", err))
		}
		return
	}
	err := buffer.EncodeVarint(uint64(1))
	if err != nil {
		panic(fmt.Errorf("This error should not occur: %s", err))
	}
	// If the value happen to be an empty byte array, it would appear as a nil during
	// deserialization - see method 'unmarshalValueWithMarker'
	err = buffer.EncodeRawBytes(value)
	if err != nil {
		panic(fmt.Errorf("This error should not occur: %s", err))
	}
}

// Unmarshal deserializes StateDelta
func (stateDelta *StateDelta) Unmarshal(bytes []byte) error {
	buffer := proto.NewBuffer(bytes)
	size, err := buffer.DecodeVarint()
	if err != nil {
		return fmt.Errorf("Error unmarashaling size: %s", err)
	}
	stateDelta.ChaincodeStateDeltas = make(map[string]*ChaincodeStateDelta, size)
	for i := uint64(0); i < size; i++ {
		chaincodeID, err := buffer.DecodeStringBytes()
		if err != nil {
			return fmt.Errorf("Error unmarshaling chaincodeID : %s", err)
		}
		chaincodeStateDelta := newChaincodeStateDelta(chaincodeID)
		err = chaincodeStateDelta.unmarshal(buffer)
		if err != nil {
			return fmt.Errorf("Error unmarshalling chaincodeStateDelta : %s", err)
		}
		stateDelta.ChaincodeStateDeltas[chaincodeID] = chaincodeStateDelta
	}

	return nil
}

func (chaincodeStateDelta *ChaincodeStateDelta) unmarshal(buffer *proto.Buffer) error {
	size, err := buffer.DecodeVarint()
	if err != nil {
		return fmt.Errorf("Error unmarshaling state delta: %s", err)
	}
	chaincodeStateDelta.UpdatedKVs = make(map[string]*UpdatedValue, size)
	for i := uint64(0); i < size; i++ {
		key, err := buffer.DecodeStringBytes()
		if err != nil {
			return fmt.Errorf("Error unmarshaling state delta : %s", err)
		}
		value, err := chaincodeStateDelta.unmarshalValueWithMarker(buffer)
		if err != nil {
			return fmt.Errorf("Error unmarshaling state delta : %s", err)
		}
		previousValue, err := chaincodeStateDelta.unmarshalValueWithMarker(buffer)
		if err != nil {
			return fmt.Errorf("Error unmarshaling state delta : %s", err)
		}
		chaincodeStateDelta.UpdatedKVs[key] = &UpdatedValue{value, previousValue}
	}
	return nil
}

func (chaincodeStateDelta *ChaincodeStateDelta) unmarshalValueWithMarker(buffer *proto.Buffer) ([]byte, error) {
	valueMarker, err := buffer.DecodeVarint()
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling state delta : %s", err)
	}
	if valueMarker == 0 {
		return nil, nil
	}
	value, err := buffer.DecodeRawBytes(false)
	if err != nil {
		return nil, fmt.Errorf("Error unmarhsaling state delta : %s", err)
	}
	// protobuff makes an empty []byte into a nil. So, assigning an empty byte array explicitly
	if value == nil {
		value = []byte{}
	}
	return value, nil
}
