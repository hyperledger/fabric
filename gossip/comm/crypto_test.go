/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
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

func init() {
	util.SetupTestLogging()
}

func createTestServer(t *testing.T, cert *tls.Certificate) (srv *gossipTestServer, ll net.Listener) {
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{*cert},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
	ll, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "%v", err)

	srv = &gossipTestServer{s: s, ll: ll, selfCertHash: certHashFromRawCert(cert.Certificate[0])}
	proto.RegisterGossipServer(s, srv)
	go s.Serve(ll)
	return srv, ll
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
	cert := GenerateCertificatesOrPanic()
	srv, ll := createTestServer(t, &cert)
	defer srv.stop()

	clientCert := GenerateCertificatesOrPanic()
	clientCertHash := certHashFromRawCert(clientCert.Certificate[0])
	ta := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, ll.Addr().String(), grpc.WithTransportCredentials(ta), grpc.WithBlock())
	require.NoError(t, err, "%v", err)

	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	require.NoError(t, err, "%v", err)
	if err != nil {
		return
	}

	time.Sleep(time.Second)
	clientSideCertHash := extractCertificateHashFromContext(stream.Context())
	serverSideCertHash := srv.getClientCertHash()

	require.NotNil(t, clientSideCertHash)
	require.NotNil(t, serverSideCertHash)

	require.Equal(t, 32, len(clientSideCertHash), "client side cert hash is %v", clientSideCertHash)
	require.Equal(t, 32, len(serverSideCertHash), "server side cert hash is %v", serverSideCertHash)

	require.Equal(t, clientSideCertHash, srv.selfCertHash, "Server self hash isn't equal to client side hash")
	require.Equal(t, clientCertHash, srv.remoteCertHash, "Server side and client hash aren't equal")
}

// GenerateCertificatesOrPanic generates a random pair of public and private keys
// and return TLS certificate.
func GenerateCertificatesOrPanic() tls.Certificate {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		SerialNumber: sn,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	rawBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		panic(err)
	}
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		panic(err)
	}
	encodedCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rawBytes})
	encodedKey := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	cert, err := tls.X509KeyPair(encodedCert, encodedKey)
	if err != nil {
		panic(err)
	}
	return cert
}
