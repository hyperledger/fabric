/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package comm_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

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
var timeout = time.Second * 3

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

func TestNewGRPCServerInvalidParameters(t *testing.T) {

	//missing address
	_, err := comm.NewGRPCServer("", nil, nil, nil, nil)
	//check for error
	msg := "Missing address parameter"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing port
	_, err = comm.NewGRPCServer("abcdef", nil, nil, nil, nil)
	//check for error
	msg = "listen tcp: missing port in address abcdef"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad port
	_, err = comm.NewGRPCServer("localhost:1BBB", nil, nil, nil, nil)
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
	_, err = comm.NewGRPCServer("hostdoesnotexist.localdomain:9050", nil, nil, nil, nil)
	//check for error only - there are a few possibilities depending on DNS resolution but will get an error
	assert.Error(t, err, "%s error expected", msg)
	if err != nil {
		t.Log(err.Error())
	}

	//address in use
	_, err = comm.NewGRPCServer(":9040", nil, nil, nil, nil)
	_, err = comm.NewGRPCServer(":9040", nil, nil, nil, nil)
	//check for error
	msg = "listen tcp :9040: bind: address already in use"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing serverCertificate
	_, err = comm.NewGRPCServer(":9041", []byte{}, nil, nil, nil)
	//check for error
	msg = "Both serverKey and serverCertificate are required in order to enable TLS"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//missing serverKey
	_, err = comm.NewGRPCServer(":9042", nil, []byte{}, nil, nil)
	//check for error
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad serverKey
	_, err = comm.NewGRPCServer(":9043", []byte{}, []byte(selfSignedCertPEM), nil, nil)
	//check for error
	msg = "tls: failed to find any PEM data in key input"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}

	//bad serverCertificate
	_, err = comm.NewGRPCServer(":9044", []byte(selfSignedKeyPEM), []byte{}, nil, nil)
	//check for error
	msg = "tls: failed to find any PEM data in certificate input"
	assert.EqualError(t, err, msg)
	if err != nil {
		t.Log(err.Error())
	}
}

func TestNewGRPCServer(t *testing.T) {

	testAddress := "localhost:9053"
	srv, err := comm.NewGRPCServer(testAddress, nil, nil, nil, nil)
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

	testAddress := "localhost:9054"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis, nil, nil, nil, nil)
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

	testAddress := "localhost:9055"
	srv, err := comm.NewGRPCServer(testAddress, []byte(selfSignedKeyPEM),
		[]byte(selfSignedCertPEM), nil, nil)
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

func TestNewSecureGRPCServerFromListener(t *testing.T) {

	testAddress := "localhost:9056"
	//create our listener
	lis, err := net.Listen("tcp", testAddress)

	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	srv, err := comm.NewGRPCServerFromListener(lis, []byte(selfSignedKeyPEM),
		[]byte(selfSignedCertPEM), nil, nil)
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

	srv, err := comm.NewGRPCServerFromListener(lis, keyPEMBlock,
		certPEMBlock, nil, nil)
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

	//create the client credentials
	certPoolServer := x509.NewCertPool()

	//use the server certificate only
	if !certPoolServer.AppendCertsFromPEM(certPEMBlock) {
		t.Fatal("Failed to append certificate to client credentials")
	}

	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	//client should not be able to connect
	//for now we can only test that we get a timeout error
	assert.EqualError(t, err, grpc.ErrClientConnTimeout.Error())
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

	srv, err := comm.NewGRPCServerFromListener(lis, keyPEMBlock,
		certPEMBlock, nil, nil)
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

	//create the client credentials
	certPoolServer := x509.NewCertPool()

	//use the server certificate only
	if !certPoolServer.AppendCertsFromPEM(certPEMBlock) {
		t.Fatal("Failed to append certificate to client credentials")
	}

	creds := credentials.NewClientTLSFromCert(certPoolServer, "")

	//GRPC client options
	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))

	//invoke the EmptyCall service
	_, err = invokeEmptyCall(testAddress, dialOptions)

	//client should not be able to connect
	//for now we can only test that we get a timeout error
	assert.EqualError(t, err, grpc.ErrClientConnTimeout.Error())
	t.Logf("assert.EqualError: %s", err.Error())

	//now use the CA certificate
	certPoolCA := x509.NewCertPool()
	if !certPoolCA.AppendCertsFromPEM(intermediatePEMBlock) {
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
