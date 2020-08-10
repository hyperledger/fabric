// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"testing"

	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/assert"
)

func TestKeyCache(t *testing.T) {

	kc := &keyCache{
		p11Keys: make(map[string]p11Key),
	}

	k, ok := kc.get([]byte("ski"))
	assert.Equal(t, false, ok)

	k1 := p11Key{
		privH: pkcs11.ObjectHandle(1),
		pubH:  pkcs11.ObjectHandle(1),
		pubKey: &ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
	}

	kc.set([]byte("ski"), k1)
	k, ok = kc.get([]byte("ski"))
	assert.Equal(t, true, ok)
	assert.Equal(t, k1, k)

	k2 := p11Key{
		privH: pkcs11.ObjectHandle(2),
		pubH:  pkcs11.ObjectHandle(2),
		pubKey: &ecdsa.PublicKey{
			Curve: elliptic.P384(),
		},
	}

	kc.set([]byte("ski2"), k2)
	k, ok = kc.get([]byte("ski2"))
	assert.Equal(t, true, ok)
	assert.Equal(t, k2, k)

	kc.clear()
	assert.Equal(t, 0, len(kc.p11Keys))
	k, ok = kc.get([]byte("ski"))
	assert.Equal(t, false, ok)
	k, ok = kc.get([]byte("ski2"))
	assert.Equal(t, false, ok)

}
