/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/comm/testpb"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestExtractCertificateHashFromContext(t *testing.T) {
	t.Parallel()
	require.Nil(t, comm.ExtractCertificateHashFromContext(context.Background()))

	p := &peer.Peer{}
	ctx := peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = &nonTLSConnection{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Raw: []byte{1, 2, 3}},
			},
		},
	}
	ctx = peer.NewContext(context.Background(), p)
	h := sha256.New()
	h.Write([]byte{1, 2, 3})
	require.Equal(t, h.Sum(nil), comm.ExtractCertificateHashFromContext(ctx))
}

type nonTLSConnection struct{}

func (*nonTLSConnection) AuthType() string {
	return ""
}

func TestBindingInspectorBadInit(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() {
		comm.NewBindingInspector(false, nil)
	})
}

func TestNoopBindingInspector(t *testing.T) {
	t.Parallel()
	extract := func(msg proto.Message) []byte {
		return nil
	}
	require.Nil(t, comm.NewBindingInspector(false, extract)(context.Background(), &common.Envelope{}))
	err := comm.NewBindingInspector(false, extract)(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, "message is nil", err.Error())
}

func TestBindingInspector(t *testing.T) {
	t.Parallel()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener for test server: %v", err)
	}

	extract := func(msg proto.Message) []byte {
		env, isEnvelope := msg.(*common.Envelope)
		if !isEnvelope || env == nil {
			return nil
		}
		ch, err := protoutil.ChannelHeader(env)
		if err != nil {
			return nil
		}
		return ch.TlsCertHash
	}
	srv := newInspectingServer(lis, comm.NewBindingInspector(true, extract))
	go srv.Start()
	defer srv.Stop()
	time.Sleep(time.Second)

	// Scenario I: Invalid header sent
	err = srv.newInspection(t).inspectBinding(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario II: invalid channel header
	ch, _ := proto.Marshal(protoutil.MakeChannelHeader(common.HeaderType_CONFIG, 0, "test", 0))
	// Corrupt channel header
	ch = append(ch, 0)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	require.Error(t, err)
	require.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario III: No TLS cert hash in envelope
	chanHdr := protoutil.MakeChannelHeader(common.HeaderType_CONFIG, 0, "test", 0)
	ch, _ = proto.Marshal(chanHdr)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	require.Error(t, err)
	require.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario IV: Client sends its TLS cert hash as needed, but doesn't use mutual TLS
	cert, _ := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	h := sha256.New()
	h.Write([]byte(cert.Certificate[0]))
	chanHdr.TlsCertHash = h.Sum(nil)
	ch, _ = proto.Marshal(chanHdr)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	require.Error(t, err)
	require.Contains(t, err.Error(), "client didn't send a TLS certificate")

	// Scenario V: Client uses mutual TLS but sends the wrong TLS cert hash
	chanHdr.TlsCertHash = []byte{1, 2, 3}
	chHdrWithWrongTLSCertHash, _ := proto.Marshal(chanHdr)
	err = srv.newInspection(t).withMutualTLS().inspectBinding(envelopeWithChannelHeader(chHdrWithWrongTLSCertHash))
	require.Error(t, err)
	require.Contains(t, err.Error(), "claimed TLS cert hash is [1 2 3] but actual TLS cert hash is")

	// Scenario VI: Client uses mutual TLS and also sends the correct TLS cert hash
	err = srv.newInspection(t).withMutualTLS().inspectBinding(envelopeWithChannelHeader(ch))
	require.NoError(t, err)
}

func TestGetLocalIP(t *testing.T) {
	ip, err := comm.GetLocalIP()
	require.NoError(t, err)
	t.Log(ip)
}

type inspectingServer struct {
	addr string
	*comm.GRPCServer
	lastContext atomic.Value
	inspector   comm.BindingInspector
}

func (is *inspectingServer) EmptyCall(ctx context.Context, _ *testpb.Empty) (*testpb.Empty, error) {
	is.lastContext.Store(ctx)
	return &testpb.Empty{}, nil
}

func (is *inspectingServer) inspect(envelope *common.Envelope) error {
	return is.inspector(is.lastContext.Load().(context.Context), envelope)
}

func newInspectingServer(listener net.Listener, inspector comm.BindingInspector) *inspectingServer {
	srv, err := comm.NewGRPCServerFromListener(listener, comm.ServerConfig{
		ConnectionTimeout: 250 * time.Millisecond,
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Certificate: []byte(selfSignedCertPEM),
			Key:         []byte(selfSignedKeyPEM),
		},
	})
	if err != nil {
		panic(err)
	}
	is := &inspectingServer{
		addr:       listener.Addr().String(),
		GRPCServer: srv,
		inspector:  inspector,
	}
	testpb.RegisterTestServiceServer(srv.Server(), is)
	return is
}

type inspection struct {
	tlsConfig *tls.Config
	server    *inspectingServer
	creds     credentials.TransportCredentials
	t         *testing.T
}

func (is *inspectingServer) newInspection(t *testing.T) *inspection {
	tlsConfig := &tls.Config{
		RootCAs: x509.NewCertPool(),
	}
	tlsConfig.RootCAs.AppendCertsFromPEM([]byte(selfSignedCertPEM))
	return &inspection{
		server:    is,
		creds:     credentials.NewTLS(tlsConfig),
		t:         t,
		tlsConfig: tlsConfig,
	}
}

func (ins *inspection) withMutualTLS() *inspection {
	cert, err := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	require.NoError(ins.t, err)
	ins.tlsConfig.Certificates = []tls.Certificate{cert}
	ins.creds = credentials.NewTLS(ins.tlsConfig)
	return ins
}

func (ins *inspection) inspectBinding(envelope *common.Envelope) error {
	ctx := context.Background()
	ctx, c := context.WithTimeout(ctx, time.Second*3)
	defer c()
	conn, err := grpc.DialContext(ctx, ins.server.addr, grpc.WithTransportCredentials(ins.creds), grpc.WithBlock())
	require.NoError(ins.t, err)
	defer conn.Close()
	_, err = testpb.NewTestServiceClient(conn).EmptyCall(context.Background(), &testpb.Empty{})
	require.NoError(ins.t, err)
	return ins.server.inspect(envelope)
}

func envelopeWithChannelHeader(ch []byte) *common.Envelope {
	pl := &common.Payload{
		Header: &common.Header{
			ChannelHeader: ch,
		},
	}
	payload, _ := proto.Marshal(pl)
	return &common.Envelope{
		Payload: payload,
	}
}
