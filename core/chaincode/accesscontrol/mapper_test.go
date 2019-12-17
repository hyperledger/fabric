/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/stretchr/testify/assert"
)

func TestPurge(t *testing.T) {
	ca, _ := tlsgen.NewCA()
	backupTTL := ttl
	defer func() {
		ttl = backupTTL
	}()
	ttl = time.Second
	m := newCertMapper(ca.NewClientCertKeyPair)
	k, err := m.genCert("A")
	assert.NoError(t, err)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	hash, err := cryptoProvider.Hash(k.TLSCert.Raw, &bccsp.SHA256Opts{})
	assert.NoError(t, err)
	assert.Equal(t, "A", m.lookup(certHash(hash)))
	time.Sleep(time.Second * 3)
	assert.Empty(t, m.lookup(certHash(hash)))
}
