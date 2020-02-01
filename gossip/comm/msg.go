/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/pkg/errors"
)

// ReceivedMessageImpl is an implementation of ReceivedMessage
type ReceivedMessageImpl struct {
	*protoext.SignedGossipMessage
	conn     *connection
	connInfo *protoext.ConnectionInfo
}

// GetSourceEnvelope Returns the Envelope the ReceivedMessage was
// constructed with
func (m *ReceivedMessageImpl) GetSourceEnvelope() *proto.Envelope {
	return m.Envelope
}

// Respond sends a msg to the source that sent the ReceivedMessageImpl
func (m *ReceivedMessageImpl) Respond(msg *proto.GossipMessage) {
	sMsg, err := protoext.NoopSign(msg)
	if err != nil {
		err = errors.WithStack(err)
		m.conn.logger.Errorf("Failed creating SignedGossipMessage: %+v", err)
		return
	}
	m.conn.send(sMsg, func(e error) {}, blockingSend)
}

// GetGossipMessage returns the inner GossipMessage
func (m *ReceivedMessageImpl) GetGossipMessage() *protoext.SignedGossipMessage {
	return m.SignedGossipMessage
}

// GetConnectionInfo returns information about the remote peer
// that send the message
func (m *ReceivedMessageImpl) GetConnectionInfo() *protoext.ConnectionInfo {
	return m.connInfo
}

// Ack returns to the sender an acknowledgement for the message
func (m *ReceivedMessageImpl) Ack(err error) {
	ackMsg := &proto.GossipMessage{
		Nonce: m.GetGossipMessage().Nonce,
		Content: &proto.GossipMessage_Ack{
			Ack: &proto.Acknowledgement{},
		},
	}
	if err != nil {
		ackMsg.GetAck().Error = err.Error()
	}
	m.Respond(ackMsg)
}
