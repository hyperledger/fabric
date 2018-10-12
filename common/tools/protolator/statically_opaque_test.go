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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/tools/protolator/testprotos"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func extractSimpleMsgPlainField(source []byte) string {
	result := &testprotos.SimpleMsg{}
	err := proto.Unmarshal(source, result)
	if err != nil {
		panic(err)
	}
	return result.PlainField
}

func TestPlainStaticallyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	startMsg := &testprotos.StaticallyOpaqueMsg{
		PlainOpaqueField: utils.MarshalOrPanic(&testprotos.SimpleMsg{
			PlainField: pfValue,
		}),
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.StaticallyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.PlainOpaqueField), extractSimpleMsgPlainField(newMsg.PlainOpaqueField))

	fieldFactories = []protoFieldFactory{tppff, staticallyOpaqueFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.PlainOpaqueField), extractSimpleMsgPlainField(newMsg.PlainOpaqueField))
}

func TestMapStaticallyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	mapKey := "bar"
	startMsg := &testprotos.StaticallyOpaqueMsg{
		MapOpaqueField: map[string][]byte{
			mapKey: utils.MarshalOrPanic(&testprotos.SimpleMsg{
				PlainField: pfValue,
			}),
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.StaticallyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.MapOpaqueField[mapKey]), extractSimpleMsgPlainField(newMsg.MapOpaqueField[mapKey]))

	fieldFactories = []protoFieldFactory{tppff, staticallyOpaqueMapFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.MapOpaqueField[mapKey]), extractSimpleMsgPlainField(newMsg.MapOpaqueField[mapKey]))
}

func TestSliceStaticallyOpaqueMsg(t *testing.T) {
	fromPrefix := "from"
	toPrefix := "to"
	tppff := &testProtoPlainFieldFactory{
		fromPrefix: fromPrefix,
		toPrefix:   toPrefix,
	}

	fieldFactories = []protoFieldFactory{tppff}

	pfValue := "foo"
	startMsg := &testprotos.StaticallyOpaqueMsg{
		SliceOpaqueField: [][]byte{
			utils.MarshalOrPanic(&testprotos.SimpleMsg{
				PlainField: pfValue,
			}),
		},
	}

	var buffer bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	newMsg := &testprotos.StaticallyOpaqueMsg{}
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.NotEqual(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.SliceOpaqueField[0]), extractSimpleMsgPlainField(newMsg.SliceOpaqueField[0]))

	fieldFactories = []protoFieldFactory{tppff, staticallyOpaqueSliceFieldFactory{}}

	buffer.Reset()
	assert.NoError(t, DeepMarshalJSON(&buffer, startMsg))
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newMsg))
	assert.Equal(t, fromPrefix+toPrefix+extractSimpleMsgPlainField(startMsg.SliceOpaqueField[0]), extractSimpleMsgPlainField(newMsg.SliceOpaqueField[0]))
}

func TestIgnoredNilFields(t *testing.T) {
	_ = StaticallyOpaqueFieldProto(&testprotos.UnmarshalableDeepFields{})
	_ = StaticallyOpaqueMapFieldProto(&testprotos.UnmarshalableDeepFields{})
	_ = StaticallyOpaqueSliceFieldProto(&testprotos.UnmarshalableDeepFields{})

	fieldFactories = []protoFieldFactory{
		staticallyOpaqueFieldFactory{},
		staticallyOpaqueMapFieldFactory{},
		staticallyOpaqueSliceFieldFactory{},
	}

	assert.Error(t, DeepMarshalJSON(&bytes.Buffer{}, &testprotos.UnmarshalableDeepFields{
		PlainOpaqueField: []byte("fake"),
	}))
	assert.Error(t, DeepMarshalJSON(&bytes.Buffer{}, &testprotos.UnmarshalableDeepFields{
		MapOpaqueField: map[string][]byte{"foo": []byte("bar")},
	}))
	assert.Error(t, DeepMarshalJSON(&bytes.Buffer{}, &testprotos.UnmarshalableDeepFields{
		SliceOpaqueField: [][]byte{[]byte("bar")},
	}))
	assert.NoError(t, DeepMarshalJSON(&bytes.Buffer{}, &testprotos.UnmarshalableDeepFields{}))
}
