/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protolator

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/tools/protolator/testprotos"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestPlainDynamicMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}}

	pfValue := "foo"
	startMsg := &testprotos.DynamicMsg{
		DynamicType: "SimpleMsg",
		PlainDynamicField: &testprotos.ContextlessMsg{
			OpaqueField: protoutil.MarshalOrPanic(&testprotos.SimpleMsg{
				PlainField: pfValue,
			}),
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.DynamicMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.PlainDynamicField.OpaqueField), extractSimpleMsgPlainField(newMsg.PlainDynamicField.OpaqueField))

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}, dynamicFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.PlainDynamicField.OpaqueField), extractSimpleMsgPlainField(newMsg.PlainDynamicField.OpaqueField))
}

func TestMapDynamicMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}}

	pfValue := "foo"
	mapKey := "bar"
	startMsg := &testprotos.DynamicMsg{
		DynamicType: "SimpleMsg",
		MapDynamicField: map[string]*testprotos.ContextlessMsg{
			mapKey: {
				OpaqueField: protoutil.MarshalOrPanic(&testprotos.SimpleMsg{
					PlainField: pfValue,
				}),
			},
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.DynamicMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.MapDynamicField[mapKey].OpaqueField), extractSimpleMsgPlainField(newMsg.MapDynamicField[mapKey].OpaqueField))

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}, dynamicMapFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.MapDynamicField[mapKey].OpaqueField), extractSimpleMsgPlainField(newMsg.MapDynamicField[mapKey].OpaqueField))
}

func TestSliceDynamicMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}}

	pfValue := "foo"
	startMsg := &testprotos.DynamicMsg{
		DynamicType: "SimpleMsg",
		SliceDynamicField: []*testprotos.ContextlessMsg{
			{
				OpaqueField: protoutil.MarshalOrPanic(&testprotos.SimpleMsg{
					PlainField: pfValue,
				}),
			},
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.DynamicMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.SliceDynamicField[0].OpaqueField), extractSimpleMsgPlainField(newMsg.SliceDynamicField[0].OpaqueField))

	fieldFactories = []protoFieldFactory{tppff, variablyOpaqueFieldFactory{}, dynamicSliceFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.SliceDynamicField[0].OpaqueField), extractSimpleMsgPlainField(newMsg.SliceDynamicField[0].OpaqueField))
}
