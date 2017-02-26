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
	"testing"

	"github.com/hyperledger/fabric/bccsp"
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

func TestHashingAlgorithm(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{}}}
	assert.Error(t, cc.validateHashingAlgorithm(), "Must supply hashing algorithm")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: "MD5"}}}
	assert.Error(t, cc.validateHashingAlgorithm(), "Bad hashing algorithm supplied")

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA256}}}
	assert.NoError(t, cc.validateHashingAlgorithm(), "Allowed hashing algorith supplied")
}

func TestBlockDataHashingStructure(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{}}}
	assert.Error(t, cc.validateBlockDataHashingStructure(), "Must supply block data hashing structure")

	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: 7}}}
	assert.Error(t, cc.validateBlockDataHashingStructure(), "Invalid Merkle tree width supplied")

	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: math.MaxUint32}}}
	assert.NoError(t, cc.validateBlockDataHashingStructure(), "Valid Merkle tree width supplied")
}

func TestOrdererAddresses(t *testing.T) {
	cc := &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{}}}
	assert.Error(t, cc.validateOrdererAddresses(), "Must supply orderer addresses")

	cc = &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{Addresses: []string{"127.0.0.1:7050"}}}}
	assert.NoError(t, cc.validateOrdererAddresses(), "Invalid Merkle tree width supplied")
}
