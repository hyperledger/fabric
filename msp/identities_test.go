/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/bccsp/signer"
	"github.com/hyperledger/fabric-lib-go/bccsp/utils"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"

	"github.com/onsi/gomega"
)

var (
	caCertPem = `
-----BEGIN CERTIFICATE-----
MIIDRzCCAi+gAwIBAgIUWWC20BUnUjeP7JjfLlxiIGR8ESQwDQYJKoZIhvcNAQEL
BQAwMzELMAkGA1UEBhMCQlIxFzAVBgNVBAgMDlNhbnRhIENhdGFyaW5hMQswCQYD
VQQKDAJDQTAeFw0yMjA4MTgxMjQyMDZaFw0yMjA5MTcxMjQyMDZaMDMxCzAJBgNV
BAYTAkJSMRcwFQYDVQQIDA5TYW50YSBDYXRhcmluYTELMAkGA1UECgwCQ0EwggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDJL2ufG03GoNpyWUbTx47IXWOR
nCgbGQg+EGTmYxg3qtZxvmEUMPxvwk5ywR0o6X9XJ0bouBvGVAwCte7wlqPWcukY
rAlI2+KHGef1+j2QeLaSjckCKP6AO+aVQo4abu9cEj+yum121+nJsyUuSaj9MjUe
Y0838SVx6XiVb/PyjRrtCCJnyh4uv9dg9cOhcmk5iEmdOgMvZ4vXQPyHZ73IgAEh
+3ZOmfjdd1R/l1X1xARgPwPECFe8LuDNuA5kioHaHFKeVThbgwE0e7O4Tan7k6H6
xMLB+nso5b24zMBmMOLobjNKkqAZQ3msqUlwY0bgP/pcza+LZ3Mq+E1jqm0XAgMB
AAGjUzBRMB0GA1UdDgQWBBRKCSP+6cyt4wlz+GkBG+N0aTt/yTAfBgNVHSMEGDAW
gBRKCSP+6cyt4wlz+GkBG+N0aTt/yTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4IBAQADT8J+K5okL+RbAaje1Y93x26yFPrRX9WgCpK5gzesPiWmUVTA
mWVZ2Tb4GvArzAYFLolJCkG4zrOVdxjtf6wTuT9woCwfyuaFGyEoI296uvt9EAn3
FCNBVBmfHvTUn8rghrRAMSKAbHYpX4/sL3MR90XgUcsrCpITAGvwG6dQK876Dllj
nOsdRXD/fx6ZMJadw4l9H1CAhchjSrewVFLFaxTCYrgzZcESPioeZAnV1ng7Gpi0
sglaCs2R+C4LfYxLgnFbSov3oLQFlNhbcT1FNXKgO43t5UZOxN3OJWApIQLbEcMW
KonqLBlBcUw+B5BI+UQuej+RhR6oDToJhEkw
-----END CERTIFICATE-----
	`

	ed25519Pem = `
-----BEGIN CERTIFICATE-----
MIICITCCAQkCFHW7pHE/+jVneU0Vf2dCTXtryD9AMA0GCSqGSIb3DQEBCwUAMDMx
CzAJBgNVBAYTAkJSMRcwFQYDVQQIDA5TYW50YSBDYXRhcmluYTELMAkGA1UECgwC
Q0EwHhcNMjIwODE4MTI0MzA2WhcNMjMwODE4MTI0MzA2WjBhMQswCQYDVQQGEwJC
UjEXMBUGA1UECAwOU2FudGEgQ2F0YXJpbmExITAfBgNVBAoMGEludGVybmV0IFdp
ZGdpdHMgUHR5IEx0ZDEWMBQGA1UEAwwNRWQyNTUxOVNpZ25lcjAqMAUGAytlcAMh
APJ2gXHXpaql6cSM+AFLAT31fLlX0h0qz5kLvpQB839zMA0GCSqGSIb3DQEBCwUA
A4IBAQCbGxGxWWxPqiinY3yMKpujLND7lrhaYjMLTPJsD+DqwhxiOGXU2j+LTieH
cl3ki73BhAfd4RnGV8HOgU9M/q0Pcijf3t2/o/0r82S+icxNxYE28aZmLuEPSBrM
E8L3bKIcCLF/gMJMUsb0jtdL+w0b1oFk3h03koSUmcWkidp8kR4B6ix2V8OXDCI7
AX+QZEtz6rX2Q6BaCVEAfgNns5p6zRU9ka0Iru4G7iJ5nh1GPsHvB2XIdCxTPlVE
/9jvH/dCBZmqEtpI3TVB+GFqqWN4OBX/ZIJ7KFb2Le6wBFRklQPfq6e24rFtBs/3
Zlqm3YYh5998U6GvkNWwWGRrWPsy
-----END CERTIFICATE-----
	`

	ed25519PrivPem = `
-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIDIZyl9PEsGJucItbrY481DDqwmvd/feM/m1U/2XLfaA
-----END PRIVATE KEY-----	
	`

	ecdsaCertPem = `
-----BEGIN CERTIFICATE-----
MIICTzCCATcCFHW7pHE/+jVneU0Vf2dCTXtryD9BMA0GCSqGSIb3DQEBCwUAMDMx
CzAJBgNVBAYTAkJSMRcwFQYDVQQIDA5TYW50YSBDYXRhcmluYTELMAkGA1UECgwC
Q0EwHhcNMjIwODE4MTI1MDU0WhcNMjMwODE4MTI1MDU0WjBgMQswCQYDVQQGEwJC
UjEXMBUGA1UECAwOU2FudGEgQ2F0YXJpbmExITAfBgNVBAoMGEludGVybmV0IFdp
ZGdpdHMgUHR5IEx0ZDEVMBMGA1UEAwwMRWNkc2EgU2lnbmVyMFkwEwYHKoZIzj0C
AQYIKoZIzj0DAQcDQgAEfADXJtP8DpjJIQPBBe2aqzIUf/uUQdaw9nmI3cKEq4e7
i4nHjL5gUdiVmT6jldSKIETZm+kszfkANWxzKZXcXjANBgkqhkiG9w0BAQsFAAOC
AQEAbFrqIJ8g9nYq03Wpq1AcfwCsAE5r590bGXZKVTl/9K0NPHptkHN0BIO1eRX4
fPja+3xA+styBb+8xejbesQlzpLAtWiwGLz/fOuT6fa1njNrmQAUHnBdFhAvygMX
VSbd9O7oZFb0FyW2xibbUcoqINMUv/flRPnd6zkLqRXVPVA2bTnUvKMj0+6c9UFB
vOkVjScYhT7Ryt0Dnk+FO7ve2yZpUKiTqohpj0Vp4e1b8k+CTvWWd6c1ukKapPQ2
aE8/sIgUnxoberofYvQxmgZA1yERCj/AWhAJLuR4pNAr77jTEtahZw9VzXjpL8Vv
64BWq4+rLbGZ+kqBL+oPC6ZJ8Q==
-----END CERTIFICATE-----
	`

	ecdsaPrivPem = `
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAHTRWzTa9FPfXPFhw3wFi83oSUvxJ5/r+Su7CDrQOMDoAoGCCqGSM49
AwEHoUQDQgAEfADXJtP8DpjJIQPBBe2aqzIUf/uUQdaw9nmI3cKEq4e7i4nHjL5g
UdiVmT6jldSKIETZm+kszfkANWxzKZXcXg==
-----END EC PRIVATE KEY-----
`
)

// This test verifies that the ED25519 signing will
// sign the MESSAGE, while the ECDSA signing will
// sign the DIGEST
// The same applies to the Verify function
func TestSignatureAlgorithms(t *testing.T) {
	gt := gomega.NewGomegaWithT(t)

	t.Run("Test ecdsa sign with digest and ed25519 sign with full message", func(t *testing.T) {
		bccspDefault := factory.GetDefault()
		mspImpl, _ := newBccspMsp(MSPv3_0, bccspDefault)
		mspImpl.(*bccspmsp).cryptoConfig = &msp.FabricCryptoConfig{
			SignatureHashFamily:            "SHA2",
			IdentityIdentifierHashFunction: "SHA256",
		}

		ed25519Der, _ := pem.Decode([]byte(ed25519Pem))
		ed25519Cert, _ := x509.ParseCertificate(ed25519Der.Bytes)
		ed25519PrivKeyDer, _ := pem.Decode([]byte(ed25519PrivPem))
		ed25519PrivKey, _ := x509.ParsePKCS8PrivateKey(ed25519PrivKeyDer.Bytes)

		ecdsaDer, _ := pem.Decode([]byte(ecdsaCertPem))
		ecdsaCert, _ := x509.ParseCertificate(ecdsaDer.Bytes)
		ecdsaPrivKeyDer, _ := pem.Decode([]byte(ecdsaPrivPem))
		ecdsaPrivKey, _ := x509.ParseECPrivateKey(ecdsaPrivKeyDer.Bytes)

		ed25519FabricPubKey, _ := bccspDefault.KeyImport(ed25519Cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
		ecdsaFabricPubKey, _ := bccspDefault.KeyImport(ecdsaCert, &bccsp.X509PublicKeyImportOpts{Temporary: true})

		ed25519FabricPrivKey, _ := bccspDefault.KeyImport(ed25519PrivKeyDer.Bytes, &bccsp.ED25519PrivateKeyImportOpts{Temporary: true})
		ecdsaFabricPrivKey, _ := bccspDefault.KeyImport(ecdsaPrivKeyDer.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})

		ed25519Signer, _ := signer.New(bccspDefault, ed25519FabricPrivKey)
		ecdsaSigner, _ := signer.New(bccspDefault, ecdsaFabricPrivKey)

		// create identities
		ed25519SigningIdentity, _ := newSigningIdentity(
			ed25519Cert,
			ed25519FabricPubKey,
			ed25519Signer,
			mspImpl.(*bccspmsp))

		ecdsaSigningIdentity, _ := newSigningIdentity(
			ecdsaCert,
			ecdsaFabricPubKey,
			ecdsaSigner,
			mspImpl.(*bccspmsp))

		sigEd25519, _ := ed25519SigningIdentity.Sign([]byte("TEST"))
		sigEcdsa, _ := ecdsaSigningIdentity.Sign([]byte("TEST"))

		expectedSig := ed25519.Sign(ed25519PrivKey.(ed25519.PrivateKey), []byte("TEST"))
		gt.Expect(expectedSig).To(gomega.Equal(sigEd25519))

		msgDgst := sha256.Sum256([]byte("TEST"))
		r, s, _ := ecdsa.Sign(rand.Reader, ecdsaPrivKey, msgDgst[:])
		s, _ = utils.ToLowS(&ecdsaPrivKey.PublicKey, s)
		expectedSig, _ = utils.MarshalECDSASignature(r, s)

		err := ed25519SigningIdentity.Verify([]byte("TEST"), sigEd25519)
		gt.Expect(err).NotTo(gomega.HaveOccurred())
		err = ecdsaSigningIdentity.Verify([]byte("TEST"), sigEcdsa)
		gt.Expect(err).NotTo(gomega.HaveOccurred())
		err = ecdsaSigningIdentity.Verify([]byte("TEST"), expectedSig)
		gt.Expect(err).NotTo(gomega.HaveOccurred())
	})
}

func TestIdentityValidation(t *testing.T) {
	gt := gomega.NewGomegaWithT(t)
	t.Run("Test MSPv3_0 ed2551 identity validation", func(t *testing.T) {
		bccspDefault := factory.GetDefault()
		mspImpl, _ := newBccspMsp(MSPv1_4_3, bccspDefault)
		cryptoConfig := &msp.FabricCryptoConfig{
			SignatureHashFamily:            "SHA2",
			IdentityIdentifierHashFunction: "SHA256",
		}

		mspImpl.(*bccspmsp).cryptoConfig = cryptoConfig
		mspConfigBytes, _ := proto.Marshal(&msp.FabricMSPConfig{
			RootCerts: [][]byte{[]byte(caCertPem)},
			Admins:    [][]byte{[]byte(ecdsaCertPem)},
		})
		mspImpl.Setup(&msp.MSPConfig{Config: mspConfigBytes})

		ed25519Der, _ := pem.Decode([]byte(ed25519Pem))
		ed25519Cert, _ := x509.ParseCertificate(ed25519Der.Bytes)

		ecdsaDer, _ := pem.Decode([]byte(ecdsaCertPem))
		ecdsaCert, _ := x509.ParseCertificate(ecdsaDer.Bytes)

		ed25519FabricPubKey, _ := bccspDefault.KeyImport(ed25519Cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
		ecdsaFabricPubKey, _ := bccspDefault.KeyImport(ecdsaCert, &bccsp.X509PublicKeyImportOpts{Temporary: true})

		ed25519Identity, _ := newIdentity(ed25519Cert, ed25519FabricPubKey, mspImpl.(*bccspmsp))
		ecdsaIdentity, _ := newIdentity(ecdsaCert, ecdsaFabricPubKey, mspImpl.(*bccspmsp))

		err := mspImpl.Validate(ed25519Identity)
		gt.Expect(err).To(gomega.HaveOccurred())
		err = mspImpl.Validate(ecdsaIdentity)
		gt.Expect(err).NotTo(gomega.HaveOccurred())

		mspImpl, _ = newBccspMsp(MSPv3_0, bccspDefault)
		mspImpl.(*bccspmsp).cryptoConfig = cryptoConfig
		mspImpl.Setup(&msp.MSPConfig{Config: mspConfigBytes})

		ed25519Identity, _ = newIdentity(ed25519Cert, ed25519FabricPubKey, mspImpl.(*bccspmsp))
		ecdsaIdentity, _ = newIdentity(ecdsaCert, ecdsaFabricPubKey, mspImpl.(*bccspmsp))

		err = mspImpl.Validate(ed25519Identity)
		gt.Expect(err).NotTo(gomega.HaveOccurred())
		err = mspImpl.Validate(ecdsaIdentity)
		gt.Expect(err).NotTo(gomega.HaveOccurred())
	})
}
