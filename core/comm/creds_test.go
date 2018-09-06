/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/credentials"

	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/stretchr/testify/assert"
)

func TestCreds(t *testing.T) {
	t.Parallel()

	caPEM, err := ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	if err != nil {
		t.Fatalf("failed to read root certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caPEM)
	if !ok {
		t.Fatalf("failed to create certPool")
	}
	cert, err := tls.LoadX509KeyPair(
		filepath.Join("testdata", "certs", "Org1-server1-cert.pem"),
		filepath.Join("testdata", "certs", "Org1-server1-key.pem"),
	)
	if err != nil {
		t.Fatalf("failed to load TLS certificate [%s]", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert}}

	logger, recorder := floggingtest.NewTestLogger(t)

	var creds credentials.TransportCredentials
	creds = comm.NewServerTransportCredentials(tlsConfig, logger)
	_, _, err = creds.ClientHandshake(nil, "", nil)
	assert.EqualError(t, err, comm.ClientHandshakeNotImplError.Error())
	err = creds.OverrideServerName("")
	assert.EqualError(t, err, comm.OverrrideHostnameNotSupportedError.Error())
	clone := creds.Clone()
	assert.Equal(t, creds, clone)
	assert.Equal(t, "1.2", creds.Info().SecurityVersion)
	assert.Equal(t, "tls", creds.Info().SecurityProtocol)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to start listener [%s]", err)
	}
	defer lis.Close()

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Logf("failed to accept connection [%s]", err)
		}
		_, _, err = creds.ServerHandshake(conn)
		if err != nil {
			t.Logf("ServerHandshake error [%s]", err)
		}
		conn, err = lis.Accept()
		if err != nil {
			t.Logf("failed to accept connection [%s]", err)
		}
		_, _, err = creds.ServerHandshake(conn)
		if err != nil {
			t.Logf("ServerHandshake error [%s]", err)
		}
	}()

	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		t.Fatalf("failed to get server port [%s]", err)
	}
	_, err = tls.Dial("tcp", fmt.Sprintf("localhost:%s", port),
		&tls.Config{RootCAs: certPool})
	assert.NoError(t, err)

	_, err = tls.Dial("tcp", fmt.Sprintf("localhost:%s", port),
		&tls.Config{
			RootCAs:    certPool,
			MaxVersion: tls.VersionTLS10})
	assert.Contains(t, err.Error(), "protocol version not supported")
	assert.Contains(t, recorder.Messages()[0], "TLS handshake failed with error")

}
