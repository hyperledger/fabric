/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
)

// Comm is an object that enables to communicate with other peers
// that also embed a CommModule.
type Comm interface {

	// GetPKIid returns this instance's PKI id
	GetPKIid() common.PKIidType

	// Send sends a message to remote peers asynchronously
	Send(msg *protoext.SignedGossipMessage, peers ...*RemotePeer)

	// SendWithAck sends a message to remote peers, waiting for acknowledgement from minAck of them, or until a certain timeout expires
	SendWithAck(msg *protoext.SignedGossipMessage, timeout time.Duration, minAck int, peers ...*RemotePeer) AggregatedSendResult

	// Probe probes a remote node and returns nil if its responsive,
	// and an error if it's not.
	Probe(peer *RemotePeer) error

	// Handshake authenticates a remote peer and returns
	// (its identity, nil) on success and (nil, error)
	Handshake(peer *RemotePeer) (api.PeerIdentityType, error)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// Each message from the channel can be used to send a reply back to the sender
	Accept(common.MessageAcceptor) <-chan protoext.ReceivedMessage

	// PresumedDead returns a read-only channel for node endpoints that are suspected to be offline
	PresumedDead() <-chan common.PKIidType

	// IdentitySwitch returns a read-only channel about identity change events
	IdentitySwitch() <-chan common.PKIidType

	// CloseConn closes a connection to a certain endpoint
	CloseConn(peer *RemotePeer)

	// Stop stops the module
	Stop()
}

// RemotePeer defines a peer's endpoint and its PKIid
type RemotePeer struct {
	Endpoint string
	PKIID    common.PKIidType
}

// SendResult defines a result of a send to a remote peer
type SendResult struct {
	error
	RemotePeer
}

// Error returns the error of the SendResult, or an empty string
// if an error hasn't occurred
func (sr SendResult) Error() string {
	if sr.error != nil {
		return sr.error.Error()
	}
	return ""
}

// AggregatedSendResult represents a slice of SendResults
type AggregatedSendResult []SendResult

// AckCount returns the number of successful acknowledgements
func (ar AggregatedSendResult) AckCount() int {
	c := 0
	for _, ack := range ar {
		if ack.error == nil {
			c++
		}
	}
	return c
}

// NackCount returns the number of unsuccessful acknowledgements
func (ar AggregatedSendResult) NackCount() int {
	return len(ar) - ar.AckCount()
}

// String returns a JSONed string representation
// of the AggregatedSendResult
func (ar AggregatedSendResult) String() string {
	errMap := map[string]int{}
	for _, ack := range ar {
		if ack.error == nil {
			continue
		}
		errMap[ack.Error()]++
	}

	ackCount := ar.AckCount()
	output := map[string]interface{}{}
	if ackCount > 0 {
		output["successes"] = ackCount
	}
	if ackCount < len(ar) {
		output["failures"] = errMap
	}
	b, _ := json.Marshal(output)
	return string(b)
}

// String converts a RemotePeer to a string
func (p *RemotePeer) String() string {
	return fmt.Sprintf("%s, PKIid:%v", p.Endpoint, p.PKIID)
}
