/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "Valid non-nested structure provided")
	require.NotNil(t, fooVal.Msg1, "Should have initialized Msg1")
	require.NotNil(t, fooVal.Msg2, "Should have initialized Msg2")

	msg1, err := sv.Deserialize("Msg1", protoutil.MarshalOrPanic(&cb.Envelope{}))
	require.NoError(t, err, "Should have found map entry")
	require.Equal(t, msg1, fooVal.Msg1, "Should be same entry")

	msg2, err := sv.Deserialize("Msg2", protoutil.MarshalOrPanic(&cb.Payload{}))
	require.NoError(t, err, "Should have found map entry")
	require.Equal(t, msg2, fooVal.Msg2, "Should be same entry")
}

func TestPair(t *testing.T) {
	fooVal := &foo{}
	barVal := &bar{}
	sv, err := NewStandardValues(fooVal, barVal)
	require.NoError(t, err, "Valid non-nested structure provided")
	require.NotNil(t, fooVal.Msg1, "Should have initialized Msg1")
	require.NotNil(t, fooVal.Msg2, "Should have initialized Msg2")
	require.NotNil(t, barVal.Msg3, "Should have initialized Msg3")

	msg1, err := sv.Deserialize("Msg1", protoutil.MarshalOrPanic(&cb.Envelope{}))
	require.NoError(t, err, "Should have found map entry")
	require.Equal(t, msg1, fooVal.Msg1, "Should be same entry")

	msg2, err := sv.Deserialize("Msg2", protoutil.MarshalOrPanic(&cb.Payload{}))
	require.NoError(t, err, "Should have found map entry")
	require.Equal(t, msg2, fooVal.Msg2, "Should be same entry")

	msg3, err := sv.Deserialize("Msg3", protoutil.MarshalOrPanic(&cb.Header{}))
	require.NoError(t, err, "Should have found map entry")
	require.Equal(t, msg3, barVal.Msg3, "Should be same entry")
}

func TestPairConflict(t *testing.T) {
	_, err := NewStandardValues(&foo{}, &conflict{})
	require.Error(t, err, "Conflicting keys provided")
}

func TestNonProtosStruct(t *testing.T) {
	_, err := NewStandardValues(&nonProtos{})
	require.Error(t, err, "Structure with non-struct non-proto fields provided")
}

func TestUnexportedField(t *testing.T) {
	_, err := NewStandardValues(&unexported{msg: nil})
	require.Error(t, err, "Structure with unexported fields")
}

func TestNonPointerParam(t *testing.T) {
	_, err := NewStandardValues(foo{})
	require.Error(t, err, "Parameter must be pointer")
}

func TestPointerToNonStruct(t *testing.T) {
	nonStruct := "foo"
	_, err := NewStandardValues(&nonStruct)
	require.Error(t, err, "Pointer must be to a struct")
}
