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
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/testutil"
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
	peerAddress := GetPeerTestingAddress("7052")
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

	creds := cas.GetDeliverServiceCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cas.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

	// append some bad certs and make sure things still work
	cas.ServerRootCAs = append(cas.ServerRootCAs, []byte("badcert"))
	cas.ServerRootCAs = append(cas.ServerRootCAs, []byte(badPEM))
	creds = cas.GetDeliverServiceCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")
	creds = cas.GetPeerCredentials()
	assert.Equal(t, "1.2", creds.Info().SecurityVersion,
		"Expected Security version to be 1.2")

}
