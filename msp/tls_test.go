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

package msp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSCAs(t *testing.T) {
	// testdata/tls contains TLS a root CA and an intermediate CA
	thisMSP := getLocalMSP(t, "testdata/tls")

	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = thisMSP.Validate(id.GetPublicVersion())
	require.NoError(t, err)

	tlsRootCerts := thisMSP.GetTLSRootCerts()
	require.Len(t, tlsRootCerts, 1)
	tlsRootCerts2, err := getPemMaterialFromDir("testdata/tls/tlscacerts")
	require.NoError(t, err)
	require.Len(t, tlsRootCerts2, 1)
	require.Equal(t, tlsRootCerts2[0], tlsRootCerts[0])

	tlsIntermediateCerts := thisMSP.GetTLSIntermediateCerts()
	require.Len(t, tlsIntermediateCerts, 1)
	tlsIntermediateCerts2, err := getPemMaterialFromDir("testdata/tls/tlsintermediatecerts")
	require.NoError(t, err)
	require.Len(t, tlsIntermediateCerts2, 1)
	require.Equal(t, tlsIntermediateCerts2[0], tlsIntermediateCerts[0])
}
