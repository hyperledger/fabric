/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"testing"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

func TestMockComm(t *testing.T) {
	first := &socketMock{"first", make(chan interface{})}
	second := &socketMock{"second", make(chan interface{})}
	members := make(map[string]*socketMock)

	members[first.endpoint] = first
	members[second.endpoint] = second

	comm1 := NewCommMock(first.endpoint, members)
	defer comm1.Stop()

	msgCh := comm1.Accept(func(message interface{}) bool {
		return message.(protoext.ReceivedMessage).GetGossipMessage().GetStateRequest() != nil ||
			message.(protoext.ReceivedMessage).GetGossipMessage().GetStateResponse() != nil
	})

	comm2 := NewCommMock(second.endpoint, members)
	defer comm2.Stop()

	sMsg, _ := protoext.NoopSign(&proto.GossipMessage{
		Content: &proto.GossipMessage_StateRequest{StateRequest: &proto.RemoteStateRequest{
			StartSeqNum: 1,
			EndSeqNum:   3,
		}},
	})
	comm2.Send(sMsg, &comm.RemotePeer{Endpoint: "first", PKIID: common.PKIidType("first")})

	msg := <-msgCh

	require.NotNil(t, msg.GetGossipMessage().GetStateRequest())
	require.Equal(t, "first", string(comm1.GetPKIid()))
}

func TestMockComm_PingPong(t *testing.T) {
	members := make(map[string]*socketMock)

	members["peerA"] = &socketMock{"peerA", make(chan interface{})}
	members["peerB"] = &socketMock{"peerB", make(chan interface{})}

	peerA := NewCommMock("peerA", members)
	peerB := NewCommMock("peerB", members)

	all := func(interface{}) bool {
		return true
	}

	rcvChA := peerA.Accept(all)
	rcvChB := peerB.Accept(all)

	sMsg, _ := protoext.NoopSign(&proto.GossipMessage{
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					SeqNum: 1,
					Data:   []byte("Ping"),
				},
			},
		},
	})
	peerA.Send(sMsg, &comm.RemotePeer{Endpoint: "peerB", PKIID: common.PKIidType("peerB")})

	msg := <-rcvChB
	dataMsg := msg.GetGossipMessage().GetDataMsg()
	data := string(dataMsg.Payload.Data)
	require.Equal(t, "Ping", data)

	msg.Respond(&proto.GossipMessage{
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					SeqNum: 1,
					Data:   []byte("Pong"),
				},
			},
		},
	})

	msg = <-rcvChA
	dataMsg = msg.GetGossipMessage().GetDataMsg()
	data = string(dataMsg.Payload.Data)
	require.Equal(t, "Pong", data)
}
