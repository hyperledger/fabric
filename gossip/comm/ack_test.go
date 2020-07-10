/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"errors"
	"testing"
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
)

func TestInterceptAcks(t *testing.T) {
	pubsub := util.NewPubSub()
	pkiID := common.PKIidType("pkiID")
	msgs := make(chan *protoext.SignedGossipMessage, 1)
	handlerFunc := func(message *protoext.SignedGossipMessage) {
		msgs <- message
	}
	wrappedHandler := interceptAcks(handlerFunc, pkiID, pubsub)
	ack := &protoext.SignedGossipMessage{
		GossipMessage: &proto.GossipMessage{
			Nonce: 1,
			Content: &proto.GossipMessage_Ack{
				Ack: &proto.Acknowledgement{},
			},
		},
	}
	sub := pubsub.Subscribe(topicForAck(1, pkiID), time.Second)
	wrappedHandler(ack)
	// Ensure ack was consumed and not passed onwards to the wrapped hander
	require.Len(t, msgs, 0)
	_, err := sub.Listen()
	// Ensure ack was published
	require.NoError(t, err)

	// Test none acks are just forwarded
	notAck := &protoext.SignedGossipMessage{
		GossipMessage: &proto.GossipMessage{
			Nonce: 2,
			Content: &proto.GossipMessage_DataMsg{
				DataMsg: &proto.DataMessage{},
			},
		},
	}
	sub = pubsub.Subscribe(topicForAck(2, pkiID), time.Second)
	wrappedHandler(notAck)
	// Ensure message was passed to the wrapped handler
	require.Len(t, msgs, 1)
	_, err = sub.Listen()
	// Ensure ack was not published
	require.Error(t, err)
}

func TestAck(t *testing.T) {
	comm1, _ := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)
	defer comm2.Stop()
	comm3, port3 := newCommInstance(t, naiveSec)
	defer comm3.Stop()
	comm4, port4 := newCommInstance(t, naiveSec)
	defer comm4.Stop()

	acceptData := func(o interface{}) bool {
		m := o.(protoext.ReceivedMessage).GetGossipMessage()
		return protoext.IsDataMsg(m.GossipMessage)
	}

	ack := func(c <-chan protoext.ReceivedMessage) {
		msg := <-c
		msg.Ack(nil)
	}

	nack := func(c <-chan protoext.ReceivedMessage) {
		msg := <-c
		msg.Ack(errors.New("Failed processing message because reasons"))
	}

	// Have instances 2 and 3 subscribe to data messages, and ack them
	inc2 := comm2.Accept(acceptData)
	inc3 := comm3.Accept(acceptData)

	// Collect 2 out of 2 acks - should succeed
	go ack(inc2)
	go ack(inc3)
	res := comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(port2), remotePeer(port3))
	require.Len(t, res, 2)
	require.Empty(t, res[0].Error())
	require.Empty(t, res[1].Error())

	// Collect 2 out of 3 acks - should succeed
	t1 := time.Now()
	go ack(inc2)
	go ack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*10, 2, remotePeer(port2), remotePeer(port3), remotePeer(port4))
	elapsed := time.Since(t1)
	require.Len(t, res, 2)
	require.Empty(t, res[0].Error())
	require.Empty(t, res[1].Error())
	// Collection of 2 out of 3 acks should have taken much less than the timeout (10 seconds)
	require.True(t, elapsed < time.Second*5)

	// Collect 2 out of 3 acks - should fail, because peer3 now have sent an error along with the ack
	go ack(inc2)
	go nack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*10, 2, remotePeer(port2), remotePeer(port3), remotePeer(port4))
	require.Len(t, res, 3)
	require.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "Failed processing message because reasons")
	require.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "timed out")

	// Collect 2 out of 2 acks - should fail because comm2 and comm3 now don't acknowledge messages
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(port2), remotePeer(port3))
	require.Len(t, res, 2)
	require.Contains(t, res[0].Error(), "timed out")
	require.Contains(t, res[1].Error(), "timed out")
	// Drain ack messages to prepare for next salvo
	<-inc2
	<-inc3

	// Collect 2 out of 3 acks - should fail
	go ack(inc2)
	go nack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(port2), remotePeer(port3), remotePeer(port4))
	require.Len(t, res, 3)
	require.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "") // This is the "successful ack"
	require.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "Failed processing message because reasons")
	require.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "timed out")
	require.Contains(t, res.String(), "\"Failed processing message because reasons\":1")
	require.Contains(t, res.String(), "\"timed out\":1")
	require.Contains(t, res.String(), "\"successes\":1")
	require.Equal(t, 2, res.NackCount())
	require.Equal(t, 1, res.AckCount())

	// Send a message to no one
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 1)
	require.Len(t, res, 0)

	// Send a message while stopping
	comm1.Stop()
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 1, remotePeer(port2), remotePeer(port3), remotePeer(port4))
	require.Len(t, res, 3)
	require.Contains(t, res[0].Error(), "comm is stopping")
	require.Contains(t, res[1].Error(), "comm is stopping")
	require.Contains(t, res[2].Error(), "comm is stopping")
}
