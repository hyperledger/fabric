/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package internal

import (
	"errors"
	"net"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type testServer struct {
	receivedMessages chan<- *pb.ChaincodeMessage
	sendMessages     <-chan *pb.ChaincodeMessage
	waitTime         time.Duration
}

func (t *testServer) Register(registerServer pb.ChaincodeSupport_RegisterServer) error {
	for {
		recv, err := registerServer.Recv()
		if err != nil {
			return err
		}

		select {
		case t.receivedMessages <- recv:
		case <-time.After(t.waitTime):
			return errors.New("failed to capture received message")
		}

		select {
		case msg, ok := <-t.sendMessages:
			if !ok {
				return nil
			}
			if err := registerServer.Send(msg); err != nil {
				return err
			}
		case <-time.After(t.waitTime):
			return errors.New("no messages available on send channel")
		}
	}
}

func TestMessageSizes(t *testing.T) {
	const waitTime = 10 * time.Second

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err, "listen failed")
	defer lis.Close()

	sendMessages := make(chan *pb.ChaincodeMessage, 1)
	receivedMessages := make(chan *pb.ChaincodeMessage, 1)
	testServer := &testServer{
		receivedMessages: receivedMessages,
		sendMessages:     sendMessages,
		waitTime:         waitTime,
	}

	server := grpc.NewServer(
		grpc.MaxSendMsgSize(2*maxSendMessageSize),
		grpc.MaxRecvMsgSize(2*maxRecvMessageSize),
	)
	pb.RegisterChaincodeSupportServer(server, testServer)

	serveCompleteCh := make(chan error, 1)
	go func() { serveCompleteCh <- server.Serve(lis) }()

	client, err := NewClientConn(lis.Addr().String(), nil, keepalive.ClientParameters{})
	assert.NoError(t, err, "failed to create client connection")

	regClient, err := NewRegisterClient(client)
	assert.NoError(t, err, "failed to create register client")

	t.Run("acceptable messaages", func(t *testing.T) {
		acceptableMessage := &pb.ChaincodeMessage{
			Payload: make([]byte, maxSendMessageSize-100),
		}
		sendMessages <- acceptableMessage
		err = regClient.Send(acceptableMessage)
		assert.NoError(t, err, "sending messge below size threshold failed")

		select {
		case m := <-receivedMessages:
			assert.Len(t, m.Payload, maxSendMessageSize-100)
		case <-time.After(waitTime):
			t.Fatalf("acceptable message was not received by server")
		}

		msg, err := regClient.Recv()
		assert.NoError(t, err, "failed to receive message")
		assert.Len(t, msg.Payload, maxSendMessageSize-100)
	})

	t.Run("response message is too large", func(t *testing.T) {
		sendMessages <- &pb.ChaincodeMessage{
			Payload: make([]byte, maxSendMessageSize+1),
		}
		err = regClient.Send(&pb.ChaincodeMessage{})
		assert.NoError(t, err, "sending messge below size threshold should succeed")

		select {
		case m := <-receivedMessages:
			assert.Len(t, m.Payload, 0)
		case <-time.After(waitTime):
			t.Fatalf("acceptable message was not received by server")
		}

		_, err := regClient.Recv()
		assert.Error(t, err, "receiving a message that is too large should fail")
	})

	t.Run("sent message is too large", func(t *testing.T) {
		tooBig := &pb.ChaincodeMessage{
			Payload: make([]byte, maxSendMessageSize+1),
		}
		err = regClient.Send(tooBig)
		assert.Error(t, err, "sending messge above size threshold should fail")
	})

	err = lis.Close()
	assert.NoError(t, err, "close failed")
	select {
	case <-serveCompleteCh:
	case <-time.After(waitTime):
		t.Fatal("server shutdown timeout")
	}
}
