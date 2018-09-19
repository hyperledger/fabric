/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	grpc_testdata "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestExtractCertificateHashFromContext(t *testing.T) {
	t.Parallel()
	assert.Nil(t, comm.ExtractCertificateHashFromContext(context.Background()))

	p := &peer.Peer{}
	ctx := peer.NewContext(context.Background(), p)
	assert.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = &nonTLSConnection{}
	ctx = peer.NewContext(context.Background(), p)
	assert.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{}
	ctx = peer.NewContext(context.Background(), p)
	assert.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Raw: []byte{1, 2, 3}},
			},
		},
	}
	ctx = peer.NewContext(context.Background(), p)
	assert.Equal(t, util.ComputeSHA256([]byte{1, 2, 3}), comm.ExtractCertificateHashFromContext(ctx))
}

type nonTLSConnection struct {
}

func (*nonTLSConnection) AuthType() string {
	return ""
}

func TestBindingInspectorBadInit(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		comm.NewBindingInspector(false, nil)
	})
}

func TestNoopBindingInspector(t *testing.T) {
	t.Parallel()
	extract := func(msg proto.Message) []byte {
		return nil
	}
	assert.Nil(t, comm.NewBindingInspector(false, extract)(context.Background(), &common.Envelope{}))
	err := comm.NewBindingInspector(false, extract)(context.Background(), nil)
	assert.Error(t, err)
	assert.Equal(t, "message is nil", err.Error())
}

func TestBindingInspector(t *testing.T) {
	t.Parallel()
	testAddress := "localhost:25000"
	extract := func(msg proto.Message) []byte {
		env, isEnvelope := msg.(*common.Envelope)
		if !isEnvelope || env == nil {
			return nil
		}
		ch, err := utils.ChannelHeader(env)
		if err != nil {
			return nil
		}
		return ch.TlsCertHash
	}
	srv := newInspectingServer(testAddress, comm.NewBindingInspector(true, extract))
	go srv.Start()
	defer srv.Stop()
	time.Sleep(time.Second)

	// Scenario I: Invalid header sent
	err := srv.newInspection(t).inspectBinding(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario II: invalid channel header
	ch, _ := proto.Marshal(utils.MakeChannelHeader(common.HeaderType_CONFIG, 0, "test", 0))
	// Corrupt channel header
	ch = append(ch, 0)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario III: No TLS cert hash in envelope
	chanHdr := utils.MakeChannelHeader(common.HeaderType_CONFIG, 0, "test", 0)
	ch, _ = proto.Marshal(chanHdr)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client didn't include its TLS cert hash")

	// Scenario IV: Client sends its TLS cert hash as needed, but doesn't use mutual TLS
	cert, _ := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	chanHdr.TlsCertHash = util.ComputeSHA256([]byte(cert.Certificate[0]))
	ch, _ = proto.Marshal(chanHdr)
	err = srv.newInspection(t).inspectBinding(envelopeWithChannelHeader(ch))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client didn't send a TLS certificate")

	// Scenario V: Client uses mutual TLS but sends the wrong TLS cert hash
	chanHdr.TlsCertHash = []byte{1, 2, 3}
	chHdrWithWrongTLSCertHash, _ := proto.Marshal(chanHdr)
	err = srv.newInspection(t).withMutualTLS().inspectBinding(envelopeWithChannelHeader(chHdrWithWrongTLSCertHash))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "claimed TLS cert hash is [1 2 3] but actual TLS cert hash is")

	// Scenario VI: Client uses mutual TLS and also sends the correct TLS cert hash
	err = srv.newInspection(t).withMutualTLS().inspectBinding(envelopeWithChannelHeader(ch))
	assert.NoError(t, err)
}

type inspectingServer struct {
	addr string
	*comm.GRPCServer
	lastContext atomic.Value
	inspector   comm.BindingInspector
}

func (is *inspectingServer) EmptyCall(ctx context.Context, _ *grpc_testdata.Empty) (*grpc_testdata.Empty, error) {
	is.lastContext.Store(ctx)
	return &grpc_testdata.Empty{}, nil
}

func (is *inspectingServer) inspect(envelope *common.Envelope) error {
	return is.inspector(is.lastContext.Load().(context.Context), envelope)
}

func newInspectingServer(addr string, inspector comm.BindingInspector) *inspectingServer {
	srv, err := comm.NewGRPCServer(addr, comm.ServerConfig{
		ConnectionTimeout: 250 * time.Millisecond,
		SecOpts: &comm.SecureOptions{
			UseTLS:      true,
			Certificate: []byte(selfSignedCertPEM),
			Key:         []byte(selfSignedKeyPEM),
		}})
	if err != nil {
		panic(err)
	}
	is := &inspectingServer{
		addr:       addr,
		GRPCServer: srv,
		inspector:  inspector,
	}
	grpc_testdata.RegisterTestServiceServer(srv.Server(), is)
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
	assert.NoError(ins.t, err)
	ins.tlsConfig.Certificates = []tls.Certificate{cert}
	ins.creds = credentials.NewTLS(ins.tlsConfig)
	return ins
}

func (ins *inspection) inspectBinding(envelope *common.Envelope) error {
	ctx := context.Background()
	ctx, c := context.WithTimeout(ctx, time.Second*3)
	defer c()
	conn, err := grpc.DialContext(ctx, ins.server.addr, grpc.WithTransportCredentials(ins.creds), grpc.WithBlock())
	defer conn.Close()
	assert.NoError(ins.t, err)
	_, err = grpc_testdata.NewTestServiceClient(conn).EmptyCall(context.Background(), &grpc_testdata.Empty{})
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
