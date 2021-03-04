/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/comm/testpb"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const testTimeout = 1 * time.Second // conservative

type echoServer struct{}

func (es *echoServer) EchoCall(ctx context.Context,
	echo *testpb.Echo) (*testpb.Echo, error) {
	return echo, nil
}

func TestClientConfigDial(t *testing.T) {
	t.Parallel()
	testCerts := comm.LoadTestCerts(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	badAddress := l.Addr().String()
	defer l.Close()

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(testCerts.CAPEM)
	if !ok {
		t.Fatal("failed to create test root cert pool")
	}

	tests := []struct {
		name          string
		clientAddress string
		config        comm.ClientConfig
		serverTLS     *tls.Config
		success       bool
		errorMsg      string
	}{
		{
			name: "client / server same port",
			config: comm.ClientConfig{
				DialTimeout: testTimeout,
			},
			success: true,
		},
		{
			name:          "client / server wrong port",
			clientAddress: badAddress,
			config: comm.ClientConfig{
				DialTimeout: time.Second,
			},
			success:  false,
			errorMsg: "(connection refused|context deadline exceeded)",
		},
		{
			name:          "client / server wrong port but with asynchronous should succeed",
			clientAddress: badAddress,
			config: comm.ClientConfig{
				AsyncConnect: true,
				DialTimeout:  testTimeout,
			},
			success: true,
		},
		{
			name: "client TLS / server no TLS",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate:       testCerts.CertPEM,
					Key:               testCerts.KeyPEM,
					UseTLS:            true,
					ServerRootCAs:     [][]byte{testCerts.CAPEM},
					RequireClientCert: true,
				},
				DialTimeout: time.Second,
			},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name: "client TLS / server TLS match",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate:   testCerts.CertPEM,
					Key:           testCerts.KeyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{testCerts.CAPEM},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
			},
			success: true,
		},
		{
			name: "client TLS / server TLS no server roots",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate:   testCerts.CertPEM,
					Key:           testCerts.KeyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
			},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name: "client TLS / server TLS missing client cert",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate:   testCerts.CertPEM,
					Key:           testCerts.KeyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{testCerts.CAPEM},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				MaxVersion:   tls.VersionTLS12, // https://github.com/golang/go/issues/33368
			},
			success:  false,
			errorMsg: "tls: bad certificate",
		},
		{
			name: "client TLS / server TLS client cert",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate:       testCerts.CertPEM,
					Key:               testCerts.KeyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.CAPEM},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool,
			},
			success: true,
		},
		{
			name: "server TLS pinning success",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					VerifyCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
						if bytes.Equal(rawCerts[0], testCerts.ServerCert.Certificate[0]) {
							return nil
						}
						panic("mismatched certificate")
					},
					Certificate:       testCerts.CertPEM,
					Key:               testCerts.KeyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.CAPEM},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool,
			},
			success: true,
		},
		{
			name: "server TLS pinning failure",
			config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					VerifyCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
						return errors.New("TLS certificate mismatch")
					},
					Certificate:       testCerts.CertPEM,
					Key:               testCerts.KeyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.CAPEM},
				},
				DialTimeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.ServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool,
			},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("error creating server for test: %v", err)
			}
			defer lis.Close()
			serverOpts := []grpc.ServerOption{}
			if test.serverTLS != nil {
				serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(test.serverTLS)))
			}
			srv := grpc.NewServer(serverOpts...)
			defer srv.Stop()
			go srv.Serve(lis)
			address := lis.Addr().String()
			if test.clientAddress != "" {
				address = test.clientAddress
			}
			conn, err := test.config.Dial(address)
			if test.success {
				require.NoError(t, err)
				require.NotNil(t, conn)
			} else {
				t.Log(errors.WithStack(err))
				require.Regexp(t, test.errorMsg, err.Error())
			}
		})
	}
}

func TestSetMessageSize(t *testing.T) {
	t.Parallel()

	// setup test server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener for test server: %v", err)
	}
	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{})
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	testpb.RegisterEchoServiceServer(srv.Server(), &echoServer{})
	defer srv.Stop()
	go srv.Start()

	tests := []struct {
		name        string
		maxRecvSize int
		maxSendSize int
		failRecv    bool
		failSend    bool
	}{
		{
			name:     "defaults should pass",
			failRecv: false,
			failSend: false,
		},
		{
			name:        "non-defaults should pass",
			failRecv:    false,
			failSend:    false,
			maxRecvSize: 20,
			maxSendSize: 20,
		},
		{
			name:        "recv should fail",
			failRecv:    true,
			failSend:    false,
			maxRecvSize: 1,
		},
		{
			name:        "send should fail",
			failRecv:    false,
			failSend:    true,
			maxSendSize: 1,
		},
	}

	// run tests
	for _, test := range tests {
		test := test
		address := lis.Addr().String()
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			config := comm.ClientConfig{
				DialTimeout:    testTimeout,
				MaxRecvMsgSize: test.maxRecvSize,
				MaxSendMsgSize: test.maxSendSize,
			}
			conn, err := config.Dial(address)
			require.NoError(t, err)
			defer conn.Close()
			// create service client from conn
			svcClient := testpb.NewEchoServiceClient(conn)
			callCtx := context.Background()
			callCtx, cancel := context.WithTimeout(callCtx, testTimeout)
			defer cancel()
			// invoke service
			echo := &testpb.Echo{
				Payload: []byte{0, 0, 0, 0, 0},
			}
			resp, err := svcClient.EchoCall(callCtx, echo)
			if !test.failRecv && !test.failSend {
				require.NoError(t, err)
				require.True(t, proto.Equal(echo, resp))
			}
			if test.failSend {
				t.Logf("send error: %v", err)
				require.Contains(t, err.Error(), "trying to send message larger than max")
			}
			if test.failRecv {
				t.Logf("recv error: %v", err)
				require.Contains(t, err.Error(), "received message larger than max")
			}
		})
	}
}
