/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync"

	proto "github.com/hyperledger/fabric/protos/gossip"
)

// ReceivedMessageImpl is an implementation of ReceivedMessage
type ReceivedMessageImpl struct {
	*proto.SignedGossipMessage
	lock     sync.Locker
	conn     *connection
	connInfo *proto.ConnectionInfo
}

// GetSourceEnvelope Returns the Envelope the ReceivedMessage was
// constructed with
func (m *ReceivedMessageImpl) GetSourceEnvelope() *proto.Envelope {
	return m.Envelope
}

// Respond sends a msg to the source that sent the ReceivedMessageImpl
func (m *ReceivedMessageImpl) Respond(msg *proto.GossipMessage) {
	sMsg, err := msg.NoopSign()
	if err != nil {
		m.conn.logger.Error("Failed creating SignedGossipMessage:", err)
		return
	}
	m.conn.send(sMsg, func(e error) {})
}

// GetGossipMessage returns the inner GossipMessage
func (m *ReceivedMessageImpl) GetGossipMessage() *proto.SignedGossipMessage {
	return m.SignedGossipMessage
}

// GetConnectionInfo returns information about the remote peer
// that send the message
func (m *ReceivedMessageImpl) GetConnectionInfo() *proto.ConnectionInfo {
	return m.connInfo
}
