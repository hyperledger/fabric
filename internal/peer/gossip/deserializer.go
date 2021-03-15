/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

// DeserializersManager is a support interface to
// access the local and channel deserializers
type DeserializersManager interface {

	// Deserialize receives SerializedIdentity bytes and returns the unmarshaled form
	// of the SerializedIdentity, or error on failure
	Deserialize(raw []byte) (*mspproto.SerializedIdentity, error)

	// GetLocalMSPIdentifier returns the local MSP identifier
	GetLocalMSPIdentifier() string

	// GetLocalDeserializer returns the local identity deserializer
	GetLocalDeserializer() msp.IdentityDeserializer

	// GetChannelDeserializers returns a map of the channel deserializers
	GetChannelDeserializers() map[string]msp.IdentityDeserializer
}

type ChannelDeserializersFunc func() map[string]msp.IdentityDeserializer

// NewDeserializersManager returns a new instance of DeserializersManager
func NewDeserializersManager(localMSP msp.MSP, getChannelDeserializers ChannelDeserializersFunc) DeserializersManager {
	return &mspDeserializersManager{
		localMSP:                localMSP,
		getChannelDeserializers: getChannelDeserializers,
	}
}

type mspDeserializersManager struct {
	localMSP                msp.MSP
	getChannelDeserializers ChannelDeserializersFunc
}

func (m *mspDeserializersManager) Deserialize(raw []byte) (*mspproto.SerializedIdentity, error) {
	return protoutil.UnmarshalSerializedIdentity(raw)
}

func (m *mspDeserializersManager) GetLocalMSPIdentifier() string {
	id, _ := m.localMSP.GetIdentifier()
	return id
}

func (m *mspDeserializersManager) GetLocalDeserializer() msp.IdentityDeserializer {
	return m.localMSP
}

func (m *mspDeserializersManager) GetChannelDeserializers() map[string]msp.IdentityDeserializer {
	return m.getChannelDeserializers()
}
