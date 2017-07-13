/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/transport"

	"github.com/hyperledger/fabric/core/comm"
	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
)

//Embedded certificates for testing
//These are the prime256v1-openssl-*.pem in testdata
//The self-signed cert expires in 2026
var selfSignedKeyPEM = `-----BEGIN EC PARAMETERS-----
BggqhkjOPQMBBw==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIM2rUTflEQ11m5g5yEm2Cer2yI+ziccl1NbSRVh3GUR0oAoGCCqGSM49
AwEHoUQDQgAEu2FEZVSr30Afey6dwcypeg5P+BuYx5JSYdG0/KJIBjWKnzYo7FEm
gMir7GbNh4pqA8KFrJZkPuxMgnEJBZTv+w==
-----END EC PRIVATE KEY-----
`
var selfSignedCertPEM = `-----BEGIN CERTIFICATE-----
MIICRDCCAemgAwIBAgIJALwW//dz2ZBvMAoGCCqGSM49BAMCMH4xCzAJBgNVBAYT
AlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRYwFAYDVQQHDA1TYW4gRnJhbmNpc2Nv
MRgwFgYDVQQKDA9MaW51eEZvdW5kYXRpb24xFDASBgNVBAsMC0h5cGVybGVkZ2Vy
MRIwEAYDVQQDDAlsb2NhbGhvc3QwHhcNMTYxMjA0MjIzMDE4WhcNMjYxMjAyMjIz
MDE4WjB+MQswCQYDVQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UE
BwwNU2FuIEZyYW5jaXNjbzEYMBYGA1UECgwPTGludXhGb3VuZGF0aW9uMRQwEgYD
VQQLDAtIeXBlcmxlZGdlcjESMBAGA1UEAwwJbG9jYWxob3N0MFkwEwYHKoZIzj0C
AQYIKoZIzj0DAQcDQgAEu2FEZVSr30Afey6dwcypeg5P+BuYx5JSYdG0/KJIBjWK
nzYo7FEmgMir7GbNh4pqA8KFrJZkPuxMgnEJBZTv+6NQME4wHQYDVR0OBBYEFAWO
4bfTEr2R6VYzQYrGk/2VWmtYMB8GA1UdIwQYMBaAFAWO4bfTEr2R6VYzQYrGk/2V
WmtYMAwGA1UdEwQFMAMBAf8wCgYIKoZIzj0EAwIDSQAwRgIhAIelqGdxPMHmQqRF
zA85vv7JhfMkvZYGPELC7I2K8V7ZAiEA9KcthV3HtDXKNDsA6ULT+qUkyoHRzCzr
A4QaL2VU6i4=
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

var pemNoCertificateHeader = `-----BEGIN NOCERT-----
MIICRDCCAemgAwIBAgIJALwW//dz2ZBvMAoGCCqGSM49BAMCMH4xCzAJBgNVBAYT
AlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRYwFAYDVQQHDA1TYW4gRnJhbmNpc2Nv
MRgwFgYDVQQKDA9MaW51eEZvdW5kYXRpb24xFDASBgNVBAsMC0h5cGVybGVkZ2Vy
MRIwEAYDVQQDDAlsb2NhbGhvc3QwHhcNMTYxMjA0MjIzMDE4WhcNMjYxMjAyMjIz
MDE4WjB+MQswCQYDVQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UE
BwwNU2FuIEZyYW5jaXNjbzEYMBYGA1UECgwPTGludXhGb3VuZGF0aW9uMRQwEgYD
VQQLDAtIeXBlcmxlZGdlcjESMBAGA1UEAwwJbG9jYWxob3N0MFkwEwYHKoZIzj0C
AQYIKoZIzj0DAQcDQgAEu2FEZVSr30Afey6dwcypeg5P+BuYx5JSYdG0/KJIBjWK
nzYo7FEmgMir7GbNh4pqA8KFrJZkPuxMgnEJBZTv+6NQME4wHQYDVR0OBBYEFAWO
4bfTEr2R6VYzQYrGk/2VWmtYMB8GA1UdIwQYMBaAFAWO4bfTEr2R6VYzQYrGk/2V
WmtYMAwGA1UdEwQFMAMBAf8wCgYIKoZIzj0EAwIDSQAwRgIhAIelqGdxPMHmQqRF
zA85vv7JhfMkvZYGPELC7I2K8V7ZAiEA9KcthV3HtDXKNDsA6ULT+qUkyoHRzCzr
A4QaL2VU6i4=
-----END NOCERT-----
`

var timeout = time.Second * 1
var testOrgs = []testOrg{}

func init() {
	//load up crypto material for test orgs
	for i := 1; i <= numOrgs; i++ {
		testOrg, err := loadOrg(i)
		if err != nil {
			log.Fatalf("Failed to load test organizations due to error: %s", err.Error())
		}
		testOrgs = append(testOrgs, testOrg)
	}
}

//test server to be registered with the GRPCServer
type testServiceServer struct{}

func (tss *testServiceServer) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	return new(testpb.Empty), nil
}

//invoke the EmptyCall RPC
func invokeEmptyCall(address string, dialOptions []grpc.DialOption) (*testpb.Empty, error) {

	//add DialOptions
	dialOptions = append(dialOptions, grpc.WithBlock())
	dialOptions = append(dialOptions, grpc.WithTimeout(timeout))
	//create GRPC client conn
	clientConn, err := grpc.Dial(address, dialOptions...)
	if err != nil {
		return nil, err
	}
	defer clientConn.Close()

	//create GRPC client
	client := testpb.NewTestServiceClient(clientConn)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//invoke service
	empty, err := client.EmptyCall(ctx, new(testpb.Empty))
	if err != nil {
		return nil, err
	}

	return empty, nil
}

const (
	numOrgs        = 2
	numChildOrgs   = 2
	numClientCerts = 2
	numServerCerts = 2
)

//string for cert filenames
var (
	orgCAKey        = filepath.Join("testdata", "certs", "Org%d-key.pem")
	orgCACert       = filepath.Join("testdata", "certs", "Org%d-cert.pem")
	orgServerKey    = filepath.Join("testdata", "certs", "Org%d-server%d-key.pem")
	orgServerCert   = filepath.Join("testdata", "certs", "Org%d-server%d-cert.pem")
	orgClientKey    = filepath.Join("testdata", "certs", "Org%d-client%d-key.pem")
	orgClientCert   = filepath.Join("testdata", "certs", "Org%d-client%d-cert.pem")
	childCAKey      = filepath.Join("testdata", "certs", "Org%d-child%d-key.pem")
	childCACert     = filepath.Join("testdata", "certs", "Org%d-child%d-cert.pem")
	childServerKey  = filepath.Join("testdata", "certs", "Org%d-child%d-server%d-key.pem")
	childServerCert = filepath.Join("testdata", "certs", "Org%d-child%d-server%d-cert.pem")
	childClientKey  = filepath.Join("testdata", "certs", "Org%d-child%d-client%d-key.pem")
	childClientCert = filepath.Join("testdata", "certs", "Org%d-child%d-client%d-cert.pem")
)

type testServer struct {
	address string
	config  comm.SecureServerConfig
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

//return *X509.CertPool for the rootCA of the org
func (org *testOrg) rootCertPool() *x509.CertPool {
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(org.rootCA)
	return certPool
}

//return testServers for the org
func (org *testOrg) testServers(port int, clientRootCAs [][]byte) []testServer {

	var testServers = []testServer{}
	clientRootCAs = append(clientRootCAs, org.rootCA)
	//loop through the serverCerts and create testServers
	for i, serverCert := range org.serverCerts {
		testServer := testServer{
			fmt.Sprintf("localhost:%d", port+i),
			comm.SecureServerConfig{
				UseTLS:            true,
				ServerCertificate: serverCert.certPEM,
				ServerKey:         serverCert.keyPEM,
				RequireClientCert: true,
				ClientRootCAs:     clientRootCAs,
			},
		}
		testServers = append(testServers, testServer)
	}
	return testServers
}

//return trusted clients for the org
func (org *testOrg) trustedClients(serverRootCAs [][]byte) []*tls.Config {

	var trustedClients = []*tls.Config{}
	//if we have any additional server root CAs add them to the certPool
	certPool := org.rootCertPool()
	for _, serverRootCA := range serverRootCAs {
		certPool.AppendCertsFromPEM(serverRootCA)
	}

	//loop through the clientCerts and create tls.Configs
	for _, clientCert := range org.clientCerts {
		trustedClient := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      certPool,
		}
		trustedClients = append(trustedClients, trustedClient)
	}
	return trustedClients
}

//createCertPool creates an x509.CertPool from an array of PEM-encoded certificates
func createCertPool(rootCAs [][]byte) (*x509.CertPool, error) {

	certPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		if !certPool.AppendCertsFromPEM(rootCA) {
			return nil, errors.New("Failed to load root certificates")
		}
	}
	return certPool, nil
}

//utility function to load crypto material for organizations
func loadOrg(parent int) (testOrg, error) {

	var org = testOrg{}
	//load the CA
	caPEM, err := ioutil.ReadFile(fmt.Sprintf(orgCACert, parent))
	if err != nil {
		return org, err
	}
	//loop through and load servers
	var serverCerts = []serverCert{}
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
	//loop through and load clients
	var clientCerts = []tls.Certificate{}
	for j := 1; j <= numServerCerts; j++ {
		clientCert, err := loadTLSKeyPairFromFile(fmt.Sprintf(orgClientKey, parent, j),
			fmt.Sprintf(orgClientCert, parent, j))
		if err != nil {
			return org, err
		}
		clientCerts = append(clientCerts, clientCert)
	}
	//loop through and load child orgs
	var childOrgs = []testOrg{}

	for k := 1; k <= numChildOrgs; k++ {
		childOrg, err := loadChildOrg(parent, k)
		if err != nil {
			return org, err
		}
		childOrgs = append(childOrgs, childOrg)
	}

	return testOrg{caPEM, serverCerts, clientCerts, childOrgs}, nil
}

//utility function to load crypto material for child organizations
func loadChildOrg(parent, child int) (testOrg, error) {

	var org = testOrg{}
	//load the CA
	caPEM, err := ioutil.ReadFile(fmt.Sprintf(childCACert, parent, child))
	if err != nil {
		return org, err
	}
	//loop through and load servers
	var serverCerts = []serverCert{}
	for i := 1; i <= numServerCerts; i++ {
		keyPEM, err := ioutil.ReadFile(fmt.Sprintf(childServerKey, parent, child, i))
		if err != nil {
			return org, err
		}
		certPEM, err := ioutil.ReadFile(fmt.Sprintf(childServerCert, parent, child, i))
		if err != nil {
			return org, err
		}
		serverCerts = append(serverCerts, serverCert{keyPEM, certPEM})
	}
	//loop through and load clients
	var clientCerts = []tls.Certificate{}
	for j := 1; j <= numServerCerts; j++ {
		clientCert, err := loadTLSKeyPairFromFile(fmt.Sprintf(childClientKey, parent, child, j),
			fmt.Sprintf(childClientCert, parent, child, j))
		if err != nil {
			return org, err
		}
		clientCerts = append(clientCerts, clientCert)
	}
	return testOrg{caPEM, serverCerts, clientCerts, []testOrg{}}, nil
}

//loadTLSKeyPairFromFile creates a tls.Certificate from PEM-encoded key and cert files
func loadTLSKeyPairFromFile(keyFile, certFile string) (tls.Certificate, error) {

	certPEMBlock, err := ioutil.ReadFile(certFile)
	keyPEMBlock, err := ioutil.ReadFile(keyFile)
	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)

	if err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

func TestNewGRPCServerInvalidParameters(t *testing.T) {

	t.Parallel()
	//missing address
	_, err := comm.NewGRPCServer("", comm.SecureServerConfig{UseTLS: false})
	//check for error
	msg := "Missing address parameter"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing port
	_, err = comm.NewGRPCServer("abcdef", comm.SecureServerConfig{UseTLS: false})
	//check for error
	msg = "listen tcp: missing port in address abcdef"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad port
	_, err = comm.NewGRPCServer("localhost:1BBB", comm.SecureServerConfig{UseTLS: false})
	//check for error
	msgs := [2]string{"listen tcp: lookup tcp/1BBB: nodename nor servname provided, or not known",
		"listen tcp: unknown port tcp/1BBB"} //different error on MacOS and in Docker

	if assert.Error(t, err, "%s or %s expected", msgs[0], msgs[1]) {
		assert.Contains(t, msgs, err.Error())
	}
	if err != nil {
		t.Log(err.Error())
	}

	//bad hostname
	_, err = comm.NewGRPCServer("hostdoesnotexist.localdomain:9050",
		comm.SecureServerConfig{UseTLS: false})
	/*
		We cannot check for a specific error message due to the fact that some
		systems will automatically resolve unknown host names to a "search"
		address so we just check to make sure that an error was returned
	*/
	assert.Error(t, err, "%s error expected", msg)
	if err != nil {
		t.Log(err.Error())
	}

	//address in use
	_, err = comm.NewGRPCServer(":9040", comm.SecureServerConfig{UseTLS: false})
	_, err = comm.NewGRPCServer(":9040", comm.SecureServerConfig{UseTLS: false})
	//check for error
	msg = "listen tcp :9040: bind: address already in use"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing serverCertificate
	_, err = comm.NewGRPCServer(":9041",
		comm.SecureServerConfig{UseTLS: true, ServerCertificate: []byte{}})
	//check for error
	msg = "secureConfig must contain both ServerKey and " +
		"ServerCertificate when UseTLS is true"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing serverKey
	_, err = comm.NewGRPCServer(":9042",
		comm.SecureServerConfig{UseTLS: true, ServerKey: []byte{}})
	//check for error
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad serverKey
	_, err = comm.NewGRPCServer(":9043",
		comm.SecureServerConfig{
			UseTLS:            true,
			ServerCertificate: []byte(selfSignedCertPEM),
			ServerKey:         []byte{}})

	//check for error
	msg = "tls: failed to find any PEM data in key input"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad serverCertificate
	_, err = comm.NewGRPCServer(":9044",
		comm.SecureServerConfig{
			UseTLS:            true,
			ServerCertificate: []byte{},
			ServerKey:         []byte(selfSignedKeyPEM)})
	//check for error
	msg = "tls: failed to find any PEM data in certificate input"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad clientRootCAs
	/** TODO: revisit after figuring out why MSP does not serialize PEMs with type
	_, err = comm.NewGRPCServer(":9045",
		comm.SecureServerConfig{
			UseTLS:            true,
			ServerCertificate: []byte(selfSignedCertPEM),
			ServerKey:         []byte(selfSignedKeyPEM),
			RequireClientCert: true,
			ClientRootCAs:     [][]byte{[]byte(pemNoCertificateHeader)}})
	//check for error
	msg = "Failed to append client root certificate(s): " +
		"No client root certificates found"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}
	*/

	srv, err := comm.NewGRPCServer(":9046",
		comm.SecureServerConfig{
			UseTLS:            true,
			ServerCertificate: []byte(selfSignedCertPEM),
			ServerKey:         []byte(selfSignedKeyPEM),
			RequireClientCert: true})
	badRootCAs := [][]byte{[]byte(badPEM)}
	err = srv.SetClientRootCAs(badRootCAs)
	//check for error
	msg = "Failed to set client root certificate(s): " +
		"asn1: syntax error: data truncated"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}
}

func TestNewGRPCServer(t *testing.T) {

	t.Parallel()
	testAddress := "localhost:9053"
	srv, err := comm.NewGRPCServer(testAddress,
		comm.SecureServerConfig{UseTLS: false})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//make sure our properties are as expected
	//resolve the address
	addr, err := net.ResolveTCPAddr("tcp", testAddress)
	assert.Equal(t, srv.Address(), addr.String())
	assert.Equal(t, srv.Listener().Addr().String(), addr.String())

	//TlSEnabled should be false
	assert.Equal(t, srv.TLSEnabled(), false)

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithInsecure())

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	if err != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}

}

func TestNewGRPCServerFromListener(t *testing.T) {

	t.Parallel()
	testAddress := "localhost:9054"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis,
		comm.SecureServerConfig{UseTLS: false})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//make sure our properties are as expected
	//resolve the address
	addr, err := net.ResolveTCPAddr("tcp", testAddress)
	assert.Equal(t, srv.Address(), addr.String())
	assert.Equal(t, srv.Listener().Addr().String(), addr.String())

	//TlSEnabled should be false
	assert.Equal(t, srv.TLSEnabled(), false)

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithInsecure())

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	if err != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}
}

func TestNewSecureGRPCServer(t *testing.T) {

	t.Parallel()
	testAddress := "localhost:9055"
	srv, err := comm.NewGRPCServer(testAddress, comm.SecureServerConfig{
		UseTLS:            true,
		ServerCertificate: []byte(selfSignedCertPEM),
		ServerKey:         []byte(selfSignedKeyPEM),
	})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//make sure our properties are as expected
	//resolve the address
	addr, err := net.ResolveTCPAddr("tcp", testAddress)
	assert.Equal(t, srv.Address(), addr.String())
	assert.Equal(t, srv.Listener().Addr().String(), addr.String())

	//check the server certificate
	cert, _ := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	assert.Equal(t, srv.ServerCertificate(), cert)

	//TlSEnabled should be true
	assert.Equal(t, srv.TLSEnabled(), true)

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//create the client credentials
	certPool := x509.NewCertPool()

	if !certPool.AppendCertsFromPEM([]byte(selfSignedCertPEM)) {

		t.Fatal("Failed to append certificate to client credentials")
	}

	creds := credentials.NewClientTLSFromCert(certPool, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	if err != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}

	// ensure that TLS 1.2 in required / enforced
	for _, tlsVersion := range []uint16{tls.VersionSSL30, tls.VersionTLS10, tls.VersionTLS11} {
		_, err = invokeEmptyCall(testAddress,
			[]grpc.DialOption{grpc.WithTransportCredentials(
				credentials.NewTLS(&tls.Config{
					RootCAs:    certPool,
					MinVersion: tlsVersion,
					MaxVersion: tlsVersion,
				}))})
		t.Logf("TLSVersion [%d] failed with [%s]", tlsVersion, err)
		assert.Error(t, err, "Should not have been able to connect with TLS version < 1.2")
		assert.Contains(t, err.Error(), "protocol version not supported")
	}
}

func TestNewSecureGRPCServerFromListener(t *testing.T) {

	t.Parallel()
	testAddress := "localhost:9056"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis, comm.SecureServerConfig{
		UseTLS:            true,
		ServerCertificate: []byte(selfSignedCertPEM),
		ServerKey:         []byte(selfSignedKeyPEM),
	})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//make sure our properties are as expected
	//resolve the address
	addr, err := net.ResolveTCPAddr("tcp", testAddress)
	assert.Equal(t, srv.Address(), addr.String())
	assert.Equal(t, srv.Listener().Addr().String(), addr.String())

	//check the server certificate
	cert, _ := tls.X509KeyPair([]byte(selfSignedCertPEM), []byte(selfSignedKeyPEM))
	assert.Equal(t, srv.ServerCertificate(), cert)

	//TlSEnabled should be true
	assert.Equal(t, srv.TLSEnabled(), true)

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//create the client credentials
	certPool := x509.NewCertPool()

	if !certPool.AppendCertsFromPEM([]byte(selfSignedCertPEM)) {

		t.Fatal("Failed to append certificate to client credentials")
	}

	creds := credentials.NewClientTLSFromCert(certPool, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	if err != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}
}

//prior tests used self-signed certficates loaded by the GRPCServer and the test client
//here we'll use certificates signed by certificate authorities
func TestWithSignedRootCertificates(t *testing.T) {

	t.Parallel()
	//use Org1 testdata
	fileBase := "Org1"
	certPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-cert.pem"))
	keyPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-key.pem"))
	caPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-cert.pem"))

	if err != nil {
		t.Fatalf("Failed to load test certificates: %v", err)
	}
	testAddress := "localhost:9057"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis, comm.SecureServerConfig{
		UseTLS:            true,
		ServerCertificate: certPEMBlock,
		ServerKey:         keyPEMBlock,
	})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//create a CertPool for use by the client with the server cert only
	certPoolServer, err := createCertPool([][]byte{certPEMBlock})
	if err != nil {
		t.Fatalf("Failed to load root certificates into pool: %v", err)
	}
	//create the client credentials
	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	//client should not be able to connect
	assert.EqualError(t, err, x509.UnknownAuthorityError{}.Error())
	t.Logf("assert.EqualError: %s", err.Error())

	//now use the CA certificate
	certPoolCA := x509.NewCertPool()
	if !certPoolCA.AppendCertsFromPEM(caPEMBlock) {
		t.Fatal("Failed to append certificate to client credentials")
	}
	creds = credentials.NewClientTLSFromCert(certPoolCA, "")
	var dialOptionsCA []grpc.DialOption
	dialOptionsCA = append(dialOptionsCA, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err2 := invokeEmptyCall(testAddress, dialOptionsCA)

	if err2 != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err2)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}
}

//here we'll use certificates signed by intermediate certificate authorities
func TestWithSignedIntermediateCertificates(t *testing.T) {

	t.Parallel()
	//use Org1 testdata
	fileBase := "Org1"
	certPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-server1-cert.pem"))
	keyPEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-server1-key.pem"))
	intermediatePEMBlock, err := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-child1-cert.pem"))

	if err != nil {
		t.Fatalf("Failed to load test certificates: %v", err)
	}
	testAddress := "localhost:9058"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis, comm.SecureServerConfig{
		UseTLS:            true,
		ServerCertificate: certPEMBlock,
		ServerKey:         keyPEMBlock,
	})
	//check for error
	if err != nil {
		t.Fatalf("Failed to return new GRPC server: %v", err)
	}

	//register the GRPC test server
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})

	//start the server
	go srv.Start()

	defer srv.Stop()
	//should not be needed
	time.Sleep(10 * time.Millisecond)

	//create a CertPool for use by the client with the server cert only
	certPoolServer, err := createCertPool([][]byte{certPEMBlock})
	if err != nil {
		t.Fatalf("Failed to load root certificates into pool: %v", err)
	}
	//create the client credentials
	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	//client should not be able to connect
	assert.EqualError(t, err, x509.UnknownAuthorityError{}.Error())
	t.Logf("assert.EqualError: %s", err.Error())

	//now use the CA certificate

	//create a CertPool for use by the client with the intermediate root CA
	certPoolCA, err := createCertPool([][]byte{intermediatePEMBlock})
	if err != nil {
		t.Fatalf("Failed to load root certificates into pool: %v", err)
	}

	creds = credentials.NewClientTLSFromCert(certPoolCA, "")
	var dialOptionsCA []grpc.DialOption
	dialOptionsCA = append(dialOptionsCA, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err2 := invokeEmptyCall(testAddress, dialOptionsCA)

	if err2 != nil {
		t.Fatalf("GRPC client failed to invoke the EmptyCall service on %s: %v",
			testAddress, err2)
	} else {
		t.Log("GRPC client successfully invoked the EmptyCall service: " + testAddress)
	}
}

//utility function for testing client / server communication using TLS
func runMutualAuth(t *testing.T, servers []testServer, trustedClients, unTrustedClients []*tls.Config) error {

	//loop through all the test servers
	for i := 0; i < len(servers); i++ {
		//create listener
		lis, err := net.Listen("tcp", servers[i].address)
		if err != nil {
			return err
		}

		//create GRPCServer
		srv, err := comm.NewGRPCServerFromListener(lis, servers[i].config)
		if err != nil {
			return err
		}

		//register the GRPC test server and start the GRPCServer
		testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})
		go srv.Start()
		defer srv.Stop()
		//should not be needed but just in case
		time.Sleep(10 * time.Millisecond)

		//loop through all the trusted clients
		for j := 0; j < len(trustedClients); j++ {
			//invoke the EmptyCall service
			_, err = invokeEmptyCall(servers[i].address,
				[]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(trustedClients[j]))})
			//we expect success from trusted clients
			if err != nil {
				return err
			} else {
				t.Logf("Trusted client%d successfully connected to %s", j, servers[i].address)
			}
		}
		//loop through all the untrusted clients
		for k := 0; k < len(unTrustedClients); k++ {
			//invoke the EmptyCall service
			_, err = invokeEmptyCall(servers[i].address, []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(unTrustedClients[k]))})
			//we expect failure from untrusted clients
			if err != nil {
				t.Logf("Untrusted client%d was correctly rejected by %s", k, servers[i].address)
			} else {
				return fmt.Errorf("Untrusted client %d should not have been able to connect to %s", k,
					servers[i].address)
			}
		}
	}

	return nil
}

func TestMutualAuth(t *testing.T) {

	t.Parallel()
	var tests = []struct {
		name             string
		servers          []testServer
		trustedClients   []*tls.Config
		unTrustedClients []*tls.Config
	}{
		{
			name:             "ClientAuthRequiredWithSingleOrg",
			servers:          testOrgs[0].testServers(9060, [][]byte{}),
			trustedClients:   testOrgs[0].trustedClients([][]byte{}),
			unTrustedClients: testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithChildClientOrg",
			servers:          testOrgs[0].testServers(9070, [][]byte{testOrgs[0].childOrgs[0].rootCA}),
			trustedClients:   testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA}),
			unTrustedClients: testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name: "ClientAuthRequiredWithMultipleChildClientOrgs",
			servers: testOrgs[0].testServers(9080, append([][]byte{},
				testOrgs[0].childOrgs[0].rootCA, testOrgs[0].childOrgs[1].rootCA)),
			trustedClients: append(append([]*tls.Config{},
				testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})...),
				testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})...),
			unTrustedClients: testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithDifferentServerAndClientOrgs",
			servers:          testOrgs[0].testServers(9090, [][]byte{testOrgs[1].rootCA}),
			trustedClients:   testOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
			unTrustedClients: testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA}),
		},
		{
			name:             "ClientAuthRequiredWithDifferentServerAndChildClientOrgs",
			servers:          testOrgs[1].testServers(9100, [][]byte{testOrgs[0].childOrgs[0].rootCA}),
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
			if testErr != nil {
				t.Fatalf("%s failed with error: %s", test.name, testErr.Error())
			}
		})
	}

}

func TestAppendRemoveWithInvalidBytes(t *testing.T) {

	// TODO: revisit when msp serialization without PEM type is resolved
	t.Skip()
	t.Parallel()

	noPEMData := [][]byte{[]byte("badcert1"), []byte("badCert2")}

	//get the config for one of our Org1 test servers
	serverConfig := testOrgs[0].testServers(9200, [][]byte{})[0].config
	address := testOrgs[0].testServers(9200, [][]byte{})[0].address

	//create a GRPCServer
	srv, err := comm.NewGRPCServer(address, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create GRPCServer due to: %s", err.Error())
	}

	//append/remove nonPEMData
	noCertsFound := "No client root certificates found"
	err = srv.AppendClientRootCAs(noPEMData)
	if err == nil {
		t.Fatalf("Expected error: %s", noCertsFound)
	}
	err = srv.RemoveClientRootCAs(noPEMData)
	if err == nil {
		t.Fatalf("Expected error: %s", noCertsFound)
	}

	//apend/remove PEM without CERTIFICATE header
	err = srv.AppendClientRootCAs([][]byte{[]byte(pemNoCertificateHeader)})
	if err == nil {
		t.Fatalf("Expected error: %s", noCertsFound)
	}

	err = srv.RemoveClientRootCAs([][]byte{[]byte(pemNoCertificateHeader)})
	if err == nil {
		t.Fatalf("Expected error: %s", noCertsFound)
	}

	//append/remove bad PEM data
	err = srv.AppendClientRootCAs([][]byte{[]byte(badPEM)})
	if err == nil {
		t.Fatalf("Expected error parsing bad PEM data")
	}

	err = srv.RemoveClientRootCAs([][]byte{[]byte(badPEM)})
	if err == nil {
		t.Fatalf("Expected error parsing bad PEM data")
	}

}

func TestAppendClientRootCAs(t *testing.T) {

	t.Parallel()
	//get the config for one of our Org1 test servers
	serverConfig := testOrgs[0].testServers(9300, [][]byte{})[0].config
	address := testOrgs[0].testServers(9300, [][]byte{})[0].address

	//create a GRPCServer
	srv, err := comm.NewGRPCServer(address, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create GRPCServer due to: %s", err.Error())
	}

	//register the GRPC test server and start the GRPCServer
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})
	go srv.Start()
	defer srv.Stop()
	//should not be needed but just in case
	time.Sleep(10 * time.Millisecond)

	//try to connect with untrusted clients from Org2 children
	clientConfig1 := testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfig2 := testOrgs[1].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigs := []*tls.Config{clientConfig1, clientConfig2}

	for i, clientConfig := range clientConfigs {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		//we expect failure as these are currently not trusted clients
		if err != nil {
			t.Logf("Untrusted client%d was correctly rejected by %s", i, address)
		} else {
			t.Fatalf("Untrusted client %d should not have been able to connect to %s", i,
				address)
		}
	}

	//now append the root CAs for the untrusted clients
	err = srv.AppendClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
		testOrgs[1].childOrgs[1].rootCA})
	if err != nil {
		t.Fatal("Failed to append client root CAs")
	}

	//now try to connect again
	for j, clientConfig := range clientConfigs {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		//we expect success as these are now trusted clients
		if err != nil {
			t.Fatalf("Now trusted client%d failed to connect to %s with error: %s",
				j, address, err.Error())
		} else {
			t.Logf("Now trusted client%d successfully connected to %s", j, address)
		}
	}

}

func TestRemoveClientRootCAs(t *testing.T) {

	t.Parallel()
	//get the config for one of our Org1 test servers and include client CAs from
	//Org2 child orgs
	serverConfig := testOrgs[0].testServers(9301,
		[][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})[0].config
	address := testOrgs[0].testServers(9301, [][]byte{})[0].address

	//create a GRPCServer
	srv, err := comm.NewGRPCServer(address, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create GRPCServer due to: %s", err.Error())
	}

	//register the GRPC test server and start the GRPCServer
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})
	go srv.Start()
	defer srv.Stop()
	//should not be needed but just in case
	time.Sleep(10 * time.Millisecond)

	//try to connect with trusted clients from Org2 children
	clientConfig1 := testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfig2 := testOrgs[1].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigs := []*tls.Config{clientConfig1, clientConfig2}

	for i, clientConfig := range clientConfigs {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})

		//we expect success as these are trusted clients
		if err != nil {
			t.Fatalf("Trusted client%d failed to connect to %s with error: %s",
				i, address, err.Error())
		} else {
			t.Logf("Trusted client%d successfully connected to %s", i, address)
		}
	}

	//now remove the root CAs for the untrusted clients
	err = srv.RemoveClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
		testOrgs[1].childOrgs[1].rootCA})
	if err != nil {
		t.Fatal("Failed to remove client root CAs")
	}

	//now try to connect again
	for j, clientConfig := range clientConfigs {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		//we expect failure as these are now untrusted clients
		if err != nil {
			t.Logf("Now untrusted client%d was correctly rejected by %s", j, address)
		} else {
			t.Fatalf("Now untrusted client %d should not have been able to connect to %s", j,
				address)
		}
	}

}

//test for race conditions - test locally using "go test -race -run TestConcurrentAppendRemoveSet"
func TestConcurrentAppendRemoveSet(t *testing.T) {

	t.Parallel()
	//get the config for one of our Org1 test servers and include client CAs from
	//Org2 child orgs
	serverConfig := testOrgs[0].testServers(9302,
		[][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})[0].config
	address := testOrgs[0].testServers(9302, [][]byte{})[0].address

	//create a GRPCServer
	srv, err := comm.NewGRPCServer(address, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create GRPCServer due to: %s", err.Error())
	}

	//register the GRPC test server and start the GRPCServer
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})
	go srv.Start()
	defer srv.Stop()

	//need to wait for the following go routines to finish
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		//now remove the root CAs for the untrusted clients
		err := srv.RemoveClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})
		if err != nil {
			t.Fatal("Failed to remove client root CAs")
		}

	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		//set client root CAs
		err := srv.SetClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})
		if err != nil {
			t.Fatal("Failed to set client root CAs")
		}

	}()

	//TODO: enable this after creating a custom type for grpc.TransportCredentials
	/*
		clientConfig := testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = invokeEmptyCall(address, []grpc.DialOption{
				grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		}()
	*/
	wg.Add(1)
	go func() {
		defer wg.Done()
		//now append the root CAs for the untrusted clients
		err := srv.AppendClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})
		if err != nil {
			t.Fatal("Failed to append client root CAs")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		//set client root CAs
		err := srv.SetClientRootCAs([][]byte{testOrgs[1].childOrgs[0].rootCA,
			testOrgs[1].childOrgs[1].rootCA})
		if err != nil {
			t.Fatal("Failed to set client root CAs")
		}

	}()

	wg.Wait()

}

func TestSetClientRootCAs(t *testing.T) {

	t.Parallel()

	//get the config for one of our Org1 test servers
	serverConfig := testOrgs[0].testServers(9303, [][]byte{})[0].config
	address := testOrgs[0].testServers(9303, [][]byte{})[0].address

	//create a GRPCServer
	srv, err := comm.NewGRPCServer(address, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create GRPCServer due to: %s", err.Error())
	}

	//register the GRPC test server and start the GRPCServer
	testpb.RegisterTestServiceServer(srv.Server(), &testServiceServer{})
	go srv.Start()
	defer srv.Stop()
	//should not be needed but just in case
	time.Sleep(10 * time.Millisecond)

	//set up out test clients
	//Org1
	clientConfigOrg1Child1 := testOrgs[0].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigOrg1Child2 := testOrgs[0].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigsOrg1Children := []*tls.Config{clientConfigOrg1Child1, clientConfigOrg1Child2}
	org1ChildRootCAs := [][]byte{testOrgs[0].childOrgs[0].rootCA,
		testOrgs[0].childOrgs[1].rootCA}
	//Org2
	clientConfigOrg2Child1 := testOrgs[1].childOrgs[0].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigOrg2Child2 := testOrgs[1].childOrgs[1].trustedClients([][]byte{testOrgs[0].rootCA})[0]
	clientConfigsOrg2Children := []*tls.Config{clientConfigOrg2Child1, clientConfigOrg2Child2}
	org2ChildRootCAs := [][]byte{testOrgs[1].childOrgs[0].rootCA,
		testOrgs[1].childOrgs[1].rootCA}

	//initially set client CAs to Org1 children
	err = srv.SetClientRootCAs(org1ChildRootCAs)
	if err != nil {
		t.Fatalf("SetClientRootCAs failed due to: %s", err.Error())
	}

	//clientConfigsOrg1Children are currently trusted
	for i, clientConfig := range clientConfigsOrg1Children {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})

		//we expect success as these are trusted clients
		if err != nil {
			t.Fatalf("Trusted client%d failed to connect to %s with error: %s",
				i, address, err.Error())
		} else {
			t.Logf("Trusted client%d successfully connected to %s", i, address)
		}
	}

	//clientConfigsOrg2Children are currently not trusted
	for j, clientConfig := range clientConfigsOrg2Children {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		//we expect failure as these are now untrusted clients
		if err != nil {
			t.Logf("Untrusted client%d was correctly rejected by %s", j, address)
		} else {
			t.Fatalf("Untrusted client %d should not have been able to connect to %s", j,
				address)
		}
	}

	//now set client CAs to Org2 children
	err = srv.SetClientRootCAs(org2ChildRootCAs)
	if err != nil {
		t.Fatalf("SetClientRootCAs failed due to: %s", err.Error())
	}

	//now reverse trusted and not trusted
	//clientConfigsOrg1Children are currently trusted
	for i, clientConfig := range clientConfigsOrg2Children {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})

		//we expect success as these are trusted clients
		if err != nil {
			t.Fatalf("Trusted client%d failed to connect to %s with error: %s",
				i, address, err.Error())
		} else {
			t.Logf("Trusted client%d successfully connected to %s", i, address)
		}
	}

	//clientConfigsOrg2Children are currently not trusted
	for j, clientConfig := range clientConfigsOrg1Children {
		//invoke the EmptyCall service
		_, err = invokeEmptyCall(address, []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientConfig))})
		//we expect failure as these are now untrusted clients
		if err != nil {
			t.Logf("Untrusted client%d was correctly rejected by %s", j, address)
		} else {
			t.Fatalf("Untrusted client %d should not have been able to connect to %s", j,
				address)
		}
	}

}

func TestKeepaliveNoClientResponse(t *testing.T) {
	t.Parallel()
	// set up GRPCServer instance
	kap := comm.KeepaliveOptions{
		ServerKeepaliveTime:    2,
		ServerKeepaliveTimeout: 1,
	}
	comm.SetKeepaliveOptions(kap)
	testAddress := "localhost:9400"
	srv, err := comm.NewGRPCServer(testAddress, comm.SecureServerConfig{})
	assert.NoError(t, err, "Unexpected error starting GRPCServer")
	go srv.Start()
	defer srv.Stop()

	// test connection close if client does not response to ping
	// net client will not response to keepalive
	client, err := net.Dial("tcp", testAddress)
	assert.NoError(t, err, "Unexpected error dialing GRPCServer")
	defer client.Close()
	// sleep past keepalive timeout
	time.Sleep(4 * time.Second)
	data := make([]byte, 24)
	for {
		_, err = client.Read(data)
		if err == nil {
			continue
		}
		assert.EqualError(t, err, io.EOF.Error(), "Expected io.EOF")
		break
	}
}

func TestKeepaliveClientResponse(t *testing.T) {
	t.Parallel()
	// set up GRPCServer instance
	kap := comm.KeepaliveOptions{
		ServerKeepaliveTime:    2,
		ServerKeepaliveTimeout: 1,
	}
	comm.SetKeepaliveOptions(kap)
	testAddress := "localhost:9401"
	srv, err := comm.NewGRPCServer(testAddress, comm.SecureServerConfig{})
	assert.NoError(t, err, "Unexpected error starting GRPCServer")
	go srv.Start()
	defer srv.Stop()

	// test that connection does not close with response to ping
	clientTransport, err := transport.NewClientTransport(context.Background(),
		transport.TargetInfo{Addr: testAddress}, transport.ConnectOptions{})
	assert.NoError(t, err, "Unexpected error creating client transport")
	defer clientTransport.Close()
	// sleep past keepalive timeout
	time.Sleep(4 * time.Second)
	// try to create a stream
	_, err = clientTransport.NewStream(context.Background(), &transport.CallHdr{})
	assert.NoError(t, err, "Unexpected error creating stream")
}
