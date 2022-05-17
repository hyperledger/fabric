/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/require"
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
		Certificates: []tls.Certificate{cert},
	}

	config := comm.NewTLSConfig(tlsConfig)

	logger, recorder := floggingtest.NewTestLogger(t)

	creds := comm.NewServerTransportCredentials(config, logger)
	_, _, err = creds.ClientHandshake(context.Background(), "", nil)
	require.EqualError(t, err, comm.ErrClientHandshakeNotImplemented.Error())
	err = creds.OverrideServerName("")
	require.EqualError(t, err, comm.ErrOverrideHostnameNotSupported.Error())
	require.Equal(t, "1.2", creds.Info().SecurityVersion)
	require.Equal(t, "tls", creds.Info().SecurityProtocol)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to start listener [%s]", err)
	}
	defer lis.Close()

	_, port, err := net.SplitHostPort(lis.Addr().String())
	require.NoError(t, err)
	addr := net.JoinHostPort("localhost", port)

	handshake := func(wg *sync.WaitGroup) {
		defer wg.Done()
		conn, err := lis.Accept()
		if err != nil {
			t.Logf("failed to accept connection [%s]", err)
		}
		_, _, err = creds.ServerHandshake(conn)
		if err != nil {
			t.Logf("ServerHandshake error [%s]", err)
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go handshake(wg)
	_, err = tls.Dial("tcp", addr, &tls.Config{RootCAs: certPool})
	wg.Wait()
	require.NoError(t, err)

	wg = &sync.WaitGroup{}
	wg.Add(1)
	go handshake(wg)
	_, err = tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    certPool,
		MaxVersion: tls.VersionTLS10,
	})
	wg.Wait()
	require.Contains(t, err.Error(), "tls: no supported versions satisfy MinVersion and MaxVersion")
	require.Contains(t, recorder.Messages()[1], "TLS handshake failed")
}

func TestNewTLSConfig(t *testing.T) {
	t.Parallel()
	tlsConfig := &tls.Config{}

	config := comm.NewTLSConfig(tlsConfig)

	require.NotEmpty(t, config, "TLSConfig is not empty")
}

func TestConfig(t *testing.T) {
	t.Parallel()
	config := comm.NewTLSConfig(&tls.Config{
		ServerName: "bueno",
	})

	configCopy := config.Config()

	certPool := x509.NewCertPool()
	config.SetClientCAs(certPool)

	require.NotEqual(t, config.Config(), &configCopy, "TLSConfig should have new certs")
}

func TestAddRootCA(t *testing.T) {
	t.Parallel()

	caPEM, err := ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	require.NoError(t, err, "failed to read root certificate")

	expectedCertPool := x509.NewCertPool()
	ok := expectedCertPool.AppendCertsFromPEM(caPEM)
	require.True(t, ok, "failed to create expected certPool")

	cert := &x509.Certificate{EmailAddresses: []string{"test@foobar.com"}}
	expectedCertPool.AddCert(cert)

	certPool := x509.NewCertPool()
	ok = certPool.AppendCertsFromPEM(caPEM)
	require.True(t, ok, "failed to create certPool")

	config := comm.NewTLSConfig(&tls.Config{ClientCAs: certPool})
	require.Same(t, config.Config().ClientCAs, certPool)

	// https://go-review.googlesource.com/c/go/+/229917
	config.AddClientRootCA(cert)
	require.Equal(t, certPool.Subjects(), expectedCertPool.Subjects(), "subjects in the pool should be equal")
}

func TestSetClientCAs(t *testing.T) {
	t.Parallel()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}
	config := comm.NewTLSConfig(tlsConfig)

	require.Empty(t, config.Config().ClientCAs, "No CertPool should be defined")

	certPool := x509.NewCertPool()
	config.SetClientCAs(certPool)

	require.NotNil(t, config.Config().ClientCAs, "The CertPools' should not be the same")
}
