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
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

const testTimeout = 1 * time.Second // conservative

type echoServer struct{}

func (es *echoServer) EchoCall(ctx context.Context,
	echo *testpb.Echo) (*testpb.Echo, error) {
	return echo, nil
}

func TestNewGRPCClient_GoodConfig(t *testing.T) {
	t.Parallel()
	testCerts := loadCerts(t)

	config := comm.ClientConfig{}
	client, err := comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.Equal(t, tls.Certificate{}, client.Certificate())
	assert.False(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts := &comm.SecureOptions{
		UseTLS: false,
	}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.Equal(t, tls.Certificate{}, client.Certificate())
	assert.False(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts = &comm.SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{testCerts.caPEM},
		RequireClientCert: false,
	}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.True(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts = &comm.SecureOptions{
		Certificate:       testCerts.certPEM,
		Key:               testCerts.keyPEM,
		UseTLS:            true,
		ServerRootCAs:     [][]byte{testCerts.caPEM},
		RequireClientCert: true,
	}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.True(t, client.TLSEnabled())
	assert.True(t, client.MutualTLSRequired())
	assert.Equal(t, testCerts.clientCert, client.Certificate())
}

func TestNewGRPCClient_BadConfig(t *testing.T) {
	t.Parallel()
	testCerts := loadCerts(t)

	// bad root cert
	config := comm.ClientConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{[]byte(badPEM)},
		},
	}
	_, err := comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), "error adding root certificate")

	// missing key
	missing := "both Key and Certificate are required when using mutual TLS"
	config.SecOpts = &comm.SecureOptions{
		Certificate:       []byte("cert"),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = comm.NewGRPCClient(config)
	assert.Equal(t, missing, err.Error())

	// missing cert
	config.SecOpts = &comm.SecureOptions{
		Key:               []byte("key"),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = comm.NewGRPCClient(config)
	assert.Equal(t, missing, err.Error())

	// bad key
	failed := "failed to load client certificate"
	config.SecOpts = &comm.SecureOptions{
		Certificate:       testCerts.certPEM,
		Key:               []byte(badPEM),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), failed)

	// bad cert
	config.SecOpts = &comm.SecureOptions{
		Certificate:       []byte(badPEM),
		Key:               testCerts.keyPEM,
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), failed)
}

func TestNewConnection_Timeout(t *testing.T) {
	t.Parallel()
	testAddress := "localhost:20040"
	config := comm.ClientConfig{
		Timeout: 1 * time.Second,
	}
	client, err := comm.NewGRPCClient(config)
	conn, err := client.NewConnection(testAddress, "")
	assert.Contains(t, err.Error(), "connection refused")
	t.Log(err)
	assert.Nil(t, conn)
}

func TestNewConnection(t *testing.T) {
	t.Parallel()
	testCerts := loadCerts(t)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(testCerts.caPEM)
	if !ok {
		t.Fatal("failed to create test root cert pool")
	}

	tests := []struct {
		name       string
		clientPort int
		config     comm.ClientConfig
		serverTLS  *tls.Config
		success    bool
		errorMsg   string
	}{
		{
			name: "client / server same port",
			config: comm.ClientConfig{
				Timeout: testTimeout,
			},
			success: true,
		},
		{
			name:       "client / server wrong port",
			clientPort: 20040,
			config: comm.ClientConfig{
				Timeout: time.Second,
			},
			success:  false,
			errorMsg: "connection refused",
		},
		{
			name: "client / server wrong port but with asynchronous should succeed",
			config: comm.ClientConfig{
				AsyncConnect: true,
				Timeout:      testTimeout,
			},
			clientPort: 20040,
			success:    true,
		},
		{
			name: "client TLS / server no TLS",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:       testCerts.certPEM,
					Key:               testCerts.keyPEM,
					UseTLS:            true,
					ServerRootCAs:     [][]byte{testCerts.caPEM},
					RequireClientCert: true,
				},
				Timeout: time.Second,
			},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name: "client TLS / server TLS match",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   testCerts.certPEM,
					Key:           testCerts.keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{testCerts.caPEM},
				},
				Timeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
			},
			success: true,
		},
		{
			name: "client TLS / server TLS no server roots",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   testCerts.certPEM,
					Key:           testCerts.keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{},
				},
				Timeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
			},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name: "client TLS / server TLS missing client cert",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   testCerts.certPEM,
					Key:           testCerts.keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{testCerts.caPEM},
				},
				Timeout: testTimeout,
			},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
			},
			success:  false,
			errorMsg: "tls: bad certificate",
		},
		{
			name: "client TLS / server TLS client cert",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:       testCerts.certPEM,
					Key:               testCerts.keyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool,
			},
			success: true,
		},
		{
			name: "server TLS pinning success",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					VerifyCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
						if bytes.Equal(rawCerts[0], testCerts.serverCert.Certificate[0]) {
							return nil
						}
						panic("mismatched certificate")
					},
					Certificate:       testCerts.certPEM,
					Key:               testCerts.keyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool,
			},
			success: true,
		},
		{
			name: "server TLS pinning failure",
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					VerifyCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
						return errors.New("TLS certificate mismatch")
					},
					Certificate:       testCerts.certPEM,
					Key:               testCerts.keyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{testCerts.caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testCerts.serverCert},
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
			client, err := comm.NewGRPCClient(test.config)
			if err != nil {
				t.Fatalf("error creating client for test: %v", err)
			}
			address := lis.Addr().String()
			if test.clientPort > 0 {
				address = fmt.Sprintf("localhost:%d", test.clientPort)
			}
			conn, err := client.NewConnection(address, "")
			if test.success {
				assert.NoError(t, err)
				assert.NotNil(t, conn)
			} else {
				t.Log(errors.WithStack(err))
				assert.Contains(t, err.Error(), test.errorMsg)
			}
		})
	}
}

func TestSetServerRootCAs(t *testing.T) {
	t.Parallel()
	testCerts := loadCerts(t)

	config := comm.ClientConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{testCerts.caPEM},
		},
		Timeout: testTimeout,
	}
	client, err := comm.NewGRPCClient(config)
	if err != nil {
		t.Fatalf("error creating base client: %v", err)
	}

	// set up test TLS server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener for test server: %v", err)
	}
	address := lis.Addr().String()
	t.Logf("server listening on [%s]", lis.Addr().String())
	t.Logf("client will use [%s]", address)
	defer lis.Close()
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{testCerts.serverCert},
	})))
	defer srv.Stop()
	go srv.Serve(lis)

	// initial config should work
	t.Log("running initial good config")
	conn, err := client.NewConnection(address, "")
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	if conn != nil {
		conn.Close()
	}

	// no root testCerts
	t.Log("running bad config")
	err = client.SetServerRootCAs([][]byte{})
	assert.NoError(t, err)
	// now connection should fail
	_, err = client.NewConnection(address, "")
	assert.Error(t, err)

	// good root cert
	t.Log("running good config")
	err = client.SetServerRootCAs([][]byte{[]byte(testCerts.caPEM)})
	assert.NoError(t, err)
	// now connection should succeed again
	conn, err = client.NewConnection(address, "")
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	if conn != nil {
		conn.Close()
	}

	// bad root cert
	t.Log("running bad root cert")
	err = client.SetServerRootCAs([][]byte{[]byte(badPEM)})
	assert.Contains(t, err.Error(), "error adding root certificate")
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

	var tests = []struct {
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

	// set up test client
	client, err := comm.NewGRPCClient(comm.ClientConfig{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	// run tests
	for _, test := range tests {
		test := test
		address := lis.Addr().String()
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.name)
			if test.maxRecvSize > 0 {
				client.SetMaxRecvMsgSize(test.maxRecvSize)
			}
			if test.maxSendSize > 0 {
				client.SetMaxSendMsgSize(test.maxSendSize)
			}
			conn, err := client.NewConnection(address, "")
			assert.NoError(t, err)
			defer conn.Close()
			// create service client from conn
			svcClient := testpb.NewEchoServiceClient(conn)
			callCtx := context.Background()
			callCtx, cancel := context.WithTimeout(callCtx, timeout)
			defer cancel()
			//invoke service
			echo := &testpb.Echo{
				Payload: []byte{0, 0, 0, 0, 0},
			}
			resp, err := svcClient.EchoCall(callCtx, echo)
			if !test.failRecv && !test.failSend {
				assert.NoError(t, err)
				assert.True(t, proto.Equal(echo, resp))
			}
			if test.failSend {
				t.Logf("send error: %v", err)
				assert.Contains(t, err.Error(), "trying to send message larger than max")
			}
			if test.failRecv {
				t.Logf("recv error: %v", err)
				assert.Contains(t, err.Error(), "received message larger than max")
			}
		})
	}
}

type testCerts struct {
	caPEM      []byte
	certPEM    []byte
	keyPEM     []byte
	serverKey  []byte
	serverPEM  []byte
	clientCert tls.Certificate
	serverCert tls.Certificate
}

func loadCerts(t *testing.T) testCerts {
	t.Helper()

	var certs testCerts
	var err error
	certs.caPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading root cert for test: %v", err)
	}
	certs.certPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-client1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading cert for test: %v", err)
	}
	certs.keyPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-client1-key.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading key for test: %v", err)
	}
	certs.clientCert, err = tls.X509KeyPair(certs.certPEM, certs.keyPEM)
	if err != nil {
		t.Fatalf("unexpected error loading certificate for test: %v", err)
	}
	certs.serverCert, err = tls.LoadX509KeyPair(
		filepath.Join("testdata", "certs", "Org1-server1-cert.pem"),
		filepath.Join("testdata", "certs", "Org1-server1-key.pem"),
	)

	return certs
}

func TestDynamicClientTLSLoading(t *testing.T) {
	t.Parallel()
	ca1, err := tlsgen.NewCA()
	assert.NoError(t, err)

	ca2, err := tlsgen.NewCA()
	assert.NoError(t, err)

	clientKP, err := ca1.NewClientCertKeyPair()
	assert.NoError(t, err)

	serverKP, err := ca2.NewServerCertKeyPair("127.0.0.1")
	assert.NoError(t, err)

	client, err := comm.NewGRPCClient(comm.ClientConfig{
		AsyncConnect: true,
		Timeout:      time.Second * 1,
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{ca1.CertBytes()},
			Certificate:   clientKP.Cert,
			Key:           clientKP.Key,
		},
	})
	assert.NoError(t, err)

	server, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		Logger: flogging.MustGetLogger("test"),
		SecOpts: &comm.SecureOptions{
			UseTLS:      true,
			Key:         serverKP.Key,
			Certificate: serverKP.Cert,
		},
	})
	assert.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		server.Start()
	}()

	var dynamicRootCerts atomic.Value
	dynamicRootCerts.Store(ca1.CertBytes())

	conn, err := client.NewConnection(server.Address(), "", func(tlsConfig *tls.Config) {
		tlsConfig.RootCAs = x509.NewCertPool()
		tlsConfig.RootCAs.AppendCertsFromPEM(dynamicRootCerts.Load().([]byte))
	})
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	waitForConnState := func(state connectivity.State, succeedOrFail string) {
		deadline := time.Now().Add(time.Second * 30)
		for conn.GetState() != state {
			time.Sleep(time.Millisecond * 10)
			if time.Now().After(deadline) {
				t.Fatalf("Test timed out, waited for connection to %s", succeedOrFail)
			}
		}
	}

	// Poll the connection state to wait for it to fail
	waitForConnState(connectivity.TransientFailure, "fail")

	// Update the TLS root CAs with the good one
	dynamicRootCerts.Store(ca2.CertBytes())

	// Reset exponential back-off to make the test faster
	conn.ResetConnectBackoff()

	// Poll the connection state to wait for it to succeed
	waitForConnState(connectivity.Ready, "succeed")

	err = conn.Close()
	assert.NoError(t, err)

	server.Stop()
	wg.Wait()
}
