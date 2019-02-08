/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/orderer"
)

func init() {
	orderer.ConsensusTypeMetadataMap["etcdraft"] = ConsensusTypeMetadataFactory{}
}

// ConsensusTypeMetadataFactory allows this implementation's proto messages to register
// their type with the orderer's proto messages. This is needed for protolator to work.
type ConsensusTypeMetadataFactory struct{}

// NewMessage implements the Orderer.ConsensusTypeMetadataFactory interface.
func (dogf ConsensusTypeMetadataFactory) NewMessage() proto.Message {
	return &Metadata{}
}
