/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	testpb "github.com/hyperledger/fabric/core/comm/testdata/grpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

func TestClientConnections(t *testing.T) {
	t.Parallel()

	//use Org1 test crypto material
	fileBase := "Org1"
	certPEMBlock, _ := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-cert.pem"))
	keyPEMBlock, _ := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-server1-key.pem"))
	caPEMBlock, _ := ioutil.ReadFile(filepath.Join("testdata", "certs", fileBase+"-cert.pem"))
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caPEMBlock)

	var tests = []struct {
		name       string
		sc         ServerConfig
		creds      credentials.TransportCredentials
		clientPort int
		fail       bool
	}{
		{
			name: "ValidConnection",
			sc: ServerConfig{
				SecOpts: &SecureOptions{
					UseTLS: false}},
		},
		{
			name: "InvalidConnection",
			sc: ServerConfig{
				SecOpts: &SecureOptions{
					UseTLS: false}},
			clientPort: 20040,
			fail:       true,
		},
		{
			name: "ValidConnectionTLS",
			sc: ServerConfig{
				SecOpts: &SecureOptions{
					UseTLS:      true,
					Certificate: certPEMBlock,
					Key:         keyPEMBlock}},
			creds: credentials.NewClientTLSFromCert(certPool, ""),
		},
		{
			name: "InvalidConnectionTLS",
			sc: ServerConfig{
				SecOpts: &SecureOptions{
					UseTLS:      true,
					Certificate: certPEMBlock,
					Key:         keyPEMBlock}},
			creds: credentials.NewClientTLSFromCert(nil, ""),
			fail:  true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("Running test %s ...", test.name)
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("failed to create listener for test server: %v", err)
			}
			clientAddress := lis.Addr().String()
			if test.clientPort > 0 {
				clientAddress = fmt.Sprintf("127.0.0.1:%d", test.clientPort)
			}
			srv, err := NewGRPCServerFromListener(lis, test.sc)
			//check for error
			if err != nil {
				t.Fatalf("Error [%s] creating test server for address [%s]",
					err, lis.Addr().String())
			}
			//start the server
			go srv.Start()
			defer srv.Stop()
			testConn, err := NewClientConnectionWithAddress(clientAddress,
				true, test.sc.SecOpts.UseTLS, test.creds, nil)
			if test.fail {
				assert.Error(t, err)
			} else {
				testConn.Close()
				assert.NoError(t, err)
			}
		})
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

func TestCredentialSupport(t *testing.T) {
	t.Parallel()
	rootCAs := loadRootCAs()
	t.Logf("loaded %d root certificates", len(rootCAs))
	if len(rootCAs) != 6 {
		t.Fatalf("failed to load root certificates")
	}

	cs := &CredentialSupport{
		AppRootCAsByChain:           make(map[string]CertificateBundle),
		OrdererRootCAsByChainAndOrg: make(OrgRootCAs),
	}
	cert := tls.Certificate{Certificate: [][]byte{}}
	cs.SetClientCertificate(cert)
	assert.Equal(t, cert, cs.clientCert)
	assert.Equal(t, cert, cs.GetClientCertificate())

	cs.AppRootCAsByChain["channel1"] = [][]byte{rootCAs[0]}
	cs.AppRootCAsByChain["channel2"] = [][]byte{rootCAs[1]}
	cs.AppRootCAsByChain["channel3"] = [][]byte{rootCAs[2]}
	cs.OrdererRootCAsByChainAndOrg.AppendCertificates("channel1", "SampleOrg", [][]byte{rootCAs[3]})
	cs.OrdererRootCAsByChainAndOrg.AppendCertificates("channel2", "SampleOrg", [][]byte{rootCAs[4]})
	cs.ServerRootCAs = [][]byte{rootCAs[5]}
	cs.ClientRootCAs = [][]byte{rootCAs[5]}

	creds, _ := cs.GetDeliverServiceCredentials("channel1", false, []string{"SampleOrg"}, nil)
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cs.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	_, err := cs.GetDeliverServiceCredentials("channel99", false, nil, nil)
	assert.EqualError(t, err, "didn't find any root CA certs for channel channel99")

	// append some bad certs and make sure things still work
	cs.ServerRootCAs = append(cs.ServerRootCAs, []byte("badcert"))
	cs.ServerRootCAs = append(cs.ServerRootCAs, []byte(badPEM))
	creds, _ = cs.GetDeliverServiceCredentials("channel1", false, []string{"SampleOrg"}, nil)
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cs.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	// test singleton
	singleton := GetCredentialSupport()
	clone := GetCredentialSupport()
	assert.Exactly(t, clone, singleton, "Expected GetCredentialSupport to be a singleton")
}

type srv struct {
	port    int
	address string
	*GRPCServer
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

func newServer(org string) *srv {
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
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("Failed to create listener: %v", err))
	}
	gSrv, err := NewGRPCServerFromListener(l, ServerConfig{
		ConnectionTimeout: 250 * time.Millisecond,
		SecOpts: &SecureOptions{
			Certificate: certs["server.crt"],
			Key:         certs["server.key"],
			UseTLS:      true,
		},
	})
	if err != nil {
		panic(fmt.Errorf("Failed starting gRPC server: %v", err))
	}
	s := &srv{
		address:    l.Addr().String(),
		caCert:     certs["ca.crt"],
		GRPCServer: gSrv,
	}
	testpb.RegisterTestServiceServer(gSrv.Server(), s)
	go s.Start()
	return s
}

func TestImpersonation(t *testing.T) {
	t.Parallel()
	// Scenario: We have 2 organizations: orgA, orgB
	// and each of them are in their respected channels- A, B.
	// The test would obtain credentials.TransportCredentials by calling GetDeliverServiceCredentials.
	// Each organization would have its own gRPC server (srvA and srvB) with a TLS certificate
	// signed by its root CA and with a SAN entry of '127.0.0.1'.
	// We test the following assertions:
	// 1) Invocation with GetDeliverServiceCredentials("A") to srvA succeeds
	// 2) Invocation with GetDeliverServiceCredentials("B") to srvB succeeds
	// 3) Invocation with GetDeliverServiceCredentials("A") to srvB fails
	// 4) Invocation with GetDeliverServiceCredentials("B") to srvA fails

	osA := newServer("orgA")
	defer osA.Stop()
	osB := newServer("orgB")
	defer osB.Stop()
	time.Sleep(time.Second)

	cs := &CredentialSupport{
		AppRootCAsByChain:           make(map[string]CertificateBundle),
		OrdererRootCAsByChainAndOrg: make(OrgRootCAs),
	}
	_, err := cs.GetDeliverServiceCredentials("C", false, []string{"SampleOrg"}, nil)
	assert.Error(t, err)

	cs.OrdererRootCAsByChainAndOrg.AppendCertificates("A", "SampleOrg", [][]byte{osA.caCert})
	cs.OrdererRootCAsByChainAndOrg.AppendCertificates("B", "SampleOrg", [][]byte{osB.caCert})
	cs.ServerRootCAs = append(cs.ServerRootCAs, osB.caCert)

	testInvoke(t, "A", osA, cs, false, true)
	testInvoke(t, "B", osB, cs, false, true)
	testInvoke(t, "A", osB, cs, false, false)
	testInvoke(t, "B", osA, cs, false, false)
	testInvoke(t, "B", osA, cs, true, false)

}

func testInvoke(
	t *testing.T,
	channelID string,
	s *srv,
	cs *CredentialSupport,
	staticRoots bool,
	shouldSucceed bool) {

	creds, err := cs.GetDeliverServiceCredentials(channelID, staticRoots, []string{"SampleOrg"}, nil)
	assert.NoError(t, err)

	endpoint := s.address
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, endpoint, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if shouldSucceed {
		assert.NoError(t, err)
		defer conn.Close()
	} else {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		return
	}
	client := testpb.NewTestServiceClient(conn)
	_, err = client.EmptyCall(context.Background(), &testpb.Empty{})
	assert.NoError(t, err)
	s.assertServiced(t)
}
