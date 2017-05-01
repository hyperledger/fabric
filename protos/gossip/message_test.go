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

package gossip

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

type protoMsg interface {
	Reset()
	String() string
	ProtoMessage()
	Descriptor() ([]byte, []int)
}

func TestMethods(t *testing.T) {
	msgs := []protoMsg{
		&Envelope{},
		&SecretEnvelope{},
		&GossipMessage{},
		&Secret{},
		&StateInfo{},
		&ConnEstablish{},
		&AliveMessage{},
		&MembershipRequest{},
		&MembershipResponse{},
		&DataMessage{},
		&GossipHello{},
		&DataDigest{},
		&DataRequest{},
		&DataUpdate{},
		&Empty{},
		&StateInfoSnapshot{},
		&StateInfoPullRequest{},
		&RemoteStateRequest{},
		&RemoteStateResponse{},
		&LeadershipMessage{},
		&PeerIdentity{},
	}

	for _, msg := range msgs {
		msg.Reset()
		_, _ = msg.Descriptor()
		msg.ProtoMessage()
		assert.Empty(t, msg.String())

	}

	contentTypes := []isGossipMessage_Content{
		&GossipMessage_AliveMsg{},
		&GossipMessage_MemReq{},
		&GossipMessage_MemRes{},
		&GossipMessage_DataMsg{},
		&GossipMessage_Hello{},
		&GossipMessage_DataDig{},
		&GossipMessage_DataReq{},
		&GossipMessage_DataUpdate{},
		&GossipMessage_Empty{},
		&GossipMessage_Conn{},
		&GossipMessage_StateInfo{},
		&GossipMessage_StateSnapshot{},
		&GossipMessage_StateInfoPullReq{},
		&GossipMessage_StateRequest{},
		&GossipMessage_StateResponse{},
		&GossipMessage_LeadershipMsg{},
		&GossipMessage_PeerIdentity{},
	}

	for _, ct := range contentTypes {
		ct.isGossipMessage_Content()
		gMsg := &GossipMessage{
			Content: ct,
		}
		v := reflect.ValueOf(gMsg)
		for i := 0; i < v.NumMethod(); i++ {
			func() {
				defer func() {
					recover()
				}()
				v.Method(i).Call([]reflect.Value{})
			}()
		}
		gMsg = &GossipMessage{
			Content: ct,
		}
		_GossipMessage_OneofSizer(gMsg)
		gMsg = &GossipMessage{
			Content: ct,
		}
		_GossipMessage_OneofMarshaler(gMsg, &proto.Buffer{})
		gMsg = &GossipMessage{
			Content: ct,
		}

		for i := 5; i < 22; i++ {
			_GossipMessage_OneofUnmarshaler(gMsg, i, 2, &proto.Buffer{})
		}
	}

	assert.NotZero(t, _Secret_OneofSizer(&Secret{
		Content: &Secret_InternalEndpoint{
			InternalEndpoint: "internalEndpoint",
		},
	}))

	assert.Nil(t, (&Envelope{}).GetSecretEnvelope())
}

func TestGrpc(t *testing.T) {
	cl := NewGossipClient(nil)
	f1 := func() {
		cl.GossipStream(nil)
	}
	assert.Panics(t, f1)
	f2 := func() {
		cl.Ping(nil, nil)
	}
	assert.Panics(t, f2)
	gscl := &gossipGossipStreamClient{}
	f3 := func() {
		gscl.Send(nil)
	}
	assert.Panics(t, f3)
	f4 := func() {
		gscl.Recv()
	}
	assert.Panics(t, f4)
	f5 := func() {
		gscl.Header()
	}
	assert.Panics(t, f5)
	f6 := func() {
		gscl.CloseSend()
	}
	assert.Panics(t, f6)
	f7 := func() {
		gscl.Context()
	}
	assert.Panics(t, f7)
	gss := &gossipGossipStreamServer{}
	f8 := func() {
		gss.Recv()
	}
	assert.Panics(t, f8)
	f9 := func() {
		gss.Send(nil)
	}
	assert.Panics(t, f9)
	f10 := func() {
		gss.Context()
	}
	assert.Panics(t, f10)
	f11 := func() {
		gss.RecvMsg(nil)
	}
	assert.Panics(t, f11)
	f12 := func() {
		gss.SendHeader(nil)
	}
	assert.Panics(t, f12)
	f13 := func() {
		gss.RecvMsg(nil)
	}
	assert.Panics(t, f13)
	f14 := func() {
		gss.SendMsg(nil)
	}
	assert.Panics(t, f14)
	f15 := func() {
		gss.SetTrailer(nil)
	}
	assert.Panics(t, f15)
	f16 := func() {
		_Gossip_Ping_Handler(nil, nil, func(interface{}) error {
			return nil
		}, nil)
	}
	assert.Panics(t, f16)
}
