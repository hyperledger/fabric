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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

type foo struct {
	Msg1 *cb.Envelope
	Msg2 *cb.Payload
}

type bar struct {
	Msg3 *cb.Header
}

type conflict struct {
	Msg1 *cb.Header
}

type nonProtos struct {
	Msg   *cb.Envelope
	Wrong string
}

type unexported struct {
	msg *cb.Envelope
}

func TestSingle(t *testing.T) {
	fooVal := &foo{}
	sv, err := NewStandardValues(fooVal)
	assert.NoError(t, err, "Valid non-nested structure provided")
	assert.NotNil(t, fooVal.Msg1, "Should have initialized Msg1")
	assert.NotNil(t, fooVal.Msg2, "Should have initialized Msg2")

	msg1, err := sv.Deserialize("Msg1", utils.MarshalOrPanic(&cb.Envelope{}))
	assert.NoError(t, err, "Should have found map entry")
	assert.Equal(t, msg1, fooVal.Msg1, "Should be same entry")

	msg2, err := sv.Deserialize("Msg2", utils.MarshalOrPanic(&cb.Payload{}))
	assert.NoError(t, err, "Should have found map entry")
	assert.Equal(t, msg2, fooVal.Msg2, "Should be same entry")
}

func TestPair(t *testing.T) {
	fooVal := &foo{}
	barVal := &bar{}
	sv, err := NewStandardValues(fooVal, barVal)
	assert.NoError(t, err, "Valid non-nested structure provided")
	assert.NotNil(t, fooVal.Msg1, "Should have initialized Msg1")
	assert.NotNil(t, fooVal.Msg2, "Should have initialized Msg2")
	assert.NotNil(t, barVal.Msg3, "Should have initialized Msg3")

	msg1, err := sv.Deserialize("Msg1", utils.MarshalOrPanic(&cb.Envelope{}))
	assert.NoError(t, err, "Should have found map entry")
	assert.Equal(t, msg1, fooVal.Msg1, "Should be same entry")

	msg2, err := sv.Deserialize("Msg2", utils.MarshalOrPanic(&cb.Payload{}))
	assert.NoError(t, err, "Should have found map entry")
	assert.Equal(t, msg2, fooVal.Msg2, "Should be same entry")

	msg3, err := sv.Deserialize("Msg3", utils.MarshalOrPanic(&cb.Header{}))
	assert.NoError(t, err, "Should have found map entry")
	assert.Equal(t, msg3, barVal.Msg3, "Should be same entry")
}

func TestPairConflict(t *testing.T) {
	_, err := NewStandardValues(&foo{}, &conflict{})
	assert.Error(t, err, "Conflicting keys provided")
}

func TestNonProtosStruct(t *testing.T) {
	_, err := NewStandardValues(&nonProtos{})
	assert.Error(t, err, "Structure with non-struct non-proto fields provided")
}

func TestUnexportedField(t *testing.T) {
	_, err := NewStandardValues(&unexported{})
	assert.Error(t, err, "Structure with unexported fields")
}

func TestNonPointerParam(t *testing.T) {
	_, err := NewStandardValues(foo{})
	assert.Error(t, err, "Parameter must be pointer")
}

func TestPointerToNonStruct(t *testing.T) {
	nonStruct := "foo"
	_, err := NewStandardValues(&nonStruct)
	assert.Error(t, err, "Pointer must be to a struct")
}
