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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type gossipTestServer struct {
	lock           sync.Mutex
	remoteCertHash []byte
	selfCertHash   []byte
	ll             net.Listener
	s              *grpc.Server
}

func createTestServer(t *testing.T, cert *tls.Certificate) *gossipTestServer {
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{*cert},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
	ll, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5611))
	assert.NoError(t, err, "%v", err)

	srv := &gossipTestServer{s: s, ll: ll, selfCertHash: certHashFromRawCert(cert.Certificate[0])}
	proto.RegisterGossipServer(s, srv)
	go s.Serve(ll)
	return srv
}

func (s *gossipTestServer) stop() {
	s.s.Stop()
	s.ll.Close()
}

func (s *gossipTestServer) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.remoteCertHash = extractCertificateHashFromContext(stream.Context())
	return nil
}

func (s *gossipTestServer) getClientCertHash() []byte {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.remoteCertHash
}

func (s *gossipTestServer) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func TestCertificateExtraction(t *testing.T) {
	err := generateCertificates("key.pem", "cert.pem")
	defer os.Remove("cert.pem")
	defer os.Remove("key.pem")
	assert.NoError(t, err, "%v", err)
	serverCert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	assert.NoError(t, err, "%v", err)

	srv := createTestServer(t, &serverCert)
	defer srv.stop()

	generateCertificates("key2.pem", "cert2.pem")
	defer os.Remove("cert2.pem")
	defer os.Remove("key2.pem")
	clientCert, err := tls.LoadX509KeyPair("cert2.pem", "key2.pem")
	clientCertHash := certHashFromRawCert(clientCert.Certificate[0])
	assert.NoError(t, err)
	ta := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true,
	})
	assert.NoError(t, err, "%v", err)
	conn, err := grpc.Dial("localhost:5611", grpc.WithTransportCredentials(&authCreds{tlsCreds: ta}), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	assert.NoError(t, err, "%v", err)

	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}

	time.Sleep(time.Second)
	clientSideCertHash := extractCertificateHashFromContext(stream.Context())
	serverSideCertHash := srv.getClientCertHash()

	assert.NotNil(t, clientSideCertHash)
	assert.NotNil(t, serverSideCertHash)

	assert.Equal(t, 32, len(clientSideCertHash), "client side cert hash is %v", clientSideCertHash)
	assert.Equal(t, 32, len(serverSideCertHash), "server side cert hash is %v", serverSideCertHash)

	assert.Equal(t, clientSideCertHash, srv.selfCertHash, "Server self hash isn't equal to client side hash")
	assert.Equal(t, clientCertHash, srv.remoteCertHash, "Server side and client hash aren't equal")
}
