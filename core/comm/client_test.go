/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	netctx "golang.org/x/net/context"
)

var caPEM, certPEM, keyPEM, serverKey, serverPEM []byte
var testClientCert, testServerCert tls.Certificate
var testTimeout = 10 * time.Second // conservative

type echoServer struct{}

func (es *echoServer) EchoCall(ctx netctx.Context,
	echo *testpb.Echo) (*testpb.Echo, error) {
	return echo, nil
}

func TestNewGRPCClient_GoodConfig(t *testing.T) {
	t.Parallel()
	loadCerts(t)

	config := comm.ClientConfig{}
	client, err := comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.Equal(t, tls.Certificate{}, client.Certificate())
	assert.False(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts := &comm.SecureOptions{
		UseTLS: false}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.Equal(t, tls.Certificate{}, client.Certificate())
	assert.False(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts = &comm.SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{caPEM},
		RequireClientCert: false}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.True(t, client.TLSEnabled())
	assert.False(t, client.MutualTLSRequired())

	secOpts = &comm.SecureOptions{
		Certificate:       certPEM,
		Key:               keyPEM,
		UseTLS:            true,
		ServerRootCAs:     [][]byte{caPEM},
		RequireClientCert: true}
	config.SecOpts = secOpts
	client, err = comm.NewGRPCClient(config)
	assert.NoError(t, err)
	assert.True(t, client.TLSEnabled())
	assert.True(t, client.MutualTLSRequired())
	assert.Equal(t, testClientCert, client.Certificate())

}

func TestNewGRPCClient_BadConfig(t *testing.T) {
	t.Parallel()
	loadCerts(t)
	// bad root cert
	config := comm.ClientConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{[]byte(badPEM)}}}
	_, err := comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), "error adding root certificate")

	// missing key
	missing := "both Key and Certificate " +
		"are required when using mutual TLS"
	config.SecOpts = &comm.SecureOptions{
		Certificate:       []byte("cert"),
		UseTLS:            true,
		RequireClientCert: true}
	_, err = comm.NewGRPCClient(config)
	assert.Equal(t, missing, err.Error())

	// missing cert
	config.SecOpts = &comm.SecureOptions{
		Key:               []byte("key"),
		UseTLS:            true,
		RequireClientCert: true}
	_, err = comm.NewGRPCClient(config)
	assert.Equal(t, missing, err.Error())

	// bad key
	failed := "failed to load client certificate"
	config.SecOpts = &comm.SecureOptions{
		Certificate:       certPEM,
		Key:               []byte(badPEM),
		UseTLS:            true,
		RequireClientCert: true}
	_, err = comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), failed)

	// bad cert
	config.SecOpts = &comm.SecureOptions{
		Certificate:       []byte(badPEM),
		Key:               keyPEM,
		UseTLS:            true,
		RequireClientCert: true}
	_, err = comm.NewGRPCClient(config)
	assert.Contains(t, err.Error(), failed)
}

func TestNewConnection_Timeout(t *testing.T) {
	t.Parallel()
	testAddress := "localhost:11111"
	config := comm.ClientConfig{
		Timeout: 1 * time.Second}
	client, err := comm.NewGRPCClient(config)
	conn, err := client.NewConnection(testAddress, "")
	assert.Contains(t, err.Error(), "context deadline exceeded")
	t.Log(err)
	assert.Nil(t, conn)
}

func TestNewConnection(t *testing.T) {
	t.Parallel()
	loadCerts(t)
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caPEM)
	if !ok {
		t.Fatal("failed to create test root cert pool")
	}

	tests := []struct {
		name       string
		serverPort int
		clientPort int
		config     comm.ClientConfig
		serverTLS  *tls.Config
		success    bool
		errorMsg   string
	}{
		{
			name:       "client / server same port",
			serverPort: 8351,
			clientPort: 8351,
			config: comm.ClientConfig{
				Timeout: testTimeout},
			success: true,
		},
		{
			name:       "client / server wrong port",
			serverPort: 8352,
			clientPort: 7352,
			config: comm.ClientConfig{
				Timeout: time.Second},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name:       "client TLS / server no TLS",
			serverPort: 8353,
			clientPort: 8353,
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:       certPEM,
					Key:               keyPEM,
					UseTLS:            true,
					ServerRootCAs:     [][]byte{caPEM},
					RequireClientCert: true},
				Timeout: time.Second},
			success:  false,
			errorMsg: "context deadline exceeded",
		},
		{
			name:       "client TLS / server TLS match",
			serverPort: 8354,
			clientPort: 8354,
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   certPEM,
					Key:           keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testServerCert}},
			success: true,
		},
		{
			name:       "client TLS / server TLS no server roots",
			serverPort: 8355,
			clientPort: 8355,
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   certPEM,
					Key:           keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testServerCert}},
			success:  false,
			errorMsg: "certificate signed by unknown authority",
		},
		{
			name:       "client TLS / server TLS missing client cert",
			serverPort: 8356,
			clientPort: 8356,
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:   certPEM,
					Key:           keyPEM,
					UseTLS:        true,
					ServerRootCAs: [][]byte{caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert},
			success:  false,
			errorMsg: "bad certificate",
		},
		{
			name:       "client TLS / server TLS client cert",
			serverPort: 8357,
			clientPort: 8357,
			config: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate:       certPEM,
					Key:               keyPEM,
					UseTLS:            true,
					RequireClientCert: true,
					ServerRootCAs:     [][]byte{caPEM}},
				Timeout: testTimeout},
			serverTLS: &tls.Config{
				Certificates: []tls.Certificate{testServerCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    certPool},
			success: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lis, err := net.Listen("tcp",
				fmt.Sprintf("localhost:%d", test.serverPort))
			if err != nil {
				t.Fatalf("error creating server for test: %v", err)
			}
			defer lis.Close()
			if test.serverTLS != nil {
				t.Log("starting server with TLS")
				tlsLis := tls.NewListener(lis, test.serverTLS)
				defer tlsLis.Close()
				go func() {
					rawConn, err := tlsLis.Accept()
					defer rawConn.Close()
					if err != nil {
						t.Fatalf("error accepting TLS connection: %v", err)
					}
					t.Logf("server: accepted from %s", rawConn.RemoteAddr())
					sconn := tls.Server(rawConn, test.serverTLS)
					err = sconn.Handshake()
					if err != nil {
						t.Logf("tls handshake error: %v", err)
					}
				}()
			}
			client, err := comm.NewGRPCClient(test.config)
			if err != nil {
				t.Fatalf("error creating client for test: %v", err)
			}
			conn, err := client.NewConnection(fmt.Sprintf("localhost:%d",
				test.clientPort), "")
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
	loadCerts(t)

	config := comm.ClientConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{caPEM}},
		Timeout: testTimeout}
	client, err := comm.NewGRPCClient(config)
	if err != nil {
		t.Fatalf("error creating base client: %v", err)
	}

	// set up test TLS server
	address := "localhost:8358"
	lis, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("failed to create listener for test server: %v", err)
	}
	defer lis.Close()
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{testServerCert}}
	tlsLis := tls.NewListener(lis, tlsConf)
	defer tlsLis.Close()
	go func() {
		// 3 connection tests
		for i := 0; i < 3; i++ {
			rawConn, err := tlsLis.Accept()
			defer rawConn.Close()
			if err != nil {
				t.Fatalf("error accepting TLS connection: %v", err)
			}
			t.Logf("server: accepted from %s", rawConn.RemoteAddr())
			sconn := tls.Server(rawConn, tlsConf)
			err = sconn.Handshake()
			if err != nil {
				t.Logf("tls handshake error: %v", err)
			}
		}
	}()

	// initial config should work
	t.Log("running initial good config")
	conn, err := client.NewConnection(address, "")
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	if conn != nil {
		conn.Close()
	}

	// no root certs
	t.Log("running bad config")
	err = client.SetServerRootCAs([][]byte{})
	assert.NoError(t, err)
	// now connection should fail
	_, err = client.NewConnection(address, "")
	assert.Error(t, err)

	// good root cert
	t.Log("running good config")
	err = client.SetServerRootCAs([][]byte{[]byte(caPEM)})
	assert.NoError(t, err)
	// now connection should succeed again
	conn, err = client.NewConnection(address, "")
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	conn.Close()

	// bad root cert
	t.Log("running bad root cert")
	err = client.SetServerRootCAs([][]byte{[]byte(badPEM)})
	assert.Contains(t, err.Error(), "error adding root certificate")
}

func TestSetMessageSize(t *testing.T) {
	t.Parallel()
	address := "localhost:8359"

	// setup test server
	srv, err := comm.NewGRPCServer(address, comm.ServerConfig{})
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
		Timeout: testTimeout})
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	// run tests
	for _, test := range tests {
		test := test
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
				Payload: []byte{0, 0, 0, 0, 0}}
			resp, err := svcClient.EchoCall(callCtx, echo)
			if !test.failRecv && !test.failSend {
				assert.NoError(t, err)
				assert.Equal(t, echo, resp)
			}
			if test.failSend {
				t.Logf("send error: %v", err)
				assert.Contains(t, err.Error(),
					"trying to send message larger than max")
			}
			if test.failRecv {
				t.Logf("recv error: %v", err)
				assert.Contains(t, err.Error(),
					"received message larger than max")
			}
		})
	}
}

func loadCerts(t *testing.T) {
	t.Helper()
	var err error
	caPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs",
		"Org1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading root cert for test: %v", err)
	}
	certPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs",
		"Org1-client1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading cert for test: %v", err)
	}
	keyPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs",
		"Org1-client1-key.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading key for test: %v", err)
	}
	testClientCert, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("unexpected error loading certificate for test: %v", err)
	}
	testServerCert, err = tls.LoadX509KeyPair(filepath.Join("testdata", "certs",
		"Org1-server1-cert.pem"), filepath.Join("testdata", "certs",
		"Org1-server1-key.pem"))
}
