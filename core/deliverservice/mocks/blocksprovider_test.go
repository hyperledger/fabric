/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mocks

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestMockBlocksDeliverer(t *testing.T) {
	// Make sure it implements BlocksDeliverer
	var bd blocksprovider.BlocksDeliverer
	bd = &MockBlocksDeliverer{}
	_ = bd

	assert.Panics(t, func() {
		bd.Recv()
	})
	bd.(*MockBlocksDeliverer).MockRecv = func(mock *MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Status{
				Status: common.Status_FORBIDDEN,
			},
		}, nil
	}
	status, err := bd.Recv()
	assert.Nil(t, err)
	assert.Equal(t, common.Status_FORBIDDEN, status.GetStatus())
	bd.(*MockBlocksDeliverer).MockRecv = MockRecv
	block, err := bd.Recv()
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), block.GetBlock().Header.Number)

	bd.(*MockBlocksDeliverer).Close()

	seekInfo := &orderer.SeekInfo{
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Oldest{Oldest: &orderer.SeekOldest{}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}
	si, err := pb.Marshal(seekInfo)
	assert.NoError(t, err)

	payload := &common.Payload{}

	payload.Data = si
	b, err := pb.Marshal(payload)
	assert.NoError(t, err)
	assert.Nil(t, bd.Send(&common.Envelope{Payload: b}))
}

func TestMockGossipServiceAdapter(t *testing.T) {
	// Make sure it implements GossipServiceAdapter
	var gsa blocksprovider.GossipServiceAdapter
	seqNums := make(chan uint64, 1)
	gsa = &MockGossipServiceAdapter{GossipBlockDisseminations: seqNums}
	_ = gsa

	// Test gossip
	msg := &proto.GossipMessage{
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					SeqNum: uint64(100),
				},
			},
		},
	}
	gsa.Gossip(msg)
	select {
	case seq := <-seqNums:
		assert.Equal(t, uint64(100), seq)
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't gossip within a timely manner")
	}

	// Test AddPayload
	gsa.AddPayload("TEST", msg.GetDataMsg().Payload)
	assert.Equal(t, int32(1), atomic.LoadInt32(&(gsa.(*MockGossipServiceAdapter).AddPayloadsCnt)))

	// Test PeersOfChannel
	assert.Len(t, gsa.PeersOfChannel(nil), 0)
}

func TestMockAtomicBroadcastClient(t *testing.T) {
	// Make sure it implements MockAtomicBroadcastClient
	var abc orderer.AtomicBroadcastClient
	abc = &MockAtomicBroadcastClient{BD: &MockBlocksDeliverer{}}

	assert.Panics(t, func() {
		abc.Broadcast(context.Background())
	})
	c, err := abc.Deliver(context.Background())
	assert.Nil(t, err)
	assert.NotNil(t, c)
}

func TestMockLedgerInfo(t *testing.T) {
	var li blocksprovider.LedgerInfo
	li = &MockLedgerInfo{uint64(8)}
	_ = li

	height, err := li.LedgerHeight()
	assert.Equal(t, uint64(8), height)
	assert.NoError(t, err)
}
