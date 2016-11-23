/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package kafka

import (
	"testing"

	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"google.golang.org/grpc"
)

func mockNew(t *testing.T, conf *config.TopLevel, disk chan []byte) Orderer {
	return &serverImpl{
		broadcaster: mockNewBroadcaster(t, conf, oldestOffset, disk),
		deliverer:   mockNewDeliverer(t, conf),
	}
}

type mockBroadcastStream struct {
	grpc.ServerStream
	incoming chan *cb.Envelope
	outgoing chan *ab.BroadcastResponse
	t        *testing.T
	closed   bool // Set to true if the outgoing channel is closed
}

func newMockBroadcastStream(t *testing.T) *mockBroadcastStream {
	return &mockBroadcastStream{
		incoming: make(chan *cb.Envelope),
		outgoing: make(chan *ab.BroadcastResponse),
		t:        t,
	}
}

func (mbs *mockBroadcastStream) Recv() (*cb.Envelope, error) {
	return <-mbs.incoming, nil
}

func (mbs *mockBroadcastStream) Send(reply *ab.BroadcastResponse) error {
	if !mbs.closed {
		mbs.outgoing <- reply
	}
	return nil
}

func (mbs *mockBroadcastStream) CloseOut() bool {
	close(mbs.outgoing)
	mbs.closed = true
	return mbs.closed
}

type mockDeliverStream struct {
	grpc.ServerStream
	incoming chan *ab.DeliverUpdate
	outgoing chan *ab.DeliverResponse
	t        *testing.T
	closed   bool
}

func newMockDeliverStream(t *testing.T) *mockDeliverStream {
	return &mockDeliverStream{
		incoming: make(chan *ab.DeliverUpdate),
		outgoing: make(chan *ab.DeliverResponse),
		t:        t,
	}
}

func (mds *mockDeliverStream) Recv() (*ab.DeliverUpdate, error) {
	return <-mds.incoming, nil

}

func (mds *mockDeliverStream) Send(reply *ab.DeliverResponse) error {
	if !mds.closed {
		mds.outgoing <- reply
	}
	return nil
}

func (mds *mockDeliverStream) CloseOut() bool {
	close(mds.outgoing)
	mds.closed = true
	return mds.closed
}
