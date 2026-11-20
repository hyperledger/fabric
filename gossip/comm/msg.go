/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync/atomic"

	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/pkg/errors"
)

// ReceivedMessageImpl is an implementation of ReceivedMessage
type ReceivedMessageImpl struct {
	*protoext.SignedGossipMessage
	conn     *connection
	connInfo *protoext.ConnectionInfo
	// pendingDeliveries counts deliveries not yet consumed; onConsumed releases
	// the message's receive-path byte accounting once it drains to zero.
	pendingDeliveries atomic.Int32
	onConsumed        func()
}

// trackDeliveries records the number of deliveries about to be made; zero
// means the message is never delivered and is released immediately.
func (m *ReceivedMessageImpl) trackDeliveries(n int) {
	if n == 0 {
		m.fireConsumed()
		return
	}
	m.pendingDeliveries.Add(int32(n))
}

// Dispose marks one delivery of the message as consumed. Consumers of
// comm.Accept channels must call it once done with the message, so the
// receive-path byte accounting of the sending connection is released.
func (m *ReceivedMessageImpl) Dispose() {
	for {
		pending := m.pendingDeliveries.Load()
		if pending <= 0 {
			// Never goes negative: a double dispose breaks the tracking
			// contract and must be visible.
			if m.conn != nil && m.conn.logger != nil {
				m.conn.logger.Warning("Dispose called with no pending deliveries")
			}
			return
		}
		if m.pendingDeliveries.CompareAndSwap(pending, pending-1) {
			if pending == 1 {
				m.fireConsumed()
			}
			return
		}
	}
}

func (m *ReceivedMessageImpl) fireConsumed() {
	if m.onConsumed != nil {
		m.onConsumed()
	}
}

// DisposeMessage releases the receive-path byte accounting held by msg, if
// any. Consumers of Accept channels must call it once done with the message.
func DisposeMessage(msg protoext.ReceivedMessage) {
	if m, ok := msg.(*ReceivedMessageImpl); ok {
		m.Dispose()
	}
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
// that sends the message
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
