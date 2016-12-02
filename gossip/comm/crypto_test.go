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

package comm

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type gossipTestServer struct {
	lock      sync.Mutex
	msgChan   chan uint64
	tlsUnique []byte
}

func (s *gossipTestServer) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	s.lock.Lock()
	s.tlsUnique = ExtractTLSUnique(stream.Context())
	s.lock.Unlock()
	m, err := stream.Recv()
	if err != nil {
		fmt.Println(err)
	} else {
		s.msgChan <- m.Nonce
	}

	return nil
}

func (s *gossipTestServer) getTLSUnique() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.tlsUnique
}

func (s *gossipTestServer) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func TestCertificateGeneration(t *testing.T) {
	err := generateCertificates("key.pem", "cert.pem")
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	defer os.Remove("cert.pem")
	defer os.Remove("key.pem")
	var ll net.Listener
	creds, err := credentials.NewServerTLSFromFile("cert.pem", "key.pem")
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	s := grpc.NewServer(grpc.Creds(creds))
	ll, err = net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5511))
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	srv := &gossipTestServer{msgChan: make(chan uint64)}
	proto.RegisterGossipServer(s, srv)
	go s.Serve(ll)
	defer func() {
		s.Stop()
		ll.Close()
	}()
	time.Sleep(time.Second * time.Duration(2))
	ta := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	conn, err := grpc.Dial("localhost:5511", grpc.WithTransportCredentials(&authCreds{tlsCreds: ta}), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}

	time.Sleep(time.Duration(1) * time.Second)

	clientTLSUnique := ExtractTLSUnique(stream.Context())
	serverTLSUnique := srv.getTLSUnique()

	assert.NotNil(t, clientTLSUnique)
	assert.NotNil(t, serverTLSUnique)

	assert.True(t, bytes.Equal(clientTLSUnique, serverTLSUnique), "Client and server TLSUnique are not equal")

	msg := createGossipMsg()
	stream.Send(msg)
	select {
	case nonce := <-srv.msgChan:
		assert.Equal(t, msg.Nonce, nonce)
		break
	case <-time.NewTicker(time.Second).C:
		assert.Fail(t, "Timed out reading from stream")
	}
}
