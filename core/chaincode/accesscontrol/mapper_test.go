/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/stretchr/testify/assert"
)

func TestCertEncoding(t *testing.T) {
	pair, err := newCertKeyPair(false, false, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, pair)
	assert.NotEmpty(t, pair.privKeyString())
	assert.NotEmpty(t, pair.pubKeyString())
	pair2, err := certKeyPairFromString(pair.privKeyString(), pair.pubKeyString())
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

func TestPurge(t *testing.T) {
	ca, _ := NewCA()
	backupTTL := ttl
	defer func() {
		ttl = backupTTL
	}()
	ttl = time.Second
	m := newCertMapper(ca.newClientCertKeyPair)
	k, err := m.genCert("A")
	assert.NoError(t, err)
	hash, _ := factory.GetDefault().Hash(k.cert.Raw, &bccsp.SHA256Opts{})
	assert.Equal(t, "A", m.lookup(certHash(hash)))
	time.Sleep(time.Second * 3)
	assert.Empty(t, m.lookup(certHash(hash)))
}
