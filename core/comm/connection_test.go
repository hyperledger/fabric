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

package comm

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/hyperledger/fabric/core/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	numOrgs      = 2
	numChildOrgs = 2
)

//string for cert filenames
var (
	orgCACert   = filepath.Join("testdata", "certs", "Org%d-cert.pem")
	childCACert = filepath.Join("testdata", "certs", "Org%d-child%d-cert.pem")
)

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

func TestConnection_Correct(t *testing.T) {
	testutil.SetupTestConfig()
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	peerAddress := GetPeerTestingAddress("7051")
	var tmpConn *grpc.ClientConn
	var err error
	if TLSEnabled() {
		tmpConn, err = NewClientConnectionWithAddress(peerAddress, true, true, InitTLSForPeer())
	}
	tmpConn, err = NewClientConnectionWithAddress(peerAddress, true, false, nil)
	if err != nil {
		t.Fatalf("error connection to server at host:port = %s\n", peerAddress)
	}

	tmpConn.Close()
}

func TestConnection_WrongAddress(t *testing.T) {
	testutil.SetupTestConfig()
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	//some random port
	peerAddress := GetPeerTestingAddress("10287")
	var tmpConn *grpc.ClientConn
	var err error
	if TLSEnabled() {
		tmpConn, err = NewClientConnectionWithAddress(peerAddress, true, true, InitTLSForPeer())
	}
	tmpConn, err = NewClientConnectionWithAddress(peerAddress, true, false, nil)
	if err == nil {
		fmt.Printf("error connection to server -  at host:port = %s\n", peerAddress)
		t.Error("error connection to server - connection should fail")
		tmpConn.Close()
	}
}

// utility function to load up our test root certificates from testdata/certs
func loadRootCAs() [][]byte {

	rootCAs := [][]byte{}
	for i := 1; i <= numOrgs; i++ {
		root, err := ioutil.ReadFile(fmt.Sprintf(orgCACert, i))
		if err != nil {
			return [][]byte{}
		}
		rootCAs = append(rootCAs, root)
		for j := 1; j <= numChildOrgs; j++ {
			root, err := ioutil.ReadFile(fmt.Sprintf(childCACert, i, j))
			if err != nil {
				return [][]byte{}
			}
			rootCAs = append(rootCAs, root)
		}
	}
	return rootCAs
}

func TestCASupport(t *testing.T) {

	rootCAs := loadRootCAs()
	t.Logf("loaded %d root certificates", len(rootCAs))
	if len(rootCAs) != 6 {
		t.Fatalf("failed to load root certificates")
	}

	cas := GetCASupport()
	cas.AppRootCAsByChain["channel1"] = [][]byte{rootCAs[0]}
	cas.AppRootCAsByChain["channel2"] = [][]byte{rootCAs[1]}
	cas.AppRootCAsByChain["channel3"] = [][]byte{rootCAs[2]}
	cas.OrdererRootCAsByChain["channel1"] = [][]byte{(rootCAs[3])}
	cas.OrdererRootCAsByChain["channel2"] = [][]byte{rootCAs[4]}
	cas.ServerRootCAs = [][]byte{rootCAs[5]}
	cas.ClientRootCAs = [][]byte{rootCAs[5]}

	appServerRoots, ordererServerRoots := cas.GetServerRootCAs()
	t.Logf("%d appServerRoots | %d ordererServerRoots", len(appServerRoots),
		len(ordererServerRoots))
	assert.Equal(t, 4, len(appServerRoots), "Expected 4 app server root CAs")
	assert.Equal(t, 2, len(ordererServerRoots), "Expected 2 orderer server root CAs")

	appClientRoots, ordererClientRoots := cas.GetClientRootCAs()
	t.Logf("%d appClientRoots | %d ordererClientRoots", len(appClientRoots),
		len(ordererClientRoots))
	assert.Equal(t, 4, len(appClientRoots), "Expected 4 app client root CAs")
	assert.Equal(t, 2, len(ordererClientRoots), "Expected 4 orderer client root CAs")

	// make sure we really have a singleton
	casClone := GetCASupport()
	assert.Exactly(t, casClone, cas, "Expected GetCASupport to be a singleton")

	creds, _ := cas.GetDeliverServiceCredentials("channel1")
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cas.GetPeerCredentials(tls.Certificate{})
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	// append some bad certs and make sure things still work
	cas.ServerRootCAs = append(cas.ServerRootCAs, []byte("badcert"))
	cas.ServerRootCAs = append(cas.ServerRootCAs, []byte(badPEM))
	creds, _ = cas.GetDeliverServiceCredentials("channel1")
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cas.GetPeerCredentials(tls.Certificate{})
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

}

type srv struct {
	port int
	GRPCServer
	caCert   []byte
	serviced uint32
}

func (s *srv) assertServiced(t *testing.T) {
	assert.Equal(t, uint32(1), atomic.LoadUint32(&s.serviced))
	atomic.StoreUint32(&s.serviced, 0)
}

func (s *srv) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	atomic.StoreUint32(&s.serviced, 1)
	return &testpb.Empty{}, nil
}

func newServer(org string, port int) *srv {
	certs := map[string][]byte{
		"ca.crt":     nil,
		"server.crt": nil,
		"server.key": nil,
	}
	for suffix := range certs {
		fName := filepath.Join("testdata", "impersonation", org, suffix)
		cert, err := ioutil.ReadFile(fName)
		if err != nil {
			panic(fmt.Errorf("Failed reading %s: %v", fName, err))
		}
		certs[suffix] = cert
	}
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(fmt.Errorf("Failed listening on port %d: %v", port, err))
	}
	gSrv, err := NewGRPCServerFromListener(l, SecureServerConfig{
		ServerCertificate: certs["server.crt"],
		ServerKey:         certs["server.key"],
		UseTLS:            true,
	})
	if err != nil {
		panic(fmt.Errorf("Failed starting gRPC server: %v", err))
	}
	s := &srv{
		port:       port,
		caCert:     certs["ca.crt"],
		GRPCServer: gSrv,
	}
	testpb.RegisterTestServiceServer(gSrv.Server(), s)
	go s.Start()
	return s
}

func TestImpersonation(t *testing.T) {
	// Scenario: We have 2 organizations: orgA, orgB
	// and each of them are in their respected channels- A, B.
	// The test would obtain credentials.TransportCredentials by calling GetDeliverServiceCredentials.
	// Each organization would have its own gRPC server (srvA and srvB) with a TLS certificate
	// signed by its root CA and with a SAN entry of 'localhost'.
	// We test the following assertions:
	// 1) Invocation with GetDeliverServiceCredentials("A") to srvA succeeds
	// 2) Invocation with GetDeliverServiceCredentials("B") to srvB succeeds
	// 3) Invocation with GetDeliverServiceCredentials("A") to srvB fails
	// 4) Invocation with GetDeliverServiceCredentials("B") to srvA fails

	osA := newServer("orgA", 7070)
	defer osA.Stop()
	osB := newServer("orgB", 7080)
	defer osB.Stop()
	time.Sleep(time.Second)

	cas := GetCASupport()
	_, err := GetCASupport().GetDeliverServiceCredentials("C")
	assert.Error(t, err)

	cas.OrdererRootCAsByChain["A"] = [][]byte{osA.caCert}
	cas.OrdererRootCAsByChain["B"] = [][]byte{osB.caCert}

	testInvoke(t, "A", osA, true)
	testInvoke(t, "B", osB, true)
	testInvoke(t, "A", osB, false)
	testInvoke(t, "B", osA, false)

}

func testInvoke(t *testing.T, channelID string, s *srv, shouldSucceed bool) {
	creds, err := GetCASupport().GetDeliverServiceCredentials(channelID)
	assert.NoError(t, err)
	endpoint := fmt.Sprintf("localhost:%d", s.port)
	conn, err := grpc.Dial(endpoint, grpc.WithTimeout(time.Second*3), grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if shouldSucceed {
		assert.NoError(t, err)
		defer conn.Close()
	} else {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "certificate signed by unknown authority")
		return
	}
	client := testpb.NewTestServiceClient(conn)
	_, err = client.EmptyCall(context.Background(), &testpb.Empty{})
	assert.NoError(t, err)
	s.assertServiced(t)
}
