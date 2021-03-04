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
	"io"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/comm/testpb"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

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

var badPEM = `-----BEGIN CERTIFICATE-----
MIICRDCCAemgAwIBAgIJALwW//dz2ZBvMAoGCCqGSM49BAMCMH4xCzAJBgNVBAYT
AlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRYwFAYDVQQHDA1TYW4gRnJhbmNpc2Nv
MRgwFgYDVQQKDA9MaW51eEZvdW5kYXRpb24xFDASBgNVBAsMC0h5cGVybGVkZ2Vy
MRIwEAYDVQQDDAlsb2NhbGhvc3QwHhcNMTYxMjA0MjIzMDE4WhcNMjYxMjAyMjIz
MDE4WjB+MQswCQYDVQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UE
BwwNU2FuIEZyYW5jaXNjbzEYMBYGA1UECgwPTGludXhGb3VuZGF0aW9uMRQwEgYD
VQQLDAtIeXBlcmxlZGdlcjESMBAGA1UEAwwJbG9jYWxob3N0MFkwEwYHKoZIzj0C
-----END CERTIFICATE-----
`

var testOrgs = []testOrg{}

func init() {
	// load up crypto material for test orgs
	for i := 1; i <= numOrgs; i++ {
		testOrg, err := loadOrg(i)
		if err != nil {
			log.Fatalf("Failed to load test organizations due to error: %s", err.Error())
		}
		testOrgs = append(testOrgs, testOrg)
	}
}

// test servers to be registered with the GRPCServer
type emptyServiceServer struct{}

func (ess *emptyServiceServer) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	return new(testpb.Empty), nil
}

func (esss *emptyServiceServer) EmptyStream(stream testpb.EmptyService_EmptyStreamServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&testpb.Empty{}); err != nil {
			return err
		}

	}
}

// invoke the EmptyCall RPC
func invokeEmptyCall(address string, dialOptions ...grpc.DialOption) (*testpb.Empty, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	// create GRPC client conn
	clientConn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return nil, err
	}
	defer clientConn.Close()

	// create GRPC client
	client := testpb.NewEmptyServiceClient(clientConn)

	// invoke service
	empty, err := client.EmptyCall(context.Background(), new(testpb.Empty))
	if err != nil {
		return nil, err
	}

	return empty, nil
}

// invoke the EmptyStream RPC
func invokeEmptyStream(address string, dialOptions ...grpc.DialOption) (*testpb.Empty, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	// create GRPC client conn
	clientConn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return nil, err
	}
	defer clientConn.Close()

	stream, err := testpb.NewEmptyServiceClient(clientConn).EmptyStream(ctx)
	if err != nil {
		return nil, err
	}

	var msg *testpb.Empty
	var streamErr error

	waitc := make(chan struct{})
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				close(waitc)
				return
			}
			if err != nil {
				streamErr = err
				close(waitc)
				return
			}
			msg = in
		}
	}()

	// TestServerInterceptors adds an interceptor that does not call the target
	// StreamHandler and returns an error so Send can return with an io.EOF since
	// the server side has already terminated. Whether or not we get an error
	// depends on timing.
	err = stream.Send(&testpb.Empty{})
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("stream send failed: %s", err)
	}

	stream.CloseSend()
	<-waitc
	return msg, streamErr
}

const (
	numOrgs        = 2
	numChildOrgs   = 2
	numServerCerts = 2
)

// string for cert filenames
var (
	orgCACert       = filepath.Join("testdata", "certs", "Org%d-cert.pem")
	orgServerKey    = filepath.Join("testdata", "certs", "Org%d-server%d-key.pem")
	orgServerCert   = filepath.Join("testdata", "certs", "Org%d-server%d-cert.pem")
	orgClientKey    = filepath.Join("testdata", "certs", "Org%d-client%d-key.pem")
	orgClientCert   = filepath.Join("testdata", "certs", "Org%d-client%d-cert.pem")
	childCACert     = filepath.Join("testdata", "certs", "Org%d-child%d-cert.pem")
	childServerKey  = filepath.Join("testdata", "certs", "Org%d-child%d-server%d-key.pem")
	childServerCert = filepath.Join("testdata", "certs", "Org%d-child%d-server%d-cert.pem")
	childClientKey  = filepath.Join("testdata", "certs", "Org%d-child%d-client%d-key.pem")
	childClientCert = filepath.Join("testdata", "certs", "Org%d-child%d-client%d-cert.pem")
)

type testServer struct {
	config comm.ServerConfig
}

type serverCert struct {
	keyPEM  []byte
	certPEM []byte
}

type testOrg struct {
	rootCA      []byte
	serverCerts []serverCert
	clientCerts []tls.Certificate
	childOrgs   []testOrg
}

// return *X509.CertPool for the rootCA of the org
func (org *testOrg) rootCertPool() *x509.CertPool {
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(org.rootCA)
	return certPool
}

// return testServers for the org
func (org *testOrg) testServers(clientRootCAs [][]byte) []testServer {
	clientRootCAs = append(clientRootCAs, org.rootCA)

	// loop through the serverCerts and create testServers
	testServers := []testServer{}
	for _, serverCert := range org.serverCerts {
		testServer := testServer{
			comm.ServerConfig{
				ConnectionTimeout: 250 * time.Millisecond,
				SecOpts: comm.SecureOptions{
					UseTLS:            true,
					Certificate:       serverCert.certPEM,
					Key:               serverCert.keyPEM,
					RequireClientCert: true,
					ClientRootCAs:     clientRootCAs,
				},
			},
		}
		testServers = append(testServers, testServer)
	}
	return testServers
}

// return trusted clients for the org
func (org *testOrg) trustedClients(serverRootCAs [][]byte) []*tls.Config {
	// if we have any additional server root CAs add them to the certPool
	certPool := org.rootCertPool()
	for _, serverRootCA := range serverRootCAs {
		certPool.AppendCertsFromPEM(serverRootCA)
	}

	// loop through the clientCerts and create tls.Configs
	trustedClients := []*tls.Config{}
	for _, clientCert := range org.clientCerts {
		trustedClient := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      certPool,
		}
		trustedClients = append(trustedClients, trustedClient)
	}
	return trustedClients
}

// createCertPool creates an x509.CertPool from an array of PEM-encoded certificates
func createCertPool(rootCAs [][]byte) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		if !certPool.AppendCertsFromPEM(rootCA) {
			return nil, errors.New("Failed to load root certificates")
		}
	}
	return certPool, nil
}

// utility function to load crypto material for organizations
func loadOrg(parent int) (testOrg, error) {
	org := testOrg{}
	// load the CA
	caPEM, err := ioutil.ReadFile(fmt.Sprintf(orgCACert, parent))
	if err != nil {
		return org, err
	}

	// loop through and load servers
	serverCerts := []serverCert{}
	for i := 1; i <= numServerCerts; i++ {
		keyPEM, err := ioutil.ReadFile(fmt.Sprintf(orgServerKey, parent, i))
		if err != nil {
			return org, err
		}
		certPEM, err := ioutil.ReadFile(fmt.Sprintf(orgServerCert, parent, i))
		if err != nil {
			return org, err
		}
		serverCerts = append(serverCerts, serverCert{keyPEM, certPEM})
	}

	// loop through and load clients
	clientCerts := []tls.Certificate{}
	for j := 1; j <= numServerCerts; j++ {
		clientCert, err := loadTLSKeyPairFromFile(fmt.Sprintf(orgClientKey, parent, j),
			fmt.Sprintf(orgClientCert, parent, j))
		if err != nil {
			return org, err
		}
		clientCerts = append(clientCerts, clientCert)
	}

	// loop through and load child orgs
	childOrgs := []testOrg{}
	for k := 1; k <= numChildOrgs; k++ {
		childOrg, err := loadChildOrg(parent, k)
		if err != nil {
			return org, err
		}
		childOrgs = append(childOrgs, childOrg)
	}

	return testOrg{caPEM, serverCerts, clientCerts, childOrgs}, nil
}

// utility function to load crypto material for child organizations
func loadChildOrg(parent, child int) (testOrg, error) {
	// load the CA
	caPEM, err := ioutil.ReadFile(fmt.Sprintf(childCACert, parent, child))
	if err != nil {
		return testOrg{}, err
	}

	// loop through and load servers
	serverCerts := []serverCert{}
	for i := 1; i <= numServerCerts; i++ {
		keyPEM, err := ioutil.ReadFile(fmt.Sprintf(childServerKey, parent, child, i))
		if err != nil {
			return testOrg{}, err
		}
		certPEM, err := ioutil.ReadFile(fmt.Sprintf(childServerCert, parent, child, i))
		if err != nil {
			return testOrg{}, err
		}
		serverCerts = append(serverCerts, serverCert{keyPEM, certPEM})
	}

	// loop through and load clients
	clientCerts := []tls.Certificate{}
	for j := 1; j <= numServerCerts; j++ {
		clientCert, err := loadTLSKeyPairFromFile(
			fmt.Sprintf(childClientKey, parent, child, j),
			fmt.Sprintf(childClientCert, parent, child, j),
		)
		if err != nil {
			return testOrg{}, err
		}
		clientCerts = append(clientCerts, clientCert)
	}

	return testOrg{caPEM, serverCerts, clientCerts, []testOrg{}}, nil
}

// loadTLSKeyPairFromFile creates a tls.Certificate from PEM-encoded key and cert files
func loadTLSKeyPairFromFile(keyFile, certFile string) (tls.Certificate, error) {
	certPEMBlock, err := ioutil.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyPEMBlock, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

func TestNewGRPCServerInvalidParameters(t *testing.T) {
	t.Parallel()

	// missing address
	_, err := comm.NewGRPCServer(
		"",
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.EqualError(t, err, "missing address parameter")

	// missing port
	_, err = comm.NewGRPCServer(
		"abcdef",
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.Error(t, err, "Expected error with missing port")
	require.Contains(t, err.Error(), "missing port in address")

	// bad port
	_, err = comm.NewGRPCServer(
		"127.0.0.1:1BBB",
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	// check for possible errors based on platform and Go release
	msgs := []string{
		"listen tcp: lookup tcp/1BBB: nodename nor servname provided, or not known",
		"listen tcp: unknown port tcp/1BBB",
		"listen tcp: address tcp/1BBB: unknown port",
		"listen tcp: lookup tcp/1BBB: Servname not supported for ai_socktype",
	}
	require.Error(t, err, fmt.Sprintf("[%s], [%s] [%s] or [%s] expected", msgs[0], msgs[1], msgs[2], msgs[3]))
	require.Contains(t, msgs, err.Error())

	// bad hostname
	_, err = comm.NewGRPCServer(
		"hostdoesnotexist.localdomain:9050",
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	// We cannot check for a specific error message due to the fact that some
	// systems will automatically resolve unknown host names to a "search"
	// address so we just check to make sure that an error was returned
	require.Error(t, err, "error expected")

	// address in use
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")
	defer lis.Close()

	_, err = comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.NoError(t, err, "failed to create grpc server")

	_, err = comm.NewGRPCServer(
		lis.Addr().String(),
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "address already in use")

	// missing server Certificate
	_, err = comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{
			SecOpts: comm.SecureOptions{UseTLS: true, Key: []byte{}},
		},
	)
	require.EqualError(t, err, "serverConfig.SecOpts must contain both Key and Certificate when UseTLS is true")

	// missing server Key
	_, err = comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{
			SecOpts: comm.SecureOptions{
				UseTLS:      true,
				Certificate: []byte{},
			},
		},
	)
	require.EqualError(t, err, "serverConfig.SecOpts must contain both Key and Certificate when UseTLS is true")

	// bad server Key
	_, err = comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{
			SecOpts: comm.SecureOptions{
				UseTLS:      true,
				Certificate: []byte(selfSignedCertPEM),
				Key:         []byte{},
			},
		},
	)
	require.EqualError(t, err, "tls: failed to find any PEM data in key input")

	// bad server Certificate
	_, err = comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{
			SecOpts: comm.SecureOptions{
				UseTLS:      true,
				Certificate: []byte{},
				Key:         []byte(selfSignedKeyPEM),
			},
		},
	)
	require.EqualError(t, err, "tls: failed to find any PEM data in certificate input")

	srv, err := comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{
			SecOpts: comm.SecureOptions{
				UseTLS:            true,
				Certificate:       []byte(selfSignedCertPEM),
				Key:               []byte(selfSignedKeyPEM),
				RequireClientCert: true,
			},
		},
	)
	require.NoError(t, err)

	badRootCAs := [][]byte{[]byte(badPEM)}
	err = srv.SetClientRootCAs(badRootCAs)
	require.EqualError(t, err, "failed to set client root certificate(s)")
}

func TestNewGRPCServer(t *testing.T) {
	t.Parallel()

	testAddress := "127.0.0.1:9053"
	srv, err := comm.NewGRPCServer(
		testAddress,
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.NoError(t, err, "failed to create new GRPC server")

	// resolve the address
	addr, err := net.ResolveTCPAddr("tcp", testAddress)
	require.NoError(t, err)

	// make sure our properties are as expected
	require.Equal(t, srv.Address(), addr.String())
	require.Equal(t, srv.Listener().Addr().String(), addr.String())
	require.Equal(t, srv.TLSEnabled(), false)
	require.Equal(t, srv.MutualTLSRequired(), false)

	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go srv.Start()
	defer srv.Stop()

	// should not be needed
	time.Sleep(10 * time.Millisecond)

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithInsecure())
	require.NoError(t, err, "failed to invoke the EmptyCall service")
}

func TestNewGRPCServerFromListener(t *testing.T) {
	t.Parallel()

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")
	testAddress := lis.Addr().String()

	srv, err := comm.NewGRPCServerFromListener(
		lis,
		comm.ServerConfig{SecOpts: comm.SecureOptions{UseTLS: false}},
	)
	require.NoError(t, err, "failed to create new GRPC server")

	require.Equal(t, srv.Address(), testAddress)
	require.Equal(t, srv.Listener().Addr().String(), testAddress)
	require.Equal(t, srv.TLSEnabled(), false)
	require.Equal(t, srv.MutualTLSRequired(), false)

	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go srv.Start()
	defer srv.Stop()

	// should not be needed
	time.Sleep(10 * time.Millisecond)

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithInsecure())
	require.NoError(t, err, "client failed to invoke the EmptyCall service")
}

func TestNewSecureGRPCServer(t *testing.T) {
	t.Parallel()

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")
	testAddress := lis.Addr().String()

	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{
		ConnectionTimeout: 250 * time.Millisecond,
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Certificate: []byte(selfSignedCertPEM),
			Key:         []byte(selfSignedKeyPEM),
		},
	},
	)
	require.NoError(t, err, "failed to create new grpc server")

	// make sure our properties are as expected
	require.NoError(t, err)
	require.Equal(t, srv.Address(), testAddress)
	require.Equal(t, srv.Listener().Addr().String(), testAddress)

	cert, _ := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	require.Equal(t, srv.ServerCertificate(), cert)

	require.Equal(t, srv.TLSEnabled(), true)
	require.Equal(t, srv.MutualTLSRequired(), false)

	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go srv.Start()
	defer srv.Stop()

	// should not be needed
	time.Sleep(10 * time.Millisecond)

	// create the client credentials
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM([]byte(selfSignedCertPEM)) {
		t.Fatal("Failed to append certificate to client credentials")
	}
	creds := credentials.NewClientTLSFromCert(certPool, "")

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds))
	require.NoError(t, err, "client failed to invoke the EmptyCall service")

	// Test TLS versions which should be valid
	tlsVersions := map[string]uint16{
		"TLS12": tls.VersionTLS12,
		"TLS13": tls.VersionTLS13,
	}
	for name, tlsVersion := range tlsVersions {
		tlsVersion := tlsVersion

		t.Run(name, func(t *testing.T) {
			creds := credentials.NewTLS(&tls.Config{RootCAs: certPool, MinVersion: tlsVersion, MaxVersion: tlsVersion})
			_, err := invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds), grpc.WithBlock())
			require.NoError(t, err)
		})
	}

	// Test TLS versions which should be invalid
	tlsVersions = map[string]uint16{
		"SSL30": tls.VersionSSL30,
		"TLS10": tls.VersionTLS10,
		"TLS11": tls.VersionTLS11,
	}
	for name, tlsVersion := range tlsVersions {
		tlsVersion := tlsVersion
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			creds := credentials.NewTLS(&tls.Config{RootCAs: certPool, MinVersion: tlsVersion, MaxVersion: tlsVersion})
			_, err := invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds), grpc.WithBlock())
			require.Error(t, err, "should not have been able to connect with TLS version < 1.2")
			require.Contains(t, err.Error(), "context deadline exceeded")
		})
	}
}

func TestVerifyCertificateCallback(t *testing.T) {
	t.Parallel()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	authorizedClientKeyPair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	notAuthorizedClientKeyPair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	verifyFunc := func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if bytes.Equal(rawCerts[0], authorizedClientKeyPair.TLSCert.Raw) {
			return nil
		}
		return errors.New("certificate mismatch")
	}

	probeTLS := func(endpoint string, clientKeyPair *tlsgen.CertKeyPair) error {
		cert, err := tls.X509KeyPair(clientKeyPair.Cert, clientKeyPair.Key)
		if err != nil {
			return err
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      x509.NewCertPool(),
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   tls.VersionTLS12,
		}
		tlsCfg.RootCAs.AppendCertsFromPEM(ca.CertBytes())

		conn, err := tls.Dial("tcp", endpoint, tlsCfg)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}

	gRPCServer, err := comm.NewGRPCServer("127.0.0.1:", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			ClientRootCAs:     [][]byte{ca.CertBytes()},
			Key:               serverKeyPair.Key,
			Certificate:       serverKeyPair.Cert,
			UseTLS:            true,
			VerifyCertificate: verifyFunc,
		},
	})
	go gRPCServer.Start()
	defer gRPCServer.Stop()

	t.Run("Success path", func(t *testing.T) {
		err = probeTLS(gRPCServer.Address(), authorizedClientKeyPair)
		require.NoError(t, err)
	})

	t.Run("Failure path", func(t *testing.T) {
		err = probeTLS(gRPCServer.Address(), notAuthorizedClientKeyPair)
		require.EqualError(t, err, "remote error: tls: bad certificate")
	})
}

// prior tests used self-signed certficates loaded by the GRPCServer and the test client
// here we'll use certificates signed by certificate authorities
func TestWithSignedRootCertificates(t *testing.T) {
	t.Parallel()

	// use Org1 testdata
	fileBase := "Org1"
	certPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-cert.pem"))
	require.NoError(t, err, "failed to load test certificates")

	keyPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-key.pem"))
	require.NoError(t, err, "failed to load test certificates: %v")

	caPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-cert.pem"))
	require.NoError(t, err, "failed to load test certificates")

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")
	testAddress := lis.Addr().String()

	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Certificate: certPEMBlock,
			Key:         keyPEMBlock,
		},
	})
	require.NoError(t, err, "failed to create new grpc server")
	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go srv.Start()
	defer srv.Stop()

	// should not be needed
	time.Sleep(10 * time.Millisecond)

	// create a CertPool for use by the client with the server cert only
	certPoolServer, err := createCertPool([][]byte{certPEMBlock})
	require.NoError(t, err, "failed to load root certificates into pool")
	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds))
	require.NoError(t, err, "Expected client to connect with server cert only")

	// now use the CA certificate
	certPoolCA := x509.NewCertPool()
	if !certPoolCA.AppendCertsFromPEM(caPEMBlock) {
		t.Fatal("Failed to append certificate to client credentials")
	}
	creds = credentials.NewClientTLSFromCert(certPoolCA, "")

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds))
	require.NoError(t, err, "client failed to invoke the EmptyCall")
}

// here we'll use certificates signed by intermediate certificate authorities
func TestWithSignedIntermediateCertificates(t *testing.T) {
	t.Parallel()

	// use Org1 testdata
	fileBase := "Org1"
	certPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-server1-cert.pem"))
	require.NoError(t, err)

	keyPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-server1-key.pem"))
	require.NoError(t, err)

	intermediatePEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-cert.pem"))
	if err != nil {
		t.Fatalf("Failed to load test certificates: %v", err)
	}

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	testAddress := lis.Addr().String()

	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Certificate: certPEMBlock,
			Key:         keyPEMBlock,
		},
	})
	// check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go srv.Start()
	defer srv.Stop()

	// should not be needed
	time.Sleep(10 * time.Millisecond)

	// create a CertPool for use by the client with the server cert only
	certPoolServer, err := createCertPool([][]byte{certPEMBlock})
	if err != nil {
		t.Fatalf("Failed to load root certificates into pool: %v", err)
	}
	// create the client credentials
	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds))

	// client should be able to connect with Go 1.9
	require.NoError(t, err, "Expected client to connect with server cert only")

	// now use the CA certificate
	// create a CertPool for use by the client with the intermediate root CA
	certPoolCA, err := createCertPool([][]byte{intermediatePEMBlock})
	require.NoError(t, err, "failed to load root certificates into pool")

	creds = credentials.NewClientTLSFromCert(certPoolCA, "")

	// invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, grpc.WithTransportCredentials(creds))
	require.NoError(t, err, "client failed to invoke the EmptyCall service")
}

// utility function for testing client / server communication using TLS
func runMutualAuth(t *testing.T, servers []testServer, trustedClients, unTrustedClients []*tls.Config) error {
	// loop through all the test servers
	for i := 0; i < len(servers); i++ {
		// create listener
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		srvAddr := lis.Addr().String()

		// create GRPCServer
		srv, err := comm.NewGRPCServerFromListener(lis, servers[i].config)
		if err != nil {
			return err
		}

		// MutualTLSRequired should be true
		require.Equal(t, srv.MutualTLSRequired(), true)

		// register the GRPC test server and start the GRPCServer
		testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})
		go srv.Start()
		defer srv.Stop()

		// should not be needed but just in case
		time.Sleep(10 * time.Millisecond)

		// loop through all the trusted clients
		for j := 0; j < len(trustedClients); j++ {
			// invoke the EmptyCall service
			_, err = invokeEmptyCall(srvAddr, grpc.WithTransportCredentials(credentials.NewTLS(trustedClients[j])))
			// we expect success from trusted clients
			if err != nil {
				return err
			} else {
				t.Logf("Trusted client%d successfully connected to %s", j, srvAddr)
			}
		}

		// loop through all the untrusted clients
		for k := 0; k < len(unTrustedClients); k++ {
			// invoke the EmptyCall service
			_, err = invokeEmptyCall(
				srvAddr,
				grpc.WithTransportCredentials(credentials.NewTLS(unTrustedClients[k])),
			)
			// we expect failure from untrusted clients
			if err != nil {
				t.Logf("Untrusted client%d was correctly rejected by %s", k, srvAddr)
			} else {
				return fmt.Errorf("Untrusted client %d should not have been able to connect to %s", k, srvAddr)
			}
		}
	}

	return nil
}

func TestMutualAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		servers          []testServer
		trustedClients   []*tls.Config
		unTrustedClients []*tls.Config
	}{
		{
			name:             "ClientAuthRequiredWithSingleOrg",
			servers:          testOrgs[0].testServers([][]byte{}),
			trustedClients:   testOrgs[0].trustedClients([][]byte{}),
			unTrustedClients: testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithChildClientOrg",
			servers:          testOrgs[0].testServers([][]byte{testOrgs[0].childOrgs[0].rootCA}),
			trustedClients:   testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA}),
			unTrustedClients: testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name: "ClientAuthRequiredWithMultipleChildClientOrgs",
			servers: testOrgs[0].testServers(append([][]byte{},
				testOrgs[0].childOrgs[0].rootCA,
				testOrgs[0].childOrgs[1].rootCA,
			)),
			trustedClients: append(append([]*tls.Config{},
				testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})...),
				testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})...),
			unTrustedClients: testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithDifferentServerAndClientOrgs",
			servers:          testOrgs[0].testServers([][]byte{testOrgs[1].rootCA}),
			trustedClients:   testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
			unTrustedClients: testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithDifferentServerAndChildClientOrgs",
			servers:          testOrgs[1].testServers([][]byte{testOrgs[0].childOrgs[0].rootCA}),
			trustedClients:   testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[1].rootCA}),
			unTrustedClients: testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[1].rootCA}),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("Running test %s ...", test.name)
			testErr := runMutualAuth(t, test.servers, test.trustedClients, test.unTrustedClients)
			require.NoError(t, testErr)
		})
	}
}

func TestSetClientRootCAs(t *testing.T) {
	t.Parallel()

	// get the config for one of our Org1 test servers
	serverConfig := testOrgs[0].testServers([][]byte{})[0].config
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen failed")
	defer lis.Close()
	address := lis.Addr().String()

	// create a GRPCServer
	srv, err := comm.NewGRPCServerFromListener(lis, serverConfig)
	require.NoError(t, err, "failed to create GRPCServer")

	// register the GRPC test server and start the GRPCServer
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})
	go srv.Start()
	defer srv.Stop()

	// should not be needed but just in case
	time.Sleep(10 * time.Millisecond)

	// set up our test clients
	// Org1
	clientConfigOrg1Child1 := testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigOrg1Child2 := testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigsOrg1Children := []*tls.Config{clientConfigOrg1Child1, clientConfigOrg1Child2}
	org1ChildRootCAs := [][]byte{testOrgs[0].childOrgs[0].rootCA, testOrgs[0].childOrgs[1].rootCA}
	// Org2
	clientConfigOrg2Child1 := testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigOrg2Child2 := testOrgs[1].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigsOrg2Children := []*tls.Config{clientConfigOrg2Child1, clientConfigOrg2Child2}
	org2ChildRootCAs := [][]byte{testOrgs[1].childOrgs[0].rootCA, testOrgs[1].childOrgs[1].rootCA}

	// initially set client CAs to Org1 children
	err = srv.SetClientRootCAs(org1ChildRootCAs)
	require.NoError(t, err, "SetClientRootCAs failed")

	// clientConfigsOrg1Children are currently trusted
	for _, clientConfig := range clientConfigsOrg1Children {
		// we expect success as these are trusted clients
		_, err = invokeEmptyCall(address, grpc.WithTransportCredentials(credentials.NewTLS(clientConfig)))
		require.NoError(t, err, "trusted client should have connected")
	}

	// clientConfigsOrg2Children are currently not trusted
	for _, clientConfig := range clientConfigsOrg2Children {
		// we expect failure as these are now untrusted clients
		_, err = invokeEmptyCall(address, grpc.WithTransportCredentials(credentials.NewTLS(clientConfig)))
		require.Error(t, err, "untrusted client should not have been able to connect")
	}

	// now set client CAs to Org2 children
	err = srv.SetClientRootCAs(org2ChildRootCAs)
	require.NoError(t, err, "SetClientRootCAs failed")

	// now reverse trusted and not trusted
	// clientConfigsOrg1Children are currently trusted
	for _, clientConfig := range clientConfigsOrg2Children {
		// we expect success as these are trusted clients
		_, err = invokeEmptyCall(address, grpc.WithTransportCredentials(credentials.NewTLS(clientConfig)))
		require.NoError(t, err, "trusted client should have connected")
	}

	// clientConfigsOrg2Children are currently not trusted
	for _, clientConfig := range clientConfigsOrg1Children {
		// we expect failure as these are now untrusted clients
		_, err = invokeEmptyCall(address, grpc.WithTransportCredentials(credentials.NewTLS(clientConfig)))
		require.Error(t, err, "untrusted client should not have connected")
	}
}

func TestUpdateTLSCert(t *testing.T) {
	t.Parallel()

	readFile := func(path string) []byte {
		fName := filepath.Join("testdata", "dynamic_cert_update", path)
		data, err := ioutil.ReadFile(fName)
		if err != nil {
			panic(fmt.Errorf("Failed reading %s: %v", fName, err))
		}
		return data
	}
	loadBytes := func(prefix string) (key, cert, caCert []byte) {
		cert = readFile(filepath.Join(prefix, "server.crt"))
		key = readFile(filepath.Join(prefix, "server.key"))
		caCert = readFile(filepath.Join("ca.crt"))
		return
	}

	key, cert, caCert := loadBytes("notlocalhost")

	cfg := comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Key:         key,
			Certificate: cert,
		},
	}

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen failed")
	testAddress := lis.Addr().String()

	srv, err := comm.NewGRPCServerFromListener(lis, cfg)
	require.NoError(t, err)
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	go srv.Start()
	defer srv.Stop()

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)

	probeServer := func() error {
		_, err = invokeEmptyCall(
			testAddress,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: certPool})),
			grpc.WithBlock(),
		)
		return err
	}

	// bootstrap TLS certificate has a SAN of "notlocalhost" so it should fail
	err = probeServer()
	require.Error(t, err)
	require.Contains(t, err.Error(), "context deadline exceeded")

	// new TLS certificate has a SAN of "127.0.0.1" so it should succeed
	certPath := filepath.Join("testdata", "dynamic_cert_update", "localhost", "server.crt")
	keyPath := filepath.Join("testdata", "dynamic_cert_update", "localhost", "server.key")
	tlsCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	srv.SetServerCertificate(tlsCert)
	err = probeServer()
	require.NoError(t, err)

	// revert back to the old certificate, should fail.
	certPath = filepath.Join("testdata", "dynamic_cert_update", "notlocalhost", "server.crt")
	keyPath = filepath.Join("testdata", "dynamic_cert_update", "notlocalhost", "server.key")
	tlsCert, err = tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	srv.SetServerCertificate(tlsCert)

	err = probeServer()
	require.Error(t, err)
	require.Contains(t, err.Error(), "context deadline exceeded")
}

func TestCipherSuites(t *testing.T) {
	t.Parallel()

	certPEM, err := ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-server1-cert.pem"))
	require.NoError(t, err)
	keyPEM, err := ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-server1-key.pem"))
	require.NoError(t, err)
	caPEM, err := ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	require.NoError(t, err)
	certPool, err := createCertPool([][]byte{caPEM})
	require.NoError(t, err)

	serverConfig := comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Certificate: certPEM,
			Key:         keyPEM,
			UseTLS:      true,
		},
	}

	fabricDefaultCipherSuite := func(cipher uint16) bool {
		for _, defaultCipher := range comm.DefaultTLSCipherSuites {
			if cipher == defaultCipher {
				return true
			}
		}
		return false
	}

	var otherCipherSuites []uint16
	for _, cipher := range append(tls.CipherSuites(), tls.InsecureCipherSuites()...) {
		if !fabricDefaultCipherSuite(cipher.ID) {
			otherCipherSuites = append(otherCipherSuites, cipher.ID)
		}
	}

	tests := []struct {
		name          string
		clientCiphers []uint16
		success       bool
		versions      []uint16
	}{
		{
			name:     "server default / client all",
			success:  true,
			versions: []uint16{tls.VersionTLS12, tls.VersionTLS13},
		},
		{
			name:          "server default / client match",
			clientCiphers: comm.DefaultTLSCipherSuites,
			success:       true,
			// Skip TLS1.3 as it ignores the Fabric DefaultCipherSuites
			// https://github.com/golang/go/issues/29349
			versions: []uint16{tls.VersionTLS12},
		},
		{
			name:          "server default / client no match",
			clientCiphers: otherCipherSuites,
			success:       false,
			// Skip TLS1.3 as it ignores the Fabric DefaultCipherSuites
			// https://github.com/golang/go/issues/29349
			versions: []uint16{tls.VersionTLS12},
		},
	}

	// create our listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen failed")
	testAddress := lis.Addr().String()
	srv, err := comm.NewGRPCServerFromListener(lis, serverConfig)
	require.NoError(t, err)
	go srv.Start()

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, tlsVersion := range test.versions {
				tlsConfig := &tls.Config{
					RootCAs:      certPool,
					CipherSuites: test.clientCiphers,
					MinVersion:   tlsVersion,
					MaxVersion:   tlsVersion,
				}
				_, err := tls.Dial("tcp", testAddress, tlsConfig)
				if test.success {
					require.NoError(t, err)
				} else {
					require.Error(t, err, "expected handshake failure")
					require.Contains(t, err.Error(), "handshake failure")
				}
			}
		})
	}
}

func TestServerInterceptors(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen failed")
	msg := "error from interceptor"

	// set up interceptors
	usiCount := uint32(0)
	ssiCount := uint32(0)
	usi1 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		atomic.AddUint32(&usiCount, 1)
		return handler(ctx, req)
	}
	usi2 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		atomic.AddUint32(&usiCount, 1)
		return nil, status.Error(codes.Aborted, msg)
	}
	ssi1 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		atomic.AddUint32(&ssiCount, 1)
		return handler(srv, ss)
	}
	ssi2 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		atomic.AddUint32(&ssiCount, 1)
		return status.Error(codes.Aborted, msg)
	}

	srvConfig := comm.ServerConfig{}
	srvConfig.UnaryInterceptors = append(srvConfig.UnaryInterceptors, usi1)
	srvConfig.UnaryInterceptors = append(srvConfig.UnaryInterceptors, usi2)
	srvConfig.StreamInterceptors = append(srvConfig.StreamInterceptors, ssi1)
	srvConfig.StreamInterceptors = append(srvConfig.StreamInterceptors, ssi2)

	srv, err := comm.NewGRPCServerFromListener(lis, srvConfig)
	require.NoError(t, err, "failed to create gRPC server")
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})
	defer srv.Stop()
	go srv.Start()

	_, err = invokeEmptyCall(
		lis.Addr().String(),
		grpc.WithBlock(),
		grpc.WithInsecure(),
	)
	require.Error(t, err)
	require.Equal(t, status.Convert(err).Message(), msg, "Expected error from second usi")
	require.Equal(t, uint32(2), atomic.LoadUint32(&usiCount), "Expected both usi handlers to be invoked")

	_, err = invokeEmptyStream(
		lis.Addr().String(),
		grpc.WithBlock(),
		grpc.WithInsecure(),
	)
	require.Error(t, err)
	require.Equal(t, status.Convert(err).Message(), msg, "Expected error from second ssi")
	require.Equal(t, uint32(2), atomic.LoadUint32(&ssiCount), "Expected both ssi handlers to be invoked")
}
