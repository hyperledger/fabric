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

package config

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
)

func nearIdentityHash(input []byte) []byte {
	return util.ConcatenateBytes([]byte("FakeHash("), input, []byte(""))
}

// Channel is a mock implementation of config.Channel
type Channel struct {
	// HashingAlgorithmVal is returned as the result of HashingAlgorithm() if set
	HashingAlgorithmVal func([]byte) []byte
	// BlockDataHashingStructureWidthVal is returned as the result of BlockDataHashingStructureWidth()
	BlockDataHashingStructureWidthVal uint32
	// OrdererAddressesVal is returned as the result of OrdererAddresses()
	OrdererAddressesVal []string
	// CapabilitiesVal is returned as the result of Capabilities()
	CapabilitiesVal channelconfig.ChannelCapabilities
}

// HashingAlgorithm returns the HashingAlgorithmVal if set, otherwise a fake simple hash function
func (scm *Channel) HashingAlgorithm() func([]byte) []byte {
	if scm.HashingAlgorithmVal == nil {
		return nearIdentityHash
	}
	return scm.HashingAlgorithmVal
}

// BlockDataHashingStructureWidth returns the BlockDataHashingStructureWidthVal
func (scm *Channel) BlockDataHashingStructureWidth() uint32 {
	return scm.BlockDataHashingStructureWidthVal
}

// OrdererAddresses returns the OrdererAddressesVal
func (scm *Channel) OrdererAddresses() []string {
	return scm.OrdererAddressesVal
}

// Capabilities returns CapabilitiesVal
func (scm *Channel) Capabilities() channelconfig.ChannelCapabilities {
	return scm.CapabilitiesVal
}

// ChannelCapabilities mocks the channelconfig.ChannelCapabilities interface
type ChannelCapabilities struct {
	// SupportedErr is returned by Supported()
	SupportedErr error

	// MSPVersionVal is returned by MSPVersion()
	MSPVersionVal msp.MSPVersion
}

// Supported returns SupportedErr
func (cc *ChannelCapabilities) Supported() error {
	return cc.SupportedErr
}

// MSPVersion returns MSPVersionVal
func (cc *ChannelCapabilities) MSPVersion() msp.MSPVersion {
	return cc.MSPVersionVal
}
