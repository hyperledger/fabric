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

package protolator

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/tools/protolator/testprotos"
	"github.com/hyperledger/fabric/protos/utils"
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
			OpaqueField: utils.MarshalOrPanic(&testprotos.SimpleMsg{
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
				OpaqueField: utils.MarshalOrPanic(&testprotos.SimpleMsg{
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
				OpaqueField: utils.MarshalOrPanic(&testprotos.SimpleMsg{
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
