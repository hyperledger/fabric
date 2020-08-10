/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"encoding/hex"
	"sync"

	"github.com/miekg/pkcs11"
)

type p11Key struct {
	privH  pkcs11.ObjectHandle
	pubH   pkcs11.ObjectHandle
	pubKey *ecdsa.PublicKey
}

type keyCache struct {
	mu      sync.RWMutex
	p11Keys map[string]p11Key
}

func (k *keyCache) get(ski []byte) (p11Key, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	h, ok := k.p11Keys[hex.EncodeToString(ski)]
	return h, ok
}

func (k *keyCache) set(ski []byte, key p11Key) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.p11Keys[hex.EncodeToString(ski)] = key

}

func (k *keyCache) clear() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.p11Keys = make(map[string]p11Key)
}
