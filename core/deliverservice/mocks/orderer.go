/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type Orderer struct {
	net.Listener
	*grpc.Server
	nextExpectedSeek uint64
	t                *testing.T
	blockChannel     chan uint64
	stopChan         chan struct{}
	failFlag         int32
	connCount        uint32
}

func NewOrderer(port int, t *testing.T) *Orderer {
	srv := grpc.NewServer()
	lsnr, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(err)
	}
	o := &Orderer{Server: srv,
		Listener:         lsnr,
		t:                t,
		nextExpectedSeek: uint64(1),
		blockChannel:     make(chan uint64, 1),
		stopChan:         make(chan struct{}, 1),
	}
	orderer.RegisterAtomicBroadcastServer(srv, o)
	go srv.Serve(lsnr)
	return o
}

func (o *Orderer) Shutdown() {
	o.stopChan <- struct{}{}
	o.Server.Stop()
	o.Listener.Close()
}

func (o *Orderer) Fail() {
	atomic.StoreInt32(&o.failFlag, int32(1))
	o.blockChannel <- 0
}

func (o *Orderer) Resurrect() {
	atomic.StoreInt32(&o.failFlag, int32(0))
	for {
		select {
		case <-o.blockChannel:
			continue
		default:
			return
		}
	}
}

func (o *Orderer) ConnCount() int {
	return int(atomic.LoadUint32(&o.connCount))
}

func (o *Orderer) hasFailed() bool {
	return atomic.LoadInt32(&o.failFlag) == int32(1)
}

func (*Orderer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("Should not have ben called")
}

func (o *Orderer) SetNextExpectedSeek(seq uint64) {
	atomic.StoreUint64(&o.nextExpectedSeek, uint64(seq))
}

func (o *Orderer) SendBlock(seq uint64) {
	o.blockChannel <- seq
}

func (o *Orderer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	atomic.AddUint32(&o.connCount, 1)
	defer atomic.AddUint32(&o.connCount, ^uint32(0))
	envlp, err := stream.Recv()
	if err != nil {
		return nil
	}
	if o.hasFailed() {
		return stream.Send(statusUnavailable())
	}
	payload := &common.Payload{}
	proto.Unmarshal(envlp.Payload, payload)
	seekInfo := &orderer.SeekInfo{}
	proto.Unmarshal(payload.Data, seekInfo)
	assert.True(o.t, seekInfo.Behavior == orderer.SeekInfo_BLOCK_UNTIL_READY)
	assert.Equal(o.t, atomic.LoadUint64(&o.nextExpectedSeek), seekInfo.Start.GetSpecified().Number)

	for {
		select {
		case <-o.stopChan:
			return nil
		case seq := <-o.blockChannel:
			if o.hasFailed() {
				return stream.Send(statusUnavailable())
			}
			o.sendBlock(stream, seq)
		}
	}

}

func statusUnavailable() *orderer.DeliverResponse {
	return &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Status{
			Status: common.Status_SERVICE_UNAVAILABLE,
		},
	}
}

func (o *Orderer) sendBlock(stream orderer.AtomicBroadcast_DeliverServer, seq uint64) {
	block := &common.Block{
		Header: &common.BlockHeader{
			Number: seq,
		},
	}
	stream.Send(&orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: block},
	})
}
