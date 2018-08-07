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

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestRevocation(t *testing.T) {
	// testdata/revocation
	// 1) a key and a signcert (used to populate the default signing identity);
	// 2) cacert is the CA that signed the intermediate;
	// 3) a revocation list that revokes signcert
	thisMSP := getLocalMSP(t, "testdata/revocation")

	id, err := thisMSP.GetDefaultSigningIdentity()
	assert.NoError(t, err)

	// the certificate associated to this id is revoked and so validation should fail!
	err = id.Validate()
	assert.Error(t, err)

	// This MSP is identical to the previous one, with only 1 difference:
	// the signature on the CRL is invalid
	thisMSP = getLocalMSP(t, "testdata/revocation2")

	id, err = thisMSP.GetDefaultSigningIdentity()
	assert.NoError(t, err)

	// the certificate associated to this id is revoked but the signature on the CRL is invalid
	// so validation should succeed
	err = id.Validate()
	assert.NoError(t, err, "Identity found revoked although the signature over the CRL is invalid")
}

func TestIdentityPolicyPrincipalAgainstRevokedIdentity(t *testing.T) {
	// testdata/revocation
	// 1) a key and a signcert (used to populate the default signing identity);
	// 2) cacert is the CA that signed the intermediate;
	// 3) a revocation list that revokes signcert
	thisMSP := getLocalMSP(t, "testdata/revocation")

	id, err := thisMSP.GetDefaultSigningIdentity()
	assert.NoError(t, err)

	idSerialized, err := id.Serialize()
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idSerialized}

	err = id.SatisfiesPrincipal(principal)
	assert.Error(t, err)
}

func TestRevokedIntermediateCA(t *testing.T) {
	// testdata/revokedica
	// 1) a key and a signcert (used to populate the default signing identity);
	// 2) cacert is the CA that signed the intermediate;
	// 3) a revocation list that revokes the intermediate CA cert
	dir := "testdata/revokedica"
	conf, err := GetLocalMspConfig(dir, nil, "SampleOrg")
	assert.NoError(t, err)

	thisMSP, err := newBccspMsp(MSPv1_0)
	assert.NoError(t, err)
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	assert.NoError(t, err)
	csp, err := sw.NewWithParams(256, "SHA2", ks)
	assert.NoError(t, err)
	thisMSP.(*bccspmsp).bccsp = csp

	err = thisMSP.Setup(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CA Certificate is not valid, ")
}
