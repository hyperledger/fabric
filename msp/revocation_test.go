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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

// the following strings contain the credentials for a test MSP setup that has
// 1) a key and a signcert (used to populate the default signing identity);
// 2) cacert is the CA that signed the intermediate;
// 2) a revocation list that revokes signcert
const keyrev = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAsWwFunEzqz1Rh6nvD4MiPkKCtmoxzh3jTquG5MSbeLoAoGCCqGSM49
AwEHoUQDQgAEHBuKsAO43hs4JGpFfiGMkB/xsILTsOvmN2WmwpsPHZNL6w8HWe3x
CPQtdG/XJJvZ+C756KEsUBM3yw5PTfku8g==
-----END EC PRIVATE KEY-----`

var signcertrev = `-----BEGIN CERTIFICATE-----
MIICjDCCAjKgAwIBAgIUBEVwsSx0TmqdbzNwleNBBzoIT0wwCgYIKoZIzj0EAwIw
fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh
biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK
BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMTExMTcwNzAw
WhcNMTcxMTExMTcwNzAwWjBjMQswCQYDVQQGEwJVUzEXMBUGA1UECBMOTm9ydGgg
Q2Fyb2xpbmExEDAOBgNVBAcTB1JhbGVpZ2gxGzAZBgNVBAoTEkh5cGVybGVkZ2Vy
IEZhYnJpYzEMMAoGA1UECxMDQ09QMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE
HBuKsAO43hs4JGpFfiGMkB/xsILTsOvmN2WmwpsPHZNL6w8HWe3xCPQtdG/XJJvZ
+C756KEsUBM3yw5PTfku8qOBpzCBpDAOBgNVHQ8BAf8EBAMCBaAwHQYDVR0lBBYw
FAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHQYDVR0OBBYEFOFC
dcUZ4es3ltiCgAVDoyLfVpPIMB8GA1UdIwQYMBaAFBdnQj2qnoI/xMUdn1vDmdG1
nEgQMCUGA1UdEQQeMByCCm15aG9zdC5jb22CDnd3dy5teWhvc3QuY29tMAoGCCqG
SM49BAMCA0gAMEUCIDf9Hbl4xn3z4EwNKmilM9lX2Fq4jWpAaRVB97OmVEeyAiEA
25aDPQHGGq2AvhKT0wvt08cX1GTGCIbfmuLpMwKQj38=
-----END CERTIFICATE-----`

var cacertrev = `-----BEGIN CERTIFICATE-----
MIICYjCCAgmgAwIBAgIUB3CTDOU47sUC5K4kn/Caqnh114YwCgYIKoZIzj0EAwIw
fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh
biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK
BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMDEyMTkzMTAw
WhcNMjExMDExMTkzMTAwWjB/MQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZv
cm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEfMB0GA1UEChMWSW50ZXJuZXQg
V2lkZ2V0cywgSW5jLjEMMAoGA1UECxMDV1dXMRQwEgYDVQQDEwtleGFtcGxlLmNv
bTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABKIH5b2JaSmqiQXHyqC+cmknICcF
i5AddVjsQizDV6uZ4v6s+PWiJyzfA/rTtMvYAPq/yeEHpBUB1j053mxnpMujYzBh
MA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBQXZ0I9
qp6CP8TFHZ9bw5nRtZxIEDAfBgNVHSMEGDAWgBQXZ0I9qp6CP8TFHZ9bw5nRtZxI
EDAKBggqhkjOPQQDAgNHADBEAiAHp5Rbp9Em1G/UmKn8WsCbqDfWecVbZPQj3RK4
oG5kQQIgQAe4OOKYhJdh3f7URaKfGTf492/nmRmtK+ySKjpHSrU=
-----END CERTIFICATE-----`

var crlrev = `-----BEGIN X509 CRL-----
MIIBYzCCAQgCAQEwCgYIKoZIzj0EAwIwfzELMAkGA1UEBhMCVVMxEzARBgNVBAgT
CkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBGcmFuY2lzY28xHzAdBgNVBAoTFklu
dGVybmV0IFdpZGdldHMsIEluYy4xDDAKBgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhh
bXBsZS5jb20XDTE3MDEyMzIwNTYyMFoXDTE3MDEyNjIwNTYyMFowJzAlAhQERXCx
LHROap1vM3CV40EHOghPTBcNMTcwMTIzMjA0NzMxWqAvMC0wHwYDVR0jBBgwFoAU
F2dCPaqegj/ExR2fW8OZ0bWcSBAwCgYDVR0UBAMCAQgwCgYIKoZIzj0EAwIDSQAw
RgIhAOTTpQYkGO+gwVe1LQOcNMD5fzFViOwBUraMrk6dRMlmAiEA8z2dpXKGwHrj
FRBbKkDnSpaVcZgjns+mLdHV2JkF0gk=
-----END X509 CRL-----`

func TestRevocation(t *testing.T) {
	keyinfo := &msp.KeyInfo{KeyIdentifier: "PEER", KeyMaterial: []byte(keyrev)}

	sigid := &msp.SigningIdentityInfo{PublicSigner: []byte(signcertrev), PrivateSigner: keyinfo}

	fmspconf := &msp.FabricMSPConfig{
		RootCerts:       [][]byte{[]byte(cacertrev)},
		RevocationList:  [][]byte{[]byte(crlrev)},
		SigningIdentity: sigid,
		Name:            "DEFAULT"}

	fmpsjs, _ := proto.Marshal(fmspconf)

	mspconf := &msp.MSPConfig{Config: fmpsjs, Type: int32(FABRIC)}

	thisMSP, err := NewBccspMsp()
	assert.NoError(t, err)

	err = thisMSP.Setup(mspconf)
	assert.NoError(t, err)

	id, err := thisMSP.GetDefaultSigningIdentity()
	assert.NoError(t, err)

	// the certificate associated to this id is revoked and so validation should fail!
	err = id.Validate()
	assert.Error(t, err)
}
