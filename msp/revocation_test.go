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
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestRevocation(t *testing.T) {
	t.Run("ValidCRLSignature", func(t *testing.T) {
		// testdata/revocation
		// 1) a key and a signcert (used to populate the default signing identity);
		// 2) cacert is the CA that signed the intermediate;
		// 3) a revocation list that revokes signcert
		thisMSP := getLocalMSP(t, "testdata/revocation")

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		// the certificate associated to this id is revoked and so validation should fail!
		err = id.Validate()
		require.Error(t, err)
	})

	t.Run("MalformedCRLSignature", func(t *testing.T) {
		// This test appends an extra int to the CRL signature. This extra data is
		// ignored in go 1.14, the signature is considered valid, and the identity
		// is treated as revoked.
		//
		// In go 1.15 the CheckCRLSignature implementation is more strict and the
		// CRL signature is treated as invalid and the identity is not treated as
		// revoked.
		//
		// This behavior change needs to be mitigated between the two versions.
		conf, err := GetLocalMspConfig("testdata/revocation", nil, "SampleOrg")
		require.NoError(t, err)

		// Unmarshal the config
		var mspConfig msp.FabricMSPConfig
		err = proto.Unmarshal(conf.Config, &mspConfig)
		require.NoError(t, err)
		require.Len(t, mspConfig.RevocationList, 1)
		crl, err := x509.ParseCRL(mspConfig.RevocationList[0])
		require.NoError(t, err)

		// Decode the CRL signature
		var sig struct{ R, S *big.Int }
		_, err = asn1.Unmarshal(crl.SignatureValue.Bytes, &sig)
		require.NoError(t, err)

		// Extend the signature with another value
		extendedSig := struct{ R, S, T *big.Int }{sig.R, sig.S, big.NewInt(100)}
		longSigBytes, err := asn1.Marshal(extendedSig)
		require.NoError(t, err)

		// Use the extended signature in the CRL
		crl.SignatureValue.Bytes = longSigBytes
		crl.SignatureValue.BitLength = 8 * len(longSigBytes)
		crlBytes, err := asn1.Marshal(*crl)
		require.NoError(t, err)
		mspConfig.RevocationList[0] = pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlBytes})

		// Remarshal the configuration
		conf.Config, err = proto.Marshal(&mspConfig)
		require.NoError(t, err)

		ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join("testdata/revocation", "keystore"), true)
		require.NoError(t, err)
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(ks)
		require.NoError(t, err)

		thisMSP, err := NewBccspMspWithKeyStore(MSPv1_0, ks, cryptoProvider)
		require.NoError(t, err)

		err = thisMSP.Setup(conf)
		require.NoError(t, err)

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		// the cert associated with this id is revoked and the extra info on the
		// signature is ignored so validation should fail!
		err = id.Validate()
		require.EqualError(t, err, "could not validate identity against certification chain: The certificate has been revoked")
	})

	t.Run("InvalidCRLSignature", func(t *testing.T) {
		// This MSP is identical to the previous one, with only 1 difference:
		// the signature on the CRL is invalid
		thisMSP := getLocalMSP(t, "testdata/revocation2")

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		// the certificate associated to this id is revoked but the signature on the CRL is invalid
		// so validation should succeed
		err = id.Validate()
		require.NoError(t, err, "Identity found revoked although the signature over the CRL is invalid")
	})
}

func TestIdentityPolicyPrincipalAgainstRevokedIdentity(t *testing.T) {
	// testdata/revocation
	// 1) a key and a signcert (used to populate the default signing identity);
	// 2) cacert is the CA that signed the intermediate;
	// 3) a revocation list that revokes signcert
	thisMSP := getLocalMSP(t, "testdata/revocation")

	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	idSerialized, err := id.Serialize()
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idSerialized,
	}

	err = id.SatisfiesPrincipal(principal)
	require.Error(t, err)
}

func TestRevokedIntermediateCA(t *testing.T) {
	// testdata/revokedica
	// 1) a key and a signcert (used to populate the default signing identity);
	// 2) cacert is the CA that signed the intermediate;
	// 3) a revocation list that revokes the intermediate CA cert
	dir := "testdata/revokedica"
	conf, err := GetLocalMspConfig(dir, nil, "SampleOrg")
	require.NoError(t, err)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	thisMSP, err := newBccspMsp(MSPv1_0, cryptoProvider)
	require.NoError(t, err)
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	require.NoError(t, err)
	csp, err := sw.NewWithParams(256, "SHA2", ks)
	require.NoError(t, err)
	thisMSP.(*bccspmsp).bccsp = csp

	err = thisMSP.Setup(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CA Certificate is not valid, ")
}
