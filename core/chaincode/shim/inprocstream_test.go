/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package shim

import (
	"testing"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

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
