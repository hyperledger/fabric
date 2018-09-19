/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	localconfig "github.com/hyperledger/fabric/orderer/common/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

func TestBroadcastNoPanic(t *testing.T) {
	// Defer recovers from the panic
	_ = (&server{}).Broadcast(nil)
}

func TestDeliverNoPanic(t *testing.T) {
	// Defer recovers from the panic
	_ = (&server{}).Deliver(nil)
}

type recvr interface {
	Recv() (*cb.Envelope, error)
}

type mockSrv struct {
	grpc.ServerStream
	msg *cb.Envelope
	err error
}

func (mockSrv) Context() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{})
}

type mockBroadcastSrv mockSrv

func (mbs *mockBroadcastSrv) Recv() (*cb.Envelope, error) {
	return mbs.msg, mbs.err
}

func (mbs *mockBroadcastSrv) Send(br *ab.BroadcastResponse) error {
	panic("Unimplimented")
}

type mockDeliverSrv mockSrv

func (mds *mockDeliverSrv) CreateStatusReply(status cb.Status) proto.Message {
	return &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: status},
	}
}

func (mds *mockDeliverSrv) CreateBlockReply(block *cb.Block) proto.Message {
	return &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{Block: block},
	}
}

func (mds *mockDeliverSrv) Recv() (*cb.Envelope, error) {
	return mds.msg, mds.err
}

func (mds *mockDeliverSrv) Send(br *ab.DeliverResponse) error {
	panic("Unimplimented")
}

func testMsgTrace(handler func(dir string, msg *cb.Envelope) recvr, t *testing.T) {
	dir, err := ioutil.TempDir("", "TestMsgTrace")
	if err != nil {
		t.Fatalf("Could not create temp dir")
	}
	defer os.RemoveAll(dir)

	msg := &cb.Envelope{Payload: []byte("somedata")}

	r := handler(dir, msg)

	rMsg, err := r.Recv()
	assert.Equal(t, msg, rMsg)
	assert.Nil(t, err)

	var fileData []byte
	for i := 0; i < 100; i++ {
		// Writing the trace file is deliberately non-blocking, wait up to a second, checking every 10 ms to see if the file now exists.
		time.Sleep(10 * time.Millisecond)
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			assert.Nil(t, err)
			if path == dir {
				return nil
			}
			assert.Nil(t, fileData, "Should only be one file")
			fileData, err = ioutil.ReadFile(path)
			assert.Nil(t, err)
			return nil
		})
		if fileData != nil {
			break
		}
	}

	assert.Equal(t, utils.MarshalOrPanic(msg), fileData)
}

func TestBroadcastMsgTrace(t *testing.T) {
	testMsgTrace(func(dir string, msg *cb.Envelope) recvr {
		return &broadcastMsgTracer{
			AtomicBroadcast_BroadcastServer: &mockBroadcastSrv{
				msg: msg,
			},
			msgTracer: msgTracer{
				debug: &localconfig.Debug{
					BroadcastTraceDir: dir,
				},
				function: "Broadcast",
			},
		}
	}, t)
}

func TestDeliverMsgTrace(t *testing.T) {
	testMsgTrace(func(dir string, msg *cb.Envelope) recvr {
		return &deliverMsgTracer{
			Receiver: &mockDeliverSrv{
				msg: msg,
			},
			msgTracer: msgTracer{
				debug: &localconfig.Debug{
					DeliverTraceDir: dir,
				},
				function: "Deliver",
			},
		}
	}, t)
}
