package remote

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apiinfo "github.com/cloudflare/cfssl/api/info"
	apisign "github.com/cloudflare/cfssl/api/signhandler"
	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/helpers/testsuite"
	"github.com/cloudflare/cfssl/info"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
)

const (
	testCaFile        = "testdata/ca.pem"
	testCaKeyFile     = "testdata/ca_key.pem"
	testServerFile    = "testdata/server.pem"
	testServerKeyFile = "testdata/server-key.pem"
	testClientFile    = "testdata/client.pem"
	testClientKeyFile = "testdata/client-key.pem"
)

var validMinimalRemoteConfig = `
{
	"signing": {
		"default": {
			"remote": "localhost"
		}
	},
	"remotes": {
		"localhost": "127.0.0.1:80"
	}
}`

var validMinimalAuthRemoteConfig = `
{
	"signing": {
		"default": {
			"auth_key": "sample",
			"remote": "localhost"
		}
	},
	"auth_keys": {
		"sample": {
			"type":"standard",
			"key":"0123456789ABCDEF0123456789ABCDEF"
		}
	},
	"remotes": {
		"localhost": "127.0.0.1:80"
	}
}`

func TestNewSigner(t *testing.T) {
	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))

	_, err := NewSigner(remoteConfig.Signing)
	if err != nil {
		t.Fatal("fail to init remote signer:", err)
	}
}

func TestNewAuthSigner(t *testing.T) {
	remoteAuthConfig := testsuite.NewConfig(t, []byte(validMinimalAuthRemoteConfig))

	_, err := NewSigner(remoteAuthConfig.Signing)
	if err != nil {
		t.Fatal("fail to init remote signer:", err)
	}
}

func TestRemoteInfo(t *testing.T) {
	remoteServer := newTestInfoServer(t, false, nil)
	defer closeTestServer(t, remoteServer)

	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))
	// override with test server address, ignore url prefix "http://"
	remoteConfig.Signing.OverrideRemotes(remoteServer.URL[7:])
	verifyRemoteInfo(t, remoteConfig)
}

func TestRemoteTLSInfo(t *testing.T) {
	remoteTLSInfo(t, false)
}

func TestRemoteMutualTLSInfo(t *testing.T) {
	remoteTLSInfo(t, true)
}

func remoteTLSInfo(t *testing.T, isMutual bool) {
	certPool, err := helpers.LoadPEMCertPool(testCaFile)
	if err != nil {
		t.Fatal(err)
	}
	var clientCA *x509.CertPool
	if isMutual {
		clientCA = certPool
	}
	remoteServer := newTestInfoServer(t, true, clientCA)
	defer closeTestServer(t, remoteServer)

	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))
	// override with full server URL to get https in protocol"
	remoteConfig.Signing.OverrideRemotes(remoteServer.URL)
	remoteConfig.Signing.SetRemoteCAs(certPool)
	if isMutual {
		remoteConfig.Signing.SetClientCertKeyPairFromFile(testClientFile, testClientKeyFile)
	}
	verifyRemoteInfo(t, remoteConfig)
}

func verifyRemoteInfo(t *testing.T, remoteConfig *config.Config) {
	s := newRemoteSigner(t, remoteConfig.Signing)
	req := info.Req{}
	resp, err := s.Info(req)
	if err != nil {
		t.Fatal("remote info failed:", err)
	}

	caBytes, err := ioutil.ReadFile(testCaFile)
	caBytes = bytes.TrimSpace(caBytes)
	if err != nil {
		t.Fatal("fail to read test CA cert:", err)
	}

	if bytes.Compare(caBytes, []byte(resp.Certificate)) != 0 {
		t.Fatal("Get a different CA cert through info api.", len(resp.Certificate), len(caBytes))
	}
}

func TestRemoteSign(t *testing.T) {
	remoteServer := newTestSignServer(t, false, nil)
	defer closeTestServer(t, remoteServer)

	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))
	// override with test server address, ignore url prefix "http://"
	remoteConfig.Signing.OverrideRemotes(remoteServer.URL[7:])
	verifyRemoteSign(t, remoteConfig)
}

func TestRemoteTLSSign(t *testing.T) {
	remoteTLSSign(t, false)
}

func TestRemoteMutualTLSSign(t *testing.T) {
	remoteTLSSign(t, true)
}

func remoteTLSSign(t *testing.T, isMutual bool) {
	certPool, err := helpers.LoadPEMCertPool(testCaFile)
	if err != nil {
		t.Fatal(err)
	}
	var clientCA *x509.CertPool
	if isMutual {
		clientCA = certPool
	}
	remoteServer := newTestSignServer(t, true, clientCA)
	defer closeTestServer(t, remoteServer)

	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))
	// override with full server URL to get https in protocol"
	remoteConfig.Signing.OverrideRemotes(remoteServer.URL)
	remoteConfig.Signing.SetRemoteCAs(certPool)
	if isMutual {
		remoteConfig.Signing.SetClientCertKeyPairFromFile(testClientFile, testClientKeyFile)
	}
	verifyRemoteSign(t, remoteConfig)
}

func verifyRemoteSign(t *testing.T, remoteConfig *config.Config) {
	s := newRemoteSigner(t, remoteConfig.Signing)

	hosts := []string{"cloudflare.com"}
	for _, test := range testsuite.CSRTests {
		csr, err := ioutil.ReadFile(test.File)
		if err != nil {
			t.Fatal("CSR loading error:", err)
		}
		testSerial := big.NewInt(0x7007F)
		certBytes, err := s.Sign(signer.SignRequest{
			Hosts:   hosts,
			Request: string(csr),
			Serial:  testSerial,
		})
		if test.ErrorCallback != nil {
			test.ErrorCallback(t, err)
		} else {
			if err != nil {
				t.Fatalf("Expected no error. Got %s. Param %s %d", err.Error(), test.KeyAlgo, test.KeyLen)
			}
			cert, err := helpers.ParseCertificatePEM(certBytes)
			if err != nil {
				t.Fatal("Fail to parse returned certificate:", err)
			}
			sn := fmt.Sprintf("%X", cert.SerialNumber)
			if sn != "7007F" {
				t.Fatal("Serial Number was incorrect:", sn)
			}
		}
	}
}

func TestRemoteSignBadServerAndOverride(t *testing.T) {
	remoteServer := newTestSignServer(t, false, nil)
	defer closeTestServer(t, remoteServer)

	// remoteConfig contains port 80 that no test server will listen on
	remoteConfig := testsuite.NewConfig(t, []byte(validMinimalRemoteConfig))
	s := newRemoteSigner(t, remoteConfig.Signing)

	hosts := []string{"cloudflare.com"}
	csr, err := ioutil.ReadFile("../local/testdata/rsa2048.csr")
	if err != nil {
		t.Fatal("CSR loading error:", err)
	}

	_, err = s.Sign(signer.SignRequest{Hosts: hosts, Request: string(csr)})
	if err == nil {
		t.Fatal("Should return error")
	}

	remoteConfig.Signing.OverrideRemotes(remoteServer.URL[7:])
	s.SetPolicy(remoteConfig.Signing)
	certBytes, err := s.Sign(signer.SignRequest{
		Hosts:   hosts,
		Request: string(csr),
		Serial:  big.NewInt(1),
	})
	if err != nil {
		t.Fatalf("Expected no error. Got %s.", err.Error())
	}
	_, err = helpers.ParseCertificatePEM(certBytes)
	if err != nil {
		t.Fatal("Fail to parse returned certificate:", err)
	}

}

// helper functions
func newRemoteSigner(t *testing.T, policy *config.Signing) *Signer {
	s, err := NewSigner(policy)
	if err != nil {
		t.Fatal("fail to init remote signer:", err)
	}

	return s
}

func newTestSignHandler(t *testing.T) (h http.Handler) {
	h, err := newHandler(t, testCaFile, testCaKeyFile, "sign")
	if err != nil {
		t.Fatal(err)
	}
	return
}

func newTestInfoHandler(t *testing.T) (h http.Handler) {
	h, err := newHandler(t, testCaFile, testCaKeyFile, "info")
	if err != nil {
		t.Fatal(err)
	}
	return
}

func newTestServer(t *testing.T, path string, handler http.Handler, isTLS bool, certPool *x509.CertPool) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewUnstartedServer(mux)
	if isTLS {
		cert, err := tls.LoadX509KeyPair(testServerFile, testServerKeyFile)
		if err != nil {
			t.Fatal(err)
		}
		clientCertRequired := tls.NoClientCert
		if certPool != nil {
			clientCertRequired = tls.RequireAndVerifyClientCert
		}
		ts.TLS = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientCAs:    certPool,
			ClientAuth:   clientCertRequired,
		}
		ts.TLS.BuildNameToCertificate()
		ts.StartTLS()
	} else {
		ts.Start()
	}
	return ts
}

func newTestSignServer(t *testing.T, isTLS bool, certPool *x509.CertPool) *httptest.Server {
	ts := newTestServer(t, "/api/v1/cfssl/sign", newTestSignHandler(t), isTLS, certPool)
	t.Log(ts.URL)
	return ts
}

func newTestInfoServer(t *testing.T, isTLS bool, certPool *x509.CertPool) *httptest.Server {
	ts := newTestServer(t, "/api/v1/cfssl/info", newTestInfoHandler(t), isTLS, certPool)
	t.Log(ts.URL)
	return ts
}

func closeTestServer(t *testing.T, ts *httptest.Server) {
	t.Log("Finalizing test server.")
	ts.Close()
}

// newHandler generates a new sign handler (or info handler) using the certificate
// authority private key and certficate to sign certificates.
func newHandler(t *testing.T, caFile, caKeyFile, op string) (http.Handler, error) {
	var expiry = 1 * time.Minute
	var CAConfig = &config.Config{
		Signing: &config.Signing{
			Profiles: map[string]*config.SigningProfile{
				"signature": {
					Usage:  []string{"digital signature"},
					Expiry: expiry,
				},
			},
			Default: &config.SigningProfile{
				Usage:        []string{"cert sign", "crl sign"},
				ExpiryString: "43800h",
				Expiry:       expiry,
				CAConstraint: config.CAConstraint{IsCA: true},

				ClientProvidesSerialNumbers: true,
			},
		},
	}
	s, err := local.NewSignerFromFile(testCaFile, testCaKeyFile, CAConfig.Signing)
	if err != nil {
		t.Fatal(err)
	}
	if op == "sign" {
		return apisign.NewHandlerFromSigner(s)
	} else if op == "info" {
		return apiinfo.NewHandler(s)
	}

	t.Fatal("Bad op code")
	return nil, nil
}
