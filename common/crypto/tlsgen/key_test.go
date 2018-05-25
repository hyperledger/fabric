/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCertEncoding(t *testing.T) {
	pair, err := newCertKeyPair(false, false, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, pair)
	assert.NotEmpty(t, pair.PrivKeyString())
	assert.NotEmpty(t, pair.PubKeyString())
	pair2, err := CertKeyPairFromString(pair.PrivKeyString(), pair.PubKeyString())
	assert.Equal(t, pair.Key, pair2.Key)
	assert.Equal(t, pair.Cert, pair2.Cert)
}

func TestLoadCert(t *testing.T) {
	pair, err := newCertKeyPair(false, false, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, pair)
	tlsCertPair, err := tls.X509KeyPair(pair.Cert, pair.Key)
	assert.NoError(t, err)
	assert.NotNil(t, tlsCertPair)
	block, _ := pem.Decode(pair.Cert)
	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err)
	assert.NotNil(t, cert)
}
