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

package sizefilter

import (
	"testing"

	"github.com/golang/protobuf/proto"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/stretchr/testify/assert"
)

func TestMaxBytesRule(t *testing.T) {
	dataSize := uint32(100)
	maxBytes := calcMessageBytesForPayloadDataSize(dataSize)
	msf := New(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{AbsoluteMaxBytes: maxBytes}})

	t.Run("LessThan", func(t *testing.T) {
		assert.Nil(t, msf.Apply(makeMessage(make([]byte, dataSize-1))))
	})
	t.Run("Exact", func(t *testing.T) {
		assert.Nil(t, msf.Apply(makeMessage(make([]byte, dataSize))))
	})
	t.Run("TooBig", func(t *testing.T) {
		assert.NotNil(t, msf.Apply(makeMessage(make([]byte, dataSize+1))))
	})
}

func calcMessageBytesForPayloadDataSize(dataSize uint32) uint32 {
	return messageByteSize(makeMessage(make([]byte, dataSize)))
}

func makeMessage(data []byte) *cb.Envelope {
	data, err := proto.Marshal(&cb.Payload{Data: data})
	if err != nil {
		panic(err)
	}
	return &cb.Envelope{Payload: data}
}
