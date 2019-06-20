/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm/testpb"
	"github.com/stretchr/testify/assert"
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

func TestNewCredentialSupport(t *testing.T) {
	expected := &CredentialSupport{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}
	assert.Equal(t, expected, NewCredentialSupport())

	rootCAs := [][]byte{
		[]byte("certificate-one"),
		[]byte("certificate-two"),
	}
	expected.serverRootCAs = rootCAs[:]
	assert.Equal(t, expected, NewCredentialSupport(rootCAs...))
}

func TestCredentialSupport(t *testing.T) {
	t.Parallel()
	rootCAs := loadRootCAs()
	t.Logf("loaded %d root certificates", len(rootCAs))
	if len(rootCAs) != 6 {
		t.Fatalf("failed to load root certificates")
	}

	cs := &CredentialSupport{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}
	cert := tls.Certificate{Certificate: [][]byte{}}
	cs.SetClientCertificate(cert)
	assert.Equal(t, cert, cs.clientCert)
	assert.Equal(t, cert, cs.GetClientCertificate())

	cs.appRootCAsByChain["channel1"] = [][]byte{rootCAs[0]}
	cs.appRootCAsByChain["channel2"] = [][]byte{rootCAs[1]}
	cs.appRootCAsByChain["channel3"] = [][]byte{rootCAs[2]}
	cs.ordererRootCAsByChain["channel1"] = [][]byte{rootCAs[3]}
	cs.ordererRootCAsByChain["channel2"] = [][]byte{rootCAs[4]}
	cs.serverRootCAs = [][]byte{rootCAs[5]}

	creds, _ := cs.GetDeliverServiceCredentials("channel1")
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cs.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	_, err := cs.GetDeliverServiceCredentials("channel99")
	assert.EqualError(t, err, "didn't find any root CA certs for channel channel99")

	// append some bad certs and make sure things still work
	cs.serverRootCAs = append(cs.serverRootCAs, []byte("badcert"))
	cs.serverRootCAs = append(cs.serverRootCAs, []byte(badPEM))
	creds, _ = cs.GetDeliverServiceCredentials("channel1")
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cs.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
}

type srv struct {
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
	osB := newServer("orgB")
	time.Sleep(time.Second)

	cs := &CredentialSupport{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}
	_, err := cs.GetDeliverServiceCredentials("C")
	assert.Error(t, err)

	cs.ordererRootCAsByChain["A"] = [][]byte{osA.caCert}
	cs.ordererRootCAsByChain["B"] = [][]byte{osB.caCert}

	var tests = []struct {
		channel string
		server  *srv
		creds   *CredentialSupport
		success bool
	}{
		{
			channel: "A",
			server:  osA,
			creds:   cs,
			success: true,
		},
		{
			channel: "B",
			server:  osB,
			creds:   cs,
			success: true,
		},
		{
			channel: "A",
			server:  osB,
			creds:   cs,
			success: false,
		},
		{
			channel: "B",
			server:  osA,
			creds:   cs,
			success: false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run("", func(t *testing.T) {
			t.Parallel()
			testInvoke(
				t,
				test.channel,
				test.server,
				test.creds,
				test.success,
			)
		})
	}
}

func testInvoke(
	t *testing.T,
	channelID string,
	s *srv,
	cs *CredentialSupport,
	shouldSucceed bool) {

	creds, err := cs.GetDeliverServiceCredentials(channelID)
	assert.NoError(t, err)

	endpoint := s.address
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
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
