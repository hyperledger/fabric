/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protolator

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/tools/protolator/testprotos"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func extractNestedMsgPlainField(source []byte) string {
	result := &testprotos.NestedMsg{}
	err := proto.Unmarshal(source, result)
	if err != nil {
		panic(err)
	}
	return result.PlainNestedField.PlainField
}

func TestPlainVariablyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	startMsg := &testprotos.VariablyOpaqueMsg{
		OpaqueType: "NestedMsg",
		PlainOpaqueField: protoutil.MarshalOrPanic(&testprotos.NestedMsg{
			PlainNestedField: &testprotos.SimpleMsg{
				PlainField: pfValue,
			},
		}),
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.VariablyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.PlainOpaqueField), extractNestedMsgPlainField(newMsg.PlainOpaqueField))

	fieldFactories = []protoFieldFactory{tppff, nestedFieldFactory{}, variablyOpaqueFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.PlainOpaqueField), extractNestedMsgPlainField(newMsg.PlainOpaqueField))
}

func TestMapVariablyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	mapKey := "bar"
	startMsg := &testprotos.VariablyOpaqueMsg{
		OpaqueType: "NestedMsg",
		MapOpaqueField: map[string][]byte{
			mapKey: protoutil.MarshalOrPanic(&testprotos.NestedMsg{
				PlainNestedField: &testprotos.SimpleMsg{
					PlainField: pfValue,
				},
			}),
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.VariablyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.MapOpaqueField[mapKey]), extractNestedMsgPlainField(newMsg.MapOpaqueField[mapKey]))

	fieldFactories = []protoFieldFactory{tppff, nestedFieldFactory{}, variablyOpaqueMapFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.MapOpaqueField[mapKey]), extractNestedMsgPlainField(newMsg.MapOpaqueField[mapKey]))
}

func TestSliceVariablyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	startMsg := &testprotos.VariablyOpaqueMsg{
		OpaqueType: "NestedMsg",
		SliceOpaqueField: [][]byte{
			protoutil.MarshalOrPanic(&testprotos.NestedMsg{
				PlainNestedField: &testprotos.SimpleMsg{
					PlainField: pfValue,
				},
			}),
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.VariablyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.SliceOpaqueField[0]), extractNestedMsgPlainField(newMsg.SliceOpaqueField[0]))

	fieldFactories = []protoFieldFactory{tppff, nestedFieldFactory{}, variablyOpaqueSliceFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractNestedMsgPlainField(startMsg.SliceOpaqueField[0]), extractNestedMsgPlainField(newMsg.SliceOpaqueField[0]))
}
