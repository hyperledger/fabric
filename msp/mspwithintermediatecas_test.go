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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMSPWithIntermediateCAs(t *testing.T) {
	// testdata/intermediate contains the credentials for a test MSP setup that has
	// 1) a key and a signcert (used to populate the default signing identity);
	//    signcert is not signed by a CA directly but by an intermediate CA
	// 2) intermediatecert is an intermediate CA, signed by the CA
	// 3) cacert is the CA that signed the intermediate
	thisMSP := getLocalMSP(t, "testdata/intermediate")

	// This MSP will trust any cert signed by the CA directly OR by the intermediate

	sid, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	sidBytes, err := sid.Serialize()
	require.NoError(t, err)
	id, err := thisMSP.DeserializeIdentity(sidBytes)
	require.NoError(t, err)

	// ensure that we validate correctly the identity
	err = thisMSP.Validate(id)
	require.NoError(t, err)

	id, err = thisMSP.DeserializeIdentity(sidBytes)
	require.NoError(t, err)

	// ensure that validation of an identity of the MSP with intermediate CAs
	// fails with the local MSP
	err = localMsp.Validate(id)
	require.Error(t, err)

	// ensure that validation of an identity of the local MSP
	// fails with the MSP with intermediate CAs
	localMSPID, err := localMsp.GetDefaultSigningIdentity()
	require.NoError(t, err)
	err = thisMSP.Validate(localMSPID)
	require.Error(t, err)
}

func TestMSPWithExternalIntermediateCAs(t *testing.T) {
	// testdata/external contains the credentials for a test MSP setup
	// identical to testdata/intermediate with the exception that it has
	// been generated independently of the fabric environment using
	// openssl.  Sanitizing certificates may cause a change in the
	// signature algorithm used from that used in original
	// certificate file.  Hashes of raw certificate bytes and
	// byte to byte comparisons between the raw certificate and the
	// one imported into the MSP could falsely fail.

	thisMSP := getLocalMSP(t, "testdata/external")

	// This MSP will trust any cert signed only by the intermediate

	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	// ensure that we validate correctly the identity
	err = thisMSP.Validate(id.GetPublicVersion())
	require.NoError(t, err)
}

func TestIntermediateCAIdentityValidity(t *testing.T) {
	// testdata/intermediate contains the credentials for a test MSP setup that has
	// 1) a key and a signcert (used to populate the default signing identity);
	//    signcert is not signed by a CA directly but by an intermediate CA
	// 2) intermediatecert is an intermediate CA, signed by the CA
	// 3) cacert is the CA that signed the intermediate
	thisMSP := getLocalMSP(t, "testdata/intermediate")

	id := thisMSP.(*bccspmsp).intermediateCerts[0]
	require.Error(t, id.Validate())
}

func TestMSPWithIntermediateCAs2(t *testing.T) {
	// testdata/intermediate2 contains the credentials for a test MSP setup that has
	// 1) a key and a signcert (used to populate the default signing identity);
	//    signcert is not signed by a CA directly but by an intermediate CA
	// 2) intermediatecert is an intermediate CA, signed by the CA
	// 3) cacert is the CA that signed the intermediate
	// 4) user2-cert is the certificate of an identity signed directly by the CA
	//    therefore validation should fail.
	thisMSP := getLocalMSP(t, filepath.Join("testdata", "intermediate2"))

	// the default signing identity is signed by the intermediate CA,
	// the validation should return no error
	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	err = thisMSP.Validate(id.GetPublicVersion())
	require.NoError(t, err)

	// user2-cert has been signed by the root CA, validation must fail
	pem, err := readPemFile(filepath.Join("testdata", "intermediate2", "users", "user2-cert.pem"))
	require.NoError(t, err)
	id2, _, err := thisMSP.(*bccspmsp).getIdentityFromConf(pem)
	require.NoError(t, err)
	err = thisMSP.Validate(id2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid validation chain. Parent certificate should be a leaf of the certification tree ")
}
