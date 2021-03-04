/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

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
)

func TestBindingInspectorBadInit(t *testing.T) {
	require.Panics(t, func() { NewBindingInspector(false, nil) })
}

func TestNoopBindingInspector(t *testing.T) {
	extract := func(msg proto.Message) []byte {
		return nil
	}
	require.Nil(t, NewBindingInspector(false, extract)(context.Background(), &common.Envelope{}))
	err := NewBindingInspector(false, extract)(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, "message is nil", err.Error())
}

func TestBindingInspector(t *testing.T) {
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
	srv := newInspectingServer(lis, NewBindingInspector(true, extract))
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

type inspectingServer struct {
	addr string
	*comm.GRPCServer
	lastContext atomic.Value
	inspector   BindingInspector
}

func (is *inspectingServer) EmptyCall(ctx context.Context, _ *testpb.Empty) (*testpb.Empty, error) {
	is.lastContext.Store(ctx)
	return &testpb.Empty{}, nil
}

func (is *inspectingServer) inspect(envelope *common.Envelope) error {
	return is.inspector(is.lastContext.Load().(context.Context), envelope)
}

func newInspectingServer(listener net.Listener, inspector BindingInspector) *inspectingServer {
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

// Embedded certificates for testing
// The self-signed cert expires in 2028
var selfSignedKeyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMLemLh3+uDzww1pvqP6Xj2Z0Kc6yqf3RxyfTBNwRuuyoAoGCCqGSM49
AwEHoUQDQgAEDB3l94vM7EqKr2L/vhqU5IsEub0rviqCAaWGiVAPp3orb/LJqFLS
yo/k60rhUiir6iD4S4pb5TEb2ouWylQI3A==
-----END EC PRIVATE KEY-----
`

var selfSignedCertPEM = `-----BEGIN CERTIFICATE-----
MIIBdDCCARqgAwIBAgIRAKCiW5r6W32jGUn+l9BORMAwCgYIKoZIzj0EAwIwEjEQ
MA4GA1UEChMHQWNtZSBDbzAeFw0xODA4MjExMDI1MzJaFw0yODA4MTgxMDI1MzJa
MBIxEDAOBgNVBAoTB0FjbWUgQ28wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQM
HeX3i8zsSoqvYv++GpTkiwS5vSu+KoIBpYaJUA+neitv8smoUtLKj+TrSuFSKKvq
IPhLilvlMRvai5bKVAjco1EwTzAOBgNVHQ8BAf8EBAMCBaAwEwYDVR0lBAwwCgYI
KwYBBQUHAwEwDAYDVR0TAQH/BAIwADAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8A
AAEwCgYIKoZIzj0EAwIDSAAwRQIgOaYc3pdGf2j0uXRyvdBJq2PlK9FkgvsUjXOT
bQ9fWRkCIQCr1FiRRzapgtrnttDn3O2fhLlbrw67kClzY8pIIN42Qw==
-----END CERTIFICATE-----
`
