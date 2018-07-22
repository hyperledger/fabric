/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/hex"
	"sync"

	"github.com/hyperledger/fabric/common/util"
)

// MemoizeSigner signs messages with the same signature
// if the message was signed recently
type MemoizeSigner struct {
	maxEntries uint
	sync.RWMutex
	memory map[string][]byte
	sign   Signer
}

// NewMemoizeSigner creates a new MemoizeSigner that signs
// message with the given sign function
func NewMemoizeSigner(signFunc Signer, maxEntries uint) *MemoizeSigner {
	return &MemoizeSigner{
		maxEntries: maxEntries,
		memory:     make(map[string][]byte),
		sign:       signFunc,
	}
}

// Signer signs a message and returns the signature and nil,
// or nil and error on failure
func (ms *MemoizeSigner) Sign(msg []byte) ([]byte, error) {
	sig, isInMemory := ms.lookup(msg)
	if isInMemory {
		return sig, nil
	}
	sig, err := ms.sign(msg)
	if err != nil {
		return nil, err
	}
	ms.memorize(msg, sig)
	return sig, nil
}

// lookup looks up the given message in memory and returns
// the signature, if the message is in memory
func (ms *MemoizeSigner) lookup(msg []byte) ([]byte, bool) {
	ms.RLock()
	defer ms.RUnlock()
	sig, exists := ms.memory[msgDigest(msg)]
	return sig, exists
}

func (ms *MemoizeSigner) memorize(msg, signature []byte) {
	if ms.maxEntries == 0 {
		return
	}
	ms.RLock()
	shouldShrink := len(ms.memory) >= (int)(ms.maxEntries)
	ms.RUnlock()

	if shouldShrink {
		ms.shrinkMemory()
	}
	ms.Lock()
	defer ms.Unlock()
	ms.memory[msgDigest(msg)] = signature

}

// evict evicts random messages from memory
// until its size is smaller than maxEntries
func (ms *MemoizeSigner) shrinkMemory() {
	ms.Lock()
	defer ms.Unlock()
	for len(ms.memory) > (int)(ms.maxEntries) {
		ms.evictFromMemory()
	}
}

// evictFromMemory evicts a random message from memory
func (ms *MemoizeSigner) evictFromMemory() {
	for dig := range ms.memory {
		delete(ms.memory, dig)
		return
	}
}

// msgDigest returns a digest of a given message
func msgDigest(msg []byte) string {
	return hex.EncodeToString(util.ComputeSHA256(msg))
}
