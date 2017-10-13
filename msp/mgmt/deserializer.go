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

package mgmt

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	mspproto "github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
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

// DeserializersManager returns a new instance of DeserializersManager
func NewDeserializersManager() DeserializersManager {
	return &mspDeserializersManager{}
}

type mspDeserializersManager struct{}

func (m *mspDeserializersManager) Deserialize(raw []byte) (*mspproto.SerializedIdentity, error) {
	sId := &mspproto.SerializedIdentity{}
	err := proto.Unmarshal(raw, sId)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdentity")
	}
	return sId, nil
}

func (m *mspDeserializersManager) GetLocalMSPIdentifier() string {
	id, _ := GetLocalMSP().GetIdentifier()
	return id
}

func (m *mspDeserializersManager) GetLocalDeserializer() msp.IdentityDeserializer {
	return GetLocalMSP()
}

func (m *mspDeserializersManager) GetChannelDeserializers() map[string]msp.IdentityDeserializer {
	return GetDeserializers()
}
