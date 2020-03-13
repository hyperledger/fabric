/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/assert"
)

func TestSend(t *testing.T) {
	ch := make(chan *pb.ChaincodeMessage)

	stream := newInProcStream(ch, ch)

	//good send (non-blocking send and receive)
	msg := &pb.ChaincodeMessage{}
	go stream.Send(msg)
	msg2, _ := stream.Recv()
	assert.Equal(t, msg, msg2, "send != recv")

	//close the channel
	close(ch)

	//bad send, should panic, unblock and return error
	err := stream.Send(msg)
	assert.NotNil(t, err, "should have errored on panic")
}

func TestRecvChannelClosedError(t *testing.T) {
	ch := make(chan *pb.ChaincodeMessage)

	stream := newInProcStream(ch, ch)

	// Close the channel
	close(ch)

	// Trying to call a closed receive channel should return an error
	_, err := stream.Recv()
	if assert.Error(t, err, "Should return an error") {
		assert.Contains(t, err.Error(), "channel is closed")
	}
}

func TestCloseSend(t *testing.T) {
	send := make(chan *pb.ChaincodeMessage)
	recv := make(chan *pb.ChaincodeMessage)

	stream := newInProcStream(recv, send)
	stream.CloseSend()

	_, ok := <-send
	assert.False(t, ok, "send channel should be closed")
	assert.NotPanics(t, func() { stream.CloseSend() }, "CloseSend should be idempotent")
}
