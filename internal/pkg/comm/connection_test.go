/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
		appRootCAsByChain: make(map[string][][]byte),
	}
	require.Equal(t, expected, NewCredentialSupport())

	rootCAs := [][]byte{
		[]byte("certificate-one"),
		[]byte("certificate-two"),
	}
	expected.serverRootCAs = rootCAs[:]
	require.Equal(t, expected, NewCredentialSupport(rootCAs...))
}

func TestCredentialSupport(t *testing.T) {
	t.Parallel()
	rootCAs := loadRootCAs()
	t.Logf("loaded %d root certificates", len(rootCAs))
	if len(rootCAs) != 6 {
		t.Fatalf("failed to load root certificates")
	}

	cs := &CredentialSupport{
		appRootCAsByChain: make(map[string][][]byte),
	}
	cert := tls.Certificate{Certificate: [][]byte{}}
	cs.SetClientCertificate(cert)
	require.Equal(t, cert, cs.clientCert)
	require.Equal(t, cert, cs.GetClientCertificate())

	cs.appRootCAsByChain["channel1"] = [][]byte{rootCAs[0]}
	cs.appRootCAsByChain["channel2"] = [][]byte{rootCAs[1]}
	cs.appRootCAsByChain["channel3"] = [][]byte{rootCAs[2]}
	cs.serverRootCAs = [][]byte{rootCAs[5]}

	creds := cs.GetPeerCredentials()
	require.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	// append some bad certs and make sure things still work
	cs.serverRootCAs = append(cs.serverRootCAs, []byte("badcert"))
	cs.serverRootCAs = append(cs.serverRootCAs, []byte(badPEM))
	creds = cs.GetPeerCredentials()
	require.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
}
