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
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

func TestMaxBytesRule(t *testing.T) {
	dataSize := uint32(100)
	maxBytes := calcMessageBytesForPayloadDataSize(dataSize)
	rs := filter.NewRuleSet([]filter.Rule{MaxBytesRule(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{AbsoluteMaxBytes: maxBytes}}), filter.AcceptRule})

	t.Run("LessThan", func(t *testing.T) {
		_, err := rs.Apply(makeMessage(make([]byte, dataSize-1)))
		if err != nil {
			t.Fatalf("Should have accepted")
		}
	})
	t.Run("Exact", func(t *testing.T) {
		_, err := rs.Apply(makeMessage(make([]byte, dataSize)))
		if err != nil {
			t.Fatalf("Should have accepted")
		}
	})
	t.Run("TooBig", func(t *testing.T) {
		_, err := rs.Apply(makeMessage(make([]byte, dataSize+1)))
		if err == nil {
			t.Fatalf("Should have rejected")
		}
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
