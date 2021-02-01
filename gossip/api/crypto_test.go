/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/pem"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestPeerIdentityTypeString(t *testing.T) {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "peer.pem"))
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
			require.Equal(t, testCase.identity.String(), testCase.expectedOut)
		})
	}
}
