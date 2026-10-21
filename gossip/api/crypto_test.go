/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestPeerIdentityTypeString(t *testing.T) {
	certBytes, err := os.ReadFile(filepath.Join("testdata", "peer.pem"))
	require.NoError(t, err)

	for _, testCase := range []struct {
		description string
		identity    PeerIdentityType
		expectedOut string
	}{
		{
			description: "non serialized identity",
			identity:    PeerIdentityType("some garbage"),
			expectedOut: "non SerializedIdentity: c29tZSBnYXJiYWdl",
		},
		{
			description: "non PEM identity",
			identity: PeerIdentityType(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
				Mspid:   "SampleOrg",
				IdBytes: []byte{1, 2, 3},
			})),
			expectedOut: "non PEM encoded identity: CglTYW1wbGVPcmcSAwECAw==",
		},
		{
			description: "non x509 identity",
			identity: PeerIdentityType(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
				Mspid: "SampleOrg",
				IdBytes: pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: []byte{1, 2, 3},
				}),
			})),
			expectedOut: `non x509 identity: CglTYW1wbGVPcmcSOy0tLS0tQkVHSU4gQ0VSVElGSUNBVEUtLS0tLQpBUUlECi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K`,
		},
		{
			description: "x509 identity",
			identity: PeerIdentityType(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
				Mspid:   "SampleOrg",
				IdBytes: certBytes,
			})),
			expectedOut: `{"CN":"peer0.org1.example.com","Issuer-CN":"ca.org1.example.com","Issuer-L-ST-C":"[San Francisco]-[]-[US]","Issuer-OU":["COP"],"L-ST-C":"[San Francisco]-[]-[US]","MSP":"SampleOrg","OU":["COP"]}`,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			require.Equal(t, testCase.expectedOut, testCase.identity.String())
		})
	}
}

// A certificate without an OU yields a nil OrganizationalUnit. encoding/json v2
// marshals nil slices as [] (v1: null) and crypto.go does not set
// FormatNilSliceAsNull, so "OU" and "Issuer-OU" appear as empty arrays.
func TestPeerIdentityTypeStringWithoutOU(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "peer-without-ou"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	identity := PeerIdentityType(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid:   "SampleOrg",
		IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}))

	expected := `{"CN":"peer-without-ou","Issuer-CN":"peer-without-ou","Issuer-L-ST-C":"[]-[]-[]","Issuer-OU":[],"L-ST-C":"[]-[]-[]","MSP":"SampleOrg","OU":[]}`
	require.Equal(t, expected, identity.String())
}
