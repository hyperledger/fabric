/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package peer_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/hyperledger/fabric/core/comm"
	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/stretchr/testify/assert"
)

// default timeout for grpc connections
var timeout = time.Second * 1

// test server to be registered with the GRPCServer
type testServiceServer struct{}

func (tss *testServiceServer) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	return new(testpb.Empty), nil
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

// helper function to invoke the EmptyCall againt the test service
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

func TestCreatePeerServer(t *testing.T) {

	t.Parallel()

	// load test certs from testdata
	org1CA, err := ioutil.ReadFile(filepath.Join("testdata", "Org1-cert.pem"))
	org1Server1Key, err := ioutil.ReadFile(filepath.Join("testdata", "Org1-server1-key.pem"))
	org1Server1Cert, err := ioutil.ReadFile(filepath.Join("testdata", "Org1-server1-cert.pem"))
	org2CA, err := ioutil.ReadFile(filepath.Join("testdata", "Org2-cert.pem"))
	org2Server1Key, err := ioutil.ReadFile(filepath.Join("testdata", "Org2-server1-key.pem"))
	org2Server1Cert, err := ioutil.ReadFile(filepath.Join("testdata", "Org2-server1-cert.pem"))

	if err != nil {
		t.Fatalf("Failed to load test certificates: %v", err)
	}

	org1CertPool, err := createCertPool([][]byte{org1CA})
	org2CertPool, err := createCertPool([][]byte{org2CA})

	if err != nil {
		t.Fatalf("Failed to load root certificates into pool: %v", err)
	}

	org1Creds := credentials.NewClientTLSFromCert(org1CertPool, "")
	org2Creds := credentials.NewClientTLSFromCert(org2CertPool, "")

	// use server cert as client cert
	org2ClientCert, err := tls.X509KeyPair(org2Server1Cert, org2Server1Key)
	if err != nil {
		t.Fatalf("Failed to load client certificate: %v", err)
	}
	org1Org2Creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{org2ClientCert},
		RootCAs:      org1CertPool,
	})

	// basic function tests
	var tests = []struct {
		name          string
		listenAddress string
		secureConfig  comm.SecureServerConfig
		expectError   bool
		goodOptions   []grpc.DialOption
		badOptions    []grpc.DialOption
	}{
		{
			name:          "NoTLS",
			listenAddress: fmt.Sprintf("localhost:%d", 4050),
			secureConfig: comm.SecureServerConfig{
				UseTLS: false,
			},
			expectError: false,
			goodOptions: []grpc.DialOption{grpc.WithInsecure()},
			badOptions:  []grpc.DialOption{grpc.WithTransportCredentials(org1Creds)},
		},
		{
			name:          "BadAddress",
			listenAddress: "badaddress",
			secureConfig: comm.SecureServerConfig{
				UseTLS: false,
			},
			expectError: true,
		},
		{
			name:          "ServerTLSOrg1",
			listenAddress: fmt.Sprintf("localhost:%d", 4051),
			secureConfig: comm.SecureServerConfig{
				UseTLS:            true,
				ServerCertificate: org1Server1Cert,
				ServerKey:         org1Server1Key,
				ServerRootCAs:     [][]byte{org1CA},
			},
			expectError: false,
			goodOptions: []grpc.DialOption{grpc.WithTransportCredentials(org1Creds)},
			badOptions:  []grpc.DialOption{grpc.WithTransportCredentials(org2Creds)},
		},
		{
			name:          "MutualTLSOrg1Org2",
			listenAddress: fmt.Sprintf("localhost:%d", 4052),
			secureConfig: comm.SecureServerConfig{
				UseTLS:            true,
				ServerCertificate: org1Server1Cert,
				ServerKey:         org1Server1Key,
				ServerRootCAs:     [][]byte{org1CA},
				ClientRootCAs:     [][]byte{org1CA, org2CA},
				RequireClientCert: true,
			},
			expectError: false,
			goodOptions: []grpc.DialOption{grpc.WithTransportCredentials(org1Org2Creds)},
			badOptions:  []grpc.DialOption{grpc.WithTransportCredentials(org1Creds)},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("Running test %s ...", test.name)

			_, err := peer.CreatePeerServer(test.listenAddress, test.secureConfig)
			// check to see whether to not we expect an error
			// we don't check the exact error because the comm package covers these cases
			if test.expectError {
				assert.Error(t, err, "CreatePeerServer should have returned an error")
			} else {
				assert.NoError(t, err, "CreatePeerServer should not have returned an error")
				// get the server from peer
				peerServer := peer.GetPeerServer()
				assert.NotNil(t, peerServer, "GetPeerServer should not return a nil value")
				// register a GRPC test service
				testpb.RegisterTestServiceServer(peerServer.Server(), &testServiceServer{})
				go peerServer.Start()
				defer peerServer.Stop()

				//invoke the EmptyCall service with good options
				_, err = invokeEmptyCall(test.listenAddress, test.goodOptions)
				assert.NoError(t, err, "Failed to invoke the EmptyCall service")
				//invoke the EmptyCall service with bad options
				_, err = invokeEmptyCall(test.listenAddress, test.badOptions)
				assert.Error(t, err, "Expected error using bad dial options")

			}
		})
	}
}
