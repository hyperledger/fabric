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

	"github.com/golang/protobuf/proto"
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
		PlainOpaqueField: utils.MarshalOrPanic(&testprotos.NestedMsg{
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
			mapKey: utils.MarshalOrPanic(&testprotos.NestedMsg{
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
			utils.MarshalOrPanic(&testprotos.NestedMsg{
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
