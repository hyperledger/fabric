/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"math"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func TestInterface(t *testing.T) {
	_ = Channel(&ChannelConfig{})
}

func TestChannelConfig(t *testing.T) {
	cc, err := NewChannelConfig(&cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			"UnknownGroupKey": {},
		},
	})
	assert.Error(t, err)
	assert.Nil(t, cc)
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
