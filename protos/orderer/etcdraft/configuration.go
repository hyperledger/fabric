/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	fmt "fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

// TypeKey is the string with which this consensus implementation is identified across Fabric.
const TypeKey = "etcdraft"

func init() {
	orderer.ConsensusTypeMetadataMap[TypeKey] = ConsensusTypeMetadataFactory{}
}

// ConsensusTypeMetadataFactory allows this implementation's proto messages to register
// their type with the orderer's proto messages. This is needed for protolator to work.
type ConsensusTypeMetadataFactory struct{}

// NewMessage implements the Orderer.ConsensusTypeMetadataFactory interface.
func (dogf ConsensusTypeMetadataFactory) NewMessage() proto.Message {
	return &Metadata{}
}

// Marshal serializes this implementation's proto messages. It is called by the encoder package
// during the creation of the Orderer ConfigGroup.
func Marshal(md *Metadata) ([]byte, error) {
	for _, c := range md.Consenters {
		// Expect the user to set the config value for client/server certs to the
		// path where they are persisted locally, then load these files to memory.
		clientCert, err := ioutil.ReadFile(string(c.GetClientTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load client cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ClientTlsCert = clientCert

		serverCert, err := ioutil.ReadFile(string(c.GetServerTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load server cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ServerTlsCert = serverCert
	}
	return proto.Marshal(md)
}
