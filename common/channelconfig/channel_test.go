/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"math"
	"reflect"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/util"
	"github.com/stretchr/testify/require"
)

func TestInterface(t *testing.T) {
	_ = Channel(&ChannelConfig{})
}

func TestChannelConfig(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	cc, err := NewChannelConfig(
		&cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"UnknownGroupKey": {}}},
		cryptoProvider,
	)
	require.Error(t, err)
	require.Nil(t, cc)
}

func TestHashingAlgorithm(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{}}}
	require.Error(t, cc.validateHashingAlgorithm(), "Must supply hashing algorithm")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: "MD5"}}}
	require.Error(t, cc.validateHashingAlgorithm(), "Bad hashing algorithm supplied")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA256}}}
	require.NoError(t, cc.validateHashingAlgorithm(), "Allowed hashing algorithm SHA256 supplied")

	require.Equal(t, reflect.ValueOf(util.ComputeSHA256).Pointer(), reflect.ValueOf(cc.HashingAlgorithm()).Pointer(),
		"Unexpected hashing algorithm returned")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA3_256}}}
	require.NoError(t, cc.validateHashingAlgorithm(), "Allowed hashing algorithm SHA3_256 supplied")

	require.Equal(t, reflect.ValueOf(util.ComputeSHA3256).Pointer(), reflect.ValueOf(cc.HashingAlgorithm()).Pointer(),
		"Unexpected hashing algorithm returned")
}

func TestBlockDataHashingStructure(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{}}}
	require.Error(t, cc.validateBlockDataHashingStructure(), "Must supply block data hashing structure")

	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: 7}}}
	require.Error(t, cc.validateBlockDataHashingStructure(), "Invalid Merkle tree width supplied")

	var width uint32 = math.MaxUint32
	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: width}}}
	require.NoError(t, cc.validateBlockDataHashingStructure(), "Valid Merkle tree width supplied")

	require.Equal(t, width, cc.BlockDataHashingStructureWidth(), "Unexpected width returned")
}

func TestOrdererAddresses(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{}}}
	require.Error(t, cc.validateOrdererAddresses(), "Must supply orderer addresses")

	cc = &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{Addresses: []string{"127.0.0.1:7050"}}}}
	require.NoError(t, cc.validateOrdererAddresses(), "Invalid orderer address supplied")

	require.Equal(t, "127.0.0.1:7050", cc.OrdererAddresses()[0], "Unexpected orderer address returned")
}

func TestConsortiumName(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{Consortium: &cb.Consortium{Name: "TestConsortium"}}}
	require.Equal(t, "TestConsortium", cc.ConsortiumName(), "Unexpected consortium name returned")
}
