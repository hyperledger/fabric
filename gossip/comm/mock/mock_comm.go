/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
)

// Mock which aims to simulate socket
type socketMock struct {
	// socket endpoint
	endpoint string

	// To simulate simple tcp socket
	socket chan interface{}
}

// Mock of primitive tcp packet structure
type packetMock struct {
	// Sender channel message sent from
	src *socketMock

	// Destination channel sent to
	dst *socketMock

	msg interface{}
}

type channelMock struct {
	accept common.MessageAcceptor

	channel chan protoext.ReceivedMessage
}

type commMock struct {
	id string

	members map[string]*socketMock

	acceptors []*channelMock

	deadChannel chan common.PKIidType

	done chan struct{}
}

var logger = util.GetLogger(util.CommMockLogger, "")

// NewCommMock creates mocked communication object
func NewCommMock(id string, members map[string]*socketMock) comm.Comm {
	res := &commMock{
		id: id,

		members: members,

		acceptors: make([]*channelMock, 0),

		done: make(chan struct{}),

		deadChannel: make(chan common.PKIidType),
	}
	// Start communication service
	go res.start()

	return res
}

// Respond sends a GossipMessage to the origin from which this ReceivedMessage was sent from
func (packet *packetMock) Respond(msg *proto.GossipMessage) {
	sMsg, _ := protoext.NoopSign(msg)
	packet.src.socket <- &packetMock{
		src: packet.dst,
		dst: packet.src,
		msg: sMsg,
	}
}

// Ack returns to the sender an acknowledgement for the message
func (packet *packetMock) Ack(err error) {
}

// GetSourceEnvelope Returns the Envelope the ReceivedMessage was
// constructed with
func (packet *packetMock) GetSourceEnvelope() *proto.Envelope {
	return nil
}

// GetGossipMessage returns the underlying GossipMessage
func (packet *packetMock) GetGossipMessage() *protoext.SignedGossipMessage {
	return packet.msg.(*protoext.SignedGossipMessage)
}

// GetConnectionInfo returns information about the remote peer
// that sent the message
func (packet *packetMock) GetConnectionInfo() *protoext.ConnectionInfo {
	return nil
}

func (mock *commMock) start() {
	logger.Debug("Starting communication mock module...")
	for {
		select {
		case <-mock.done:
			{
				// Got final signal, exiting...
				logger.Debug("Exiting...")
				return
			}
		case msg := <-mock.members[mock.id].socket:
			{
				logger.Debug("Got new message", msg)
				packet := msg.(*packetMock)
				for _, channel := range mock.acceptors {
					// if message acceptor agrees to get
					// new message forward it to the received
					// messages channel
					if channel.accept(packet) {
						channel.channel <- packet
					}
				}
			}
		}
	}
}

func (mock *commMock) IdentitySwitch() <-chan common.PKIidType {
	panic("implement me")
}

// GetPKIid returns this instance's PKI id
func (mock *commMock) GetPKIid() common.PKIidType {
	return common.PKIidType(mock.id)
}

// Send sends a message to remote peers asynchronously
func (mock *commMock) Send(msg *protoext.SignedGossipMessage, peers ...*comm.RemotePeer) {
	for _, peer := range peers {
		logger.Debug("Sending message to peer ", peer.Endpoint, "from ", mock.id)
		mock.members[peer.Endpoint].socket <- &packetMock{
			src: mock.members[mock.id],
			dst: mock.members[peer.Endpoint],
			msg: msg,
		}
	}
}

func (mock *commMock) SendWithAck(_ *protoext.SignedGossipMessage, _ time.Duration, _ int, _ ...*comm.RemotePeer) comm.AggregatedSendResult {
	panic("not implemented")
}

// Probe probes a remote node and returns nil if its responsive,
// and an error if it's not.
func (mock *commMock) Probe(peer *comm.RemotePeer) error {
	return nil
}

// Handshake authenticates a remote peer and returns
// (its identity, nil) on success and (nil, error)
func (mock *commMock) Handshake(peer *comm.RemotePeer) (api.PeerIdentityType, error) {
	return nil, nil
}

// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
// Each message from the channel can be used to send a reply back to the sender
func (mock *commMock) Accept(accept common.MessageAcceptor) <-chan protoext.ReceivedMessage {
	ch := make(chan protoext.ReceivedMessage)
	mock.acceptors = append(mock.acceptors, &channelMock{accept, ch})
	return ch
}

// PresumedDead returns a read-only channel for node endpoints that are suspected to be offline
func (mock *commMock) PresumedDead() <-chan common.PKIidType {
	return mock.deadChannel
}

// CloseConn closes a connection to a certain endpoint
func (mock *commMock) CloseConn(peer *comm.RemotePeer) {
	// NOOP
}

// Stop stops the module
func (mock *commMock) Stop() {
	logger.Debug("Stopping communication module, closing all accepting channels.")
	for _, accept := range mock.acceptors {
		close(accept.channel)
	}
	logger.Debug("[XXX]: Sending done signal to close the module.")
	mock.done <- struct{}{}
}
