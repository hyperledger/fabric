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
//    signcert is not signed by a CA directly but by an intermediate CA
// 2) intermediatecert is an intermediate CA, signed by the CA
// 3) cacert is the CA that signed the intermediate
const key = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEII27gKS2mFIIGkyGFEvHyv1khaJHe+p+sDt0++JByCDToAoGCCqGSM49
AwEHoUQDQgAEJUUpwMg/jQ+qpmkVewEvwTySl+XWbd4AXtb/0XsDqXNcyXl0DVgA
gJNGnt5r+bvZdB8SOk1ySAEEsCQArkarMg==
-----END EC PRIVATE KEY-----`

var signcert = `-----BEGIN CERTIFICATE-----
MIIDAzCCAqigAwIBAgIBAjAKBggqhkjOPQQDAjBsMQswCQYDVQQGEwJHQjEQMA4G
A1UECAwHRW5nbGFuZDEOMAwGA1UECgwFQmFyMTkxDjAMBgNVBAsMBUJhcjE5MQ4w
DAYDVQQDDAVCYXIxOTEbMBkGCSqGSIb3DQEJARYMQmFyMTktY2xpZW50MB4XDTE3
MDIwOTE2MDcxMFoXDTE4MDIxOTE2MDcxMFowfDELMAkGA1UEBhMCR0IxEDAOBgNV
BAgMB0VuZ2xhbmQxEDAOBgNVBAcMB0lwc3dpY2gxDjAMBgNVBAoMBUJhcjE5MQ4w
DAYDVQQLDAVCYXIxOTEOMAwGA1UEAwwFQmFyMTkxGTAXBgkqhkiG9w0BCQEWCkJh
cjE5LXBlZXIwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQlRSnAyD+ND6qmaRV7
AS/BPJKX5dZt3gBe1v/RewOpc1zJeXQNWACAk0ae3mv5u9l0HxI6TXJIAQSwJACu
Rqsyo4IBKTCCASUwCQYDVR0TBAIwADARBglghkgBhvhCAQEEBAMCBkAwMwYJYIZI
AYb4QgENBCYWJE9wZW5TU0wgR2VuZXJhdGVkIFNlcnZlciBDZXJ0aWZpY2F0ZTAd
BgNVHQ4EFgQUwHzbLJQMaWd1cpHdkSaEFxdKB1owgYsGA1UdIwSBgzCBgIAUYxFe
+cXOD5iQ223bZNdOuKCRiTKhZaRjMGExCzAJBgNVBAYTAkdCMRAwDgYDVQQIDAdF
bmdsYW5kMRAwDgYDVQQHDAdJcHN3aWNoMQ4wDAYDVQQKDAVCYXIxOTEOMAwGA1UE
CwwFQmFyMTkxDjAMBgNVBAMMBUJhcjE5ggEBMA4GA1UdDwEB/wQEAwIFoDATBgNV
HSUEDDAKBggrBgEFBQcDATAKBggqhkjOPQQDAgNJADBGAiEAuMq65lOaie4705Ol
Ow52DjbaO2YuIxK2auBCqNIu0gECIQCDoKdUQ/sa+9Ah1mzneE6iz/f/YFVWo4EP
HeamPGiDTQ==
-----END CERTIFICATE-----`

var intermediatecert = `-----BEGIN CERTIFICATE-----
MIICITCCAcigAwIBAgIBATAKBggqhkjOPQQDAjBhMQswCQYDVQQGEwJHQjEQMA4G
A1UECAwHRW5nbGFuZDEQMA4GA1UEBwwHSXBzd2ljaDEOMAwGA1UECgwFQmFyMTkx
DjAMBgNVBAsMBUJhcjE5MQ4wDAYDVQQDDAVCYXIxOTAeFw0xNzAyMDkxNTUyMDBa
Fw0yNzAyMDcxNTUyMDBaMGwxCzAJBgNVBAYTAkdCMRAwDgYDVQQIDAdFbmdsYW5k
MQ4wDAYDVQQKDAVCYXIxOTEOMAwGA1UECwwFQmFyMTkxDjAMBgNVBAMMBUJhcjE5
MRswGQYJKoZIhvcNAQkBFgxCYXIxOS1jbGllbnQwWTATBgcqhkjOPQIBBggqhkjO
PQMBBwNCAAQBymfTx4GWt1lnTV4Xp3skM5LJpZ40HVhCDLfvfrD8/3WQLHaLc7XW
KpphhXW8HYLyyjkEZVLsAFHkKjwmlcpzo2YwZDAdBgNVHQ4EFgQUYxFe+cXOD5iQ
223bZNdOuKCRiTIwHwYDVR0jBBgwFoAU4UJ1xRnh6zeW2IKABUOjIt9Wk8gwEgYD
VR0TAQH/BAgwBgEB/wIBADAOBgNVHQ8BAf8EBAMCAYYwCgYIKoZIzj0EAwIDRwAw
RAIgGUgzRtqWx98KkgKNDyeEmBmhpptW966iS7+c8ig4ksMCIEyzhATMpiI4pHzH
xSwZMvo3y3wkMwgf/WrhwdCyZNku
-----END CERTIFICATE-----`

var cacert = `-----BEGIN CERTIFICATE-----
MIICHDCCAcKgAwIBAgIJAJ/qse7uYF0LMAoGCCqGSM49BAMCMGExCzAJBgNVBAYT
AkdCMRAwDgYDVQQIDAdFbmdsYW5kMRAwDgYDVQQHDAdJcHN3aWNoMQ4wDAYDVQQK
DAVCYXIxOTEOMAwGA1UECwwFQmFyMTkxDjAMBgNVBAMMBUJhcjE5MB4XDTE3MDIw
OTE1MzE1MloXDTM3MDIwNDE1MzE1MlowYTELMAkGA1UEBhMCR0IxEDAOBgNVBAgM
B0VuZ2xhbmQxEDAOBgNVBAcMB0lwc3dpY2gxDjAMBgNVBAoMBUJhcjE5MQ4wDAYD
VQQLDAVCYXIxOTEOMAwGA1UEAwwFQmFyMTkwWTATBgcqhkjOPQIBBggqhkjOPQMB
BwNCAAQcG4qwA7jeGzgkakV+IYyQH/GwgtOw6+Y3ZabCmw8dk0vrDwdZ7fEI9C10
b9ckm9n4LvnooSxQEzfLDk9N+S7yo2MwYTAdBgNVHQ4EFgQU4UJ1xRnh6zeW2IKA
BUOjIt9Wk8gwHwYDVR0jBBgwFoAU4UJ1xRnh6zeW2IKABUOjIt9Wk8gwDwYDVR0T
AQH/BAUwAwEB/zAOBgNVHQ8BAf8EBAMCAYYwCgYIKoZIzj0EAwIDSAAwRQIgGvB0
854QmGi1yG5wnWMiwzQxtcEhvCXbnCuiQvr5VrkCIQDoMooDC/WmhBwuCfo7iGDo
AsFd44a8aa9yzABfALG2Gw==
-----END CERTIFICATE-----`

func TestMSPWithIntermediateCAs(t *testing.T) {
	keyinfo := &msp.KeyInfo{KeyIdentifier: "PEER", KeyMaterial: []byte(key)}

	sigid := &msp.SigningIdentityInfo{PublicSigner: []byte(signcert), PrivateSigner: keyinfo}

	fmspconf := &msp.FabricMSPConfig{
		RootCerts:         [][]byte{[]byte(cacert)},
		IntermediateCerts: [][]byte{[]byte(intermediatecert)},
		SigningIdentity:   sigid,
		Name:              "DEFAULT"}

	fmpsjs, _ := proto.Marshal(fmspconf)

	mspconf := &msp.MSPConfig{Config: fmpsjs, Type: int32(FABRIC)}

	thisMSP, err := NewBccspMsp()
	assert.NoError(t, err)

	err = thisMSP.Setup(mspconf)
	assert.NoError(t, err)

	// This MSP will trust any cert signed by the CA directly OR by the intermediate

	id, err := thisMSP.GetDefaultSigningIdentity()
	assert.NoError(t, err)

	// ensure that we validate correctly the identity
	err = thisMSP.Validate(id.GetPublicVersion())
	assert.NoError(t, err)

	// ensure that validation of an identity of the MSP with intermediate CAs
	// fails with the local MSP
	err = localMsp.Validate(id.GetPublicVersion())
	assert.Error(t, err)

	// ensure that validation of an identity of the local MSP
	// fails with the MSP with intermediate CAs
	localMSPID, err := localMsp.GetDefaultSigningIdentity()
	assert.NoError(t, err)
	err = thisMSP.Validate(localMSPID.GetPublicVersion())
	assert.Error(t, err)
}
