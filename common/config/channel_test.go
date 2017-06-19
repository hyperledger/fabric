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
	"math"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestInterface(t *testing.T) {
	_ = Channel(NewChannelGroup(nil))
}

func TestChannelGroup(t *testing.T) {
	cg := NewChannelGroup(nil)
	assert.NotNil(t, cg, "ChannelGroup should not be nil")
	assert.Nil(t, cg.OrdererConfig(), "OrdererConfig should be nil")
	assert.Nil(t, cg.ApplicationConfig(), "ApplicationConfig should be nil")
	assert.Nil(t, cg.ConsortiumsConfig(), "ConsortiumsConfig should be nil")

	_, err := cg.NewGroup(ApplicationGroupKey)
	assert.NoError(t, err, "Unexpected error for ApplicationGroupKey")
	_, err = cg.NewGroup(OrdererGroupKey)
	assert.NoError(t, err, "Unexpected error for OrdererGroupKey")
	_, err = cg.NewGroup(ConsortiumsGroupKey)
	assert.NoError(t, err, "Unexpected error for ConsortiumsGroupKey")
	_, err = cg.NewGroup("BadGroupKey")
	assert.Error(t, err, "Should have returned error for BadGroupKey")

}

func TestChannelConfig(t *testing.T) {
	cc := NewChannelConfig()
	assert.NotNil(t, cc, "ChannelConfig should not be nil")

	cc.protos = &ChannelProtos{
		HashingAlgorithm:          &cb.HashingAlgorithm{Name: bccsp.SHA256},
		BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: math.MaxUint32},
		OrdererAddresses:          &cb.OrdererAddresses{Addresses: []string{"127.0.0.1:7050"}},
	}

	ag := NewApplicationGroup(nil)
	og := NewOrdererGroup(nil)
	csg := NewConsortiumsGroup(nil)
	good := make(map[string]ValueProposer)
	good[ApplicationGroupKey] = ag
	good[OrdererGroupKey] = og
	good[ConsortiumsGroupKey] = csg

	err := cc.Validate(nil, good)
	assert.NoError(t, err, "Unexpected error validating good config groups")
	err = cc.Validate(nil, map[string]ValueProposer{ApplicationGroupKey: NewConsortiumsGroup(nil)})
	assert.Error(t, err, "Expected error validating bad config group")
	err = cc.Validate(nil, map[string]ValueProposer{OrdererGroupKey: NewConsortiumsGroup(nil)})
	assert.Error(t, err, "Expected error validating bad config group")
	err = cc.Validate(nil, map[string]ValueProposer{ConsortiumsGroupKey: NewOrdererGroup(nil)})
	assert.Error(t, err, "Expected error validating bad config group")
	err = cc.Validate(nil, map[string]ValueProposer{ConsortiumKey: NewConsortiumGroup(nil)})
	assert.Error(t, err, "Expected error validating bad config group")

}

func TestHashingAlgorithm(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{}}}
	assert.Error(t, cc.validateHashingAlgorithm(), "Must supply hashing algorithm")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: "MD5"}}}
	assert.Error(t, cc.validateHashingAlgorithm(), "Bad hashing algorithm supplied")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA256}}}
	assert.NoError(t, cc.validateHashingAlgorithm(), "Allowed hashing algorith SHA256 supplied")

	assert.Equal(t, reflect.ValueOf(util.ComputeSHA256).Pointer(), reflect.ValueOf(cc.HashingAlgorithm()).Pointer(),
		"Unexpected hashing algorithm returned")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA3_256}}}
	assert.NoError(t, cc.validateHashingAlgorithm(), "Allowed hashing algorith SHA3_256 supplied")

	assert.Equal(t, reflect.ValueOf(util.ComputeSHA3256).Pointer(), reflect.ValueOf(cc.HashingAlgorithm()).Pointer(),
		"Unexpected hashing algorithm returned")
}

func TestBlockDataHashingStructure(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{}}}
	assert.Error(t, cc.validateBlockDataHashingStructure(), "Must supply block data hashing structure")

	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: 7}}}
	assert.Error(t, cc.validateBlockDataHashingStructure(), "Invalid Merkle tree width supplied")

	var width uint32
	width = math.MaxUint32
	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: width}}}
	assert.NoError(t, cc.validateBlockDataHashingStructure(), "Valid Merkle tree width supplied")

	assert.Equal(t, width, cc.BlockDataHashingStructureWidth(), "Unexpected width returned")
}

func TestOrdererAddresses(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{}}}
	assert.Error(t, cc.validateOrdererAddresses(), "Must supply orderer addresses")

	cc = &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{Addresses: []string{"127.0.0.1:7050"}}}}
	assert.NoError(t, cc.validateOrdererAddresses(), "Invalid orderer address supplied")

	assert.Equal(t, "127.0.0.1:7050", cc.OrdererAddresses()[0], "Unexpected orderer address returned")
}

func TestConsortiumName(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{Consortium: &cb.Consortium{Name: "TestConsortium"}}}
	assert.Equal(t, "TestConsortium", cc.ConsortiumName(), "Unexpected consortium name returned")
}

func TestChannelUtils(t *testing.T) {
	// these functions all panic if marshaling fails so just executing them is sufficient
	_ = TemplateConsortium("test")
	_ = DefaultHashingAlgorithm()
	_ = DefaultBlockDataHashingStructure()
	_ = DefaultOrdererAddresses()

}
