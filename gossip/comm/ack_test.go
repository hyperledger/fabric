/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func TestInterceptAcks(t *testing.T) {
	pubsub := util.NewPubSub()
	pkiID := common.PKIidType("pkiID")
	msgs := make(chan *proto.SignedGossipMessage, 1)
	handlerFunc := func(message *proto.SignedGossipMessage) {
		msgs <- message
	}
	wrappedHandler := interceptAcks(handlerFunc, pkiID, pubsub)
	ack := &proto.SignedGossipMessage{
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
	assert.Len(t, msgs, 0)
	_, err := sub.Listen()
	// Ensure ack was published
	assert.NoError(t, err)

	// Test none acks are just forwarded
	notAck := &proto.SignedGossipMessage{
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
	assert.Len(t, msgs, 1)
	_, err = sub.Listen()
	// Ensure ack was not published
	assert.Error(t, err)
}

func TestAck(t *testing.T) {
	t.Parallel()

	comm1, _ := newCommInstance(14000, naiveSec)
	comm2, _ := newCommInstance(14001, naiveSec)
	defer comm2.Stop()
	comm3, _ := newCommInstance(14002, naiveSec)
	defer comm3.Stop()
	comm4, _ := newCommInstance(14003, naiveSec)
	defer comm4.Stop()

	acceptData := func(o interface{}) bool {
		return o.(proto.ReceivedMessage).GetGossipMessage().IsDataMsg()
	}

	ack := func(c <-chan proto.ReceivedMessage) {
		msg := <-c
		msg.Ack(nil)
	}

	nack := func(c <-chan proto.ReceivedMessage) {
		msg := <-c
		msg.Ack(errors.New("Failed processing message because reasons"))
	}

	// Have instances 2 and 3 subscribe to data messages, and ack them
	inc2 := comm2.Accept(acceptData)
	inc3 := comm3.Accept(acceptData)

	// Collect 2 out of 2 acks - should succeed
	go ack(inc2)
	go ack(inc3)
	res := comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(14001), remotePeer(14002))
	assert.Len(t, res, 2)
	assert.Empty(t, res[0].Error())
	assert.Empty(t, res[1].Error())

	// Collect 2 out of 3 acks - should succeed
	t1 := time.Now()
	go ack(inc2)
	go ack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*10, 2, remotePeer(14001), remotePeer(14002), remotePeer(14003))
	elapsed := time.Since(t1)
	assert.Len(t, res, 2)
	assert.Empty(t, res[0].Error())
	assert.Empty(t, res[1].Error())
	// Collection of 2 out of 3 acks should have taken much less than the timeout (10 seconds)
	assert.True(t, elapsed < time.Second*5)

	// Collect 2 out of 3 acks - should fail, because peer3 now have sent an error along with the ack
	go ack(inc2)
	go nack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*10, 2, remotePeer(14001), remotePeer(14002), remotePeer(14003))
	assert.Len(t, res, 3)
	assert.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "Failed processing message because reasons")
	assert.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "timed out")

	// Collect 2 out of 2 acks - should fail because comm2 and comm3 now don't acknowledge messages
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(14001), remotePeer(14002))
	assert.Len(t, res, 2)
	assert.Contains(t, res[0].Error(), "timed out")
	assert.Contains(t, res[1].Error(), "timed out")
	// Drain ack messages to prepare for next salvo
	<-inc2
	<-inc3

	// Collect 2 out of 3 acks - should fail
	go ack(inc2)
	go nack(inc3)
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 2, remotePeer(14001), remotePeer(14002), remotePeer(14003))
	assert.Len(t, res, 3)
	assert.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "") // This is the "successful ack"
	assert.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "Failed processing message because reasons")
	assert.Contains(t, []string{res[0].Error(), res[1].Error(), res[2].Error()}, "timed out")
	assert.Contains(t, res.String(), "\"Failed processing message because reasons\":1")
	assert.Contains(t, res.String(), "\"timed out\":1")
	assert.Contains(t, res.String(), "\"successes\":1")
	assert.Equal(t, 2, res.NackCount())
	assert.Equal(t, 1, res.AckCount())

	// Send a message to no one
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 1)
	assert.Len(t, res, 0)

	// Send a message while stopping
	comm1.Stop()
	res = comm1.SendWithAck(createGossipMsg(), time.Second*3, 1, remotePeer(14001), remotePeer(14002), remotePeer(14003))
	assert.Len(t, res, 3)
	assert.Contains(t, res[0].Error(), "comm is stopping")
	assert.Contains(t, res[1].Error(), "comm is stopping")
	assert.Contains(t, res[2].Error(), "comm is stopping")
}
