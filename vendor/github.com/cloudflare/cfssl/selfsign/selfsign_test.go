package selfsign

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/helpers"
)

const (
	keyFile = "testdata/localhost.key"
	csrFile = "testdata/localhost.csr"
)

func TestDefaultSign(t *testing.T) {
	csrBytes, err := ioutil.ReadFile(csrFile)
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, err := ioutil.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}

	priv, err := helpers.ParsePrivateKeyPEM(keyBytes)
	if err != nil {
		t.Fatal(err)
	}

	profile := config.DefaultConfig()
	profile.Expiry = 10 * time.Hour

	_, err = Sign(priv, csrBytes, profile)
	if err != nil {
		t.Fatal(err)
	}
}
