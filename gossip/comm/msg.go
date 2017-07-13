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
