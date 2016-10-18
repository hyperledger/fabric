package localca

import (
	"encoding/pem"
	"io/ioutil"
	"os"
	"testing"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/selfsign"
	"github.com/kisom/goutils/assert"
)

func tempName() (string, error) {
	tmpf, err := ioutil.TempFile("", "transport_cachedkp_")
	if err != nil {
		return "", err
	}

	name := tmpf.Name()
	tmpf.Close()
	return name, nil
}

func testGenerateKeypair(req *csr.CertificateRequest) (keyFile, certFile string, err error) {
	fail := func(err error) (string, string, error) {
		if keyFile != "" {
			os.Remove(keyFile)
		}
		if certFile != "" {
			os.Remove(certFile)
		}
		return "", "", err
	}

	keyFile, err = tempName()
	if err != nil {
		return fail(err)
	}

	certFile, err = tempName()
	if err != nil {
		return fail(err)
	}

	csrPEM, keyPEM, err := csr.ParseRequest(req)
	if err != nil {
		return fail(err)
	}

	if err = ioutil.WriteFile(keyFile, keyPEM, 0644); err != nil {
		return fail(err)
	}

	priv, err := helpers.ParsePrivateKeyPEM(keyPEM)
	if err != nil {
		return fail(err)
	}

	cert, err := selfsign.Sign(priv, csrPEM, config.DefaultConfig())
	if err != nil {
		return fail(err)
	}

	if err = ioutil.WriteFile(certFile, cert, 0644); err != nil {
		return fail(err)
	}

	return
}

func TestEncodePEM(t *testing.T) {
	p := &pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: []byte(`¯\_(ツ)_/¯`),
	}
	t.Logf("PEM:\n%s\n\n", string(pem.EncodeToMemory(p)))
}

func TestLoadSigner(t *testing.T) {
	lca := &CA{}
	certPEM, csrPEM, keyPEM, err := initca.New(ExampleRequest())
	assert.NoErrorT(t, err)

	_, err = lca.CACertificate()
	assert.ErrorEqT(t, errNotSetup, err)

	_, err = lca.SignCSR(csrPEM)
	assert.ErrorEqT(t, errNotSetup, err)

	lca.KeyFile, err = tempName()
	assert.NoErrorT(t, err)
	defer os.Remove(lca.KeyFile)

	lca.CertFile, err = tempName()
	assert.NoErrorT(t, err)
	defer os.Remove(lca.CertFile)

	err = ioutil.WriteFile(lca.KeyFile, keyPEM, 0644)
	assert.NoErrorT(t, err)

	err = ioutil.WriteFile(lca.CertFile, certPEM, 0644)
	assert.NoErrorT(t, err)

	err = Load(lca, ExampleSigningConfig())
	assert.NoErrorT(t, err)
}

var testRequest = &csr.CertificateRequest{
	CN: "Transport Test Identity",
	KeyRequest: &csr.BasicKeyRequest{
		A: "ecdsa",
		S: 256,
	},
	Hosts: []string{"127.0.0.1"},
}

func TestNewSigner(t *testing.T) {
	req := ExampleRequest()
	lca, err := New(req, ExampleSigningConfig())
	assert.NoErrorT(t, err)

	csrPEM, _, err := csr.ParseRequest(testRequest)
	assert.NoErrorT(t, err)

	certPEM, err := lca.SignCSR(csrPEM)
	assert.NoErrorT(t, err)

	_, err = helpers.ParseCertificatePEM(certPEM)
	assert.NoErrorT(t, err)

	certPEM, err = lca.CACertificate()
	assert.NoErrorT(t, err)

	cert, err := helpers.ParseCertificatePEM(certPEM)
	assert.NoErrorT(t, err)

	assert.BoolT(t, cert.Subject.CommonName == req.CN,
		"common names don't match")

	lca.Toggle()
	_, err = lca.SignCSR(csrPEM)
	assert.ErrorEqT(t, errDisabled, err)
	lca.Toggle()

	_, err = lca.SignCSR(certPEM)
	assert.ErrorT(t, err, "shouldn't be able to sign non-CSRs")

	p := &pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: []byte(`¯\_(ツ)_/¯`),
	}
	junkCSR := pem.EncodeToMemory(p)

	_, err = lca.SignCSR(junkCSR)
	assert.ErrorT(t, err, "signing a junk CSR should fail")
	t.Logf("error: %s", err)
}
