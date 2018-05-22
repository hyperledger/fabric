/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestX509CertExpiresAt(t *testing.T) {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "cert.pem"))
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: certBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := ExpiresAt(serializedIdentity)
	assert.Equal(t, time.Date(2027, 8, 17, 12, 19, 48, 0, time.UTC), expirationTime)
}

func TestX509InvalidCertExpiresAt(t *testing.T) {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "badCert.pem"))
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: certBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := ExpiresAt(serializedIdentity)
	assert.True(t, expirationTime.IsZero())
}

func TestIdemixIdentityExpiresAt(t *testing.T) {
	idemixId := &msp.SerializedIdemixIdentity{
		NymX: []byte{1, 2, 3},
		NymY: []byte{1, 2, 3},
		Ou:   []byte("OU1"),
	}
	idemixBytes, err := proto.Marshal(idemixId)
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: idemixBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := ExpiresAt(serializedIdentity)
	assert.True(t, expirationTime.IsZero())
}

func TestInvalidIdentityExpiresAt(t *testing.T) {
	expirationTime := ExpiresAt([]byte{1, 2, 3})
	assert.True(t, expirationTime.IsZero())
}
