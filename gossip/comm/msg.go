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

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/protos/gossip"
)

// ReceivedMessageImpl is an implementation of ReceivedMessage
type ReceivedMessageImpl struct {
	*proto.GossipMessage
	lock sync.Locker
	conn *connection
}

// Respond sends a msg to the source that sent the ReceivedMessageImpl
func (m *ReceivedMessageImpl) Respond(msg *proto.GossipMessage) {
	m.conn.send(msg, func(e error) {})
}

// GetGossipMessage returns the inner GossipMessage
func (m *ReceivedMessageImpl) GetGossipMessage() *proto.GossipMessage {
	return m.GossipMessage
}

// GetPKIID returns the PKI-ID of the remote peer
// that sent the message
func (m *ReceivedMessageImpl) GetPKIID() common.PKIidType {
	return m.conn.pkiID
}
