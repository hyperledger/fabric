/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package internal_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io/ioutil"
	"os"
	"testing"
	"time"

	. "github.com/hyperledger/fabric/core/chaincode/shim/internal"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/keepalive"
)

var keyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMLemLh3+uDzww1pvqP6Xj2Z0Kc6yqf3RxyfTBNwRuuyoAoGCCqGSM49
AwEHoUQDQgAEDB3l94vM7EqKr2L/vhqU5IsEub0rviqCAaWGiVAPp3orb/LJqFLS
yo/k60rhUiir6iD4S4pb5TEb2ouWylQI3A==
-----END EC PRIVATE KEY-----
`
var certPEM = `-----BEGIN CERTIFICATE-----
MIIBdDCCARqgAwIBAgIRAKCiW5r6W32jGUn+l9BORMAwCgYIKoZIzj0EAwIwEjEQ
MA4GA1UEChMHQWNtZSBDbzAeFw0xODA4MjExMDI1MzJaFw0yODA4MTgxMDI1MzJa
MBIxEDAOBgNVBAoTB0FjbWUgQ28wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQM
HeX3i8zsSoqvYv++GpTkiwS5vSu+KoIBpYaJUA+neitv8smoUtLKj+TrSuFSKKvq
IPhLilvlMRvai5bKVAjco1EwTzAOBgNVHQ8BAf8EBAMCBaAwEwYDVR0lBAwwCgYI
KwYBBQUHAwEwDAYDVR0TAQH/BAIwADAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8A
AAEwCgYIKoZIzj0EAwIDSAAwRQIgOaYc3pdGf2j0uXRyvdBJq2PlK9FkgvsUjXOT
bQ9fWRkCIQCr1FiRRzapgtrnttDn3O2fhLlbrw67kClzY8pIIN42Qw==
-----END CERTIFICATE-----
`
var rootPEM = `-----BEGIN CERTIFICATE-----
MIIB8TCCAZegAwIBAgIQUigdJy6IudO7sVOXsKVrtzAKBggqhkjOPQQDAjBYMQsw
CQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy
YW5jaXNjbzENMAsGA1UEChMET3JnMTENMAsGA1UEAxMET3JnMTAeFw0xODA4MjEw
ODI1MzNaFw0yODA4MTgwODI1MzNaMFgxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpD
YWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMQ0wCwYDVQQKEwRPcmcx
MQ0wCwYDVQQDEwRPcmcxMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEVOI+oAAB
Pl+iRsCcGq81WbXap2L1r432T5gbzUNKYRvVsyFFYmdO8ql8uDi4UxSY64eaeRFT
uxdcsTG7M5K2yaNDMEEwDgYDVR0PAQH/BAQDAgGmMA8GA1UdJQQIMAYGBFUdJQAw
DwYDVR0TAQH/BAUwAwEB/zANBgNVHQ4EBgQEAQIDBDAKBggqhkjOPQQDAgNIADBF
AiEA6U7IRGf+S7e9U2+jSI2eFiBsVEBIi35LgYoKqjELj5oCIAD7DfVMaMHzzjiQ
XIlJQdS/9afDi32qZWZfe3kAUAs0
-----END CERTIFICATE-----
`

func TestLoadConfig(t *testing.T) {
	// setup key/cert files
	testDir, err := ioutil.TempDir("", "shiminternal")
	if err != nil {
		t.Fatalf("Failed to test directory: %s", err)
	}
	defer os.RemoveAll(testDir)

	keyFile, err := ioutil.TempFile(testDir, "testKey")
	if err != nil {
		t.Fatalf("Failed to create key file: %s", err)
	}
	b64Key := base64.StdEncoding.EncodeToString([]byte(keyPEM))
	if _, err := keyFile.WriteString(b64Key); err != nil {
		t.Fatalf("Failed to write to key file: %s", err)
	}

	certFile, err := ioutil.TempFile(testDir, "testCert")
	if err != nil {
		t.Fatalf("Failed to create cert file: %s", err)
	}
	b64Cert := base64.StdEncoding.EncodeToString([]byte(certPEM))
	if _, err := certFile.WriteString(b64Cert); err != nil {
		t.Fatalf("Failed to write to cert file: %s", err)
	}

	rootFile, err := ioutil.TempFile(testDir, "testRoot")
	if err != nil {
		t.Fatalf("Failed to create root file: %s", err)
	}
	if _, err := rootFile.WriteString(rootPEM); err != nil {
		t.Fatalf("Failed to write to root file: %s", err)
	}

	notb64File, err := ioutil.TempFile(testDir, "testNotb64")
	if err != nil {
		t.Fatalf("Failed to create notb64 file: %s", err)
	}
	if _, err := notb64File.WriteString("#####"); err != nil {
		t.Fatalf("Failed to write to notb64 file: %s", err)
	}

	notPEMFile, err := ioutil.TempFile(testDir, "testNotPEM")
	if err != nil {
		t.Fatalf("Failed to create notPEM file: %s", err)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("not pem"))
	if _, err := notPEMFile.WriteString(b64); err != nil {
		t.Fatalf("Failed to write to notPEM file: %s", err)
	}

	defer cleanupEnv()

	// expected TLS config
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM([]byte(rootPEM))
	clientCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("Failed to load client cert pair: %s", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      rootPool,
	}

	kaOpts := keepalive.ClientParameters{
		Time:                1 * time.Minute,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}

	var tests = []struct {
		name     string
		env      map[string]string
		expected Config
		errMsg   string
	}{
		{
			name: "TLS disabled",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME": "testCC",
				"CORE_PEER_TLS_ENABLED":  "false",
			},
			expected: Config{
				ChaincodeName: "testCC",
				KaOpts:        kaOpts,
			},
		},
		{
			name: "TLS Enabled",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":      "testCC",
				"CORE_PEER_TLS_ENABLED":       "true",
				"CORE_TLS_CLIENT_KEY_PATH":    keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH":   certFile.Name(),
				"CORE_PEER_TLS_ROOTCERT_FILE": rootFile.Name(),
			},
			expected: Config{
				ChaincodeName: "testCC",
				TLS:           tlsConfig,
				KaOpts:        kaOpts,
			},
		},
		{
			name: "Bad TLS_ENABLED",
			env: map[string]string{
				"CORE_PEER_TLS_ENABLED": "nottruthy",
			},
			errMsg: "'CORE_PEER_TLS_ENABLED' must be set to 'true' or 'false'",
		},
		{
			name: "Missing key file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":   "testCC",
				"CORE_PEER_TLS_ENABLED":    "true",
				"CORE_TLS_CLIENT_KEY_PATH": "missingkey",
			},
			errMsg: "failed to read private key file",
		},
		{
			name: "Bad key file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":   "testCC",
				"CORE_PEER_TLS_ENABLED":    "true",
				"CORE_TLS_CLIENT_KEY_PATH": notb64File.Name(),
			},
			errMsg: "failed to decode private key file",
		},
		{
			name: "Missing cert file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":    "testCC",
				"CORE_PEER_TLS_ENABLED":     "true",
				"CORE_TLS_CLIENT_KEY_PATH":  keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH": "missingkey",
			},
			errMsg: "failed to read public key file",
		},
		{
			name: "Bad cert file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":    "testCC",
				"CORE_PEER_TLS_ENABLED":     "true",
				"CORE_TLS_CLIENT_KEY_PATH":  keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH": notb64File.Name(),
			},
			errMsg: "failed to decode public key file",
		},
		{
			name: "Missing root file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":      "testCC",
				"CORE_PEER_TLS_ENABLED":       "true",
				"CORE_TLS_CLIENT_KEY_PATH":    keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH":   certFile.Name(),
				"CORE_PEER_TLS_ROOTCERT_FILE": "missingkey",
			},
			errMsg: "failed to read root cert file",
		},
		{
			name: "Bad root file",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":      "testCC",
				"CORE_PEER_TLS_ENABLED":       "true",
				"CORE_TLS_CLIENT_KEY_PATH":    keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH":   certFile.Name(),
				"CORE_PEER_TLS_ROOTCERT_FILE": notb64File.Name(),
			},
			errMsg: "failed to load root cert file",
		},
		{
			name: "Key not PEM",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":      "testCC",
				"CORE_PEER_TLS_ENABLED":       "true",
				"CORE_TLS_CLIENT_KEY_PATH":    notPEMFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH":   certFile.Name(),
				"CORE_PEER_TLS_ROOTCERT_FILE": rootFile.Name(),
			},
			errMsg: "failed to parse client key pair",
		},
		{
			name: "Cert not PEM",
			env: map[string]string{
				"CORE_CHAINCODE_ID_NAME":      "testCC",
				"CORE_PEER_TLS_ENABLED":       "true",
				"CORE_TLS_CLIENT_KEY_PATH":    keyFile.Name(),
				"CORE_TLS_CLIENT_CERT_PATH":   notPEMFile.Name(),
				"CORE_PEER_TLS_ROOTCERT_FILE": rootFile.Name(),
			},
			errMsg: "failed to parse client key pair",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.env {
				os.Setenv(k, v)
			}
			conf, err := LoadConfig()
			if test.errMsg == "" {
				assert.Equal(t, test.expected, conf)
			} else {
				assert.Contains(t, err.Error(), test.errMsg)
			}
		})
	}

}

func cleanupEnv() {
	os.Unsetenv("CORE_PEER_TLS_ENABLED")
	os.Unsetenv("CORE_TLS_CLIENT_KEY_PATH")
	os.Unsetenv("CORE_TLS_CLIENT_CERT_PATH")
	os.Unsetenv("CORE_PEER_TLS_ROOTCERT_FILE")
	os.Unsetenv("CORE_CHAINCODE_ID_NAME")
}
