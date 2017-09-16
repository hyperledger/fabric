/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"

	"github.com/golang/groupcache/lru"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	pmsp "github.com/hyperledger/fabric/protos/msp"
)

const (
	deserializeIdentityCacheSize = 100
	validateIdentityCacheSize    = 100
	satisfiesPrincipalCacheSize  = 100
)

var mspLogger = flogging.MustGetLogger("msp")

func New(o msp.MSP) (msp.MSP, error) {
	mspLogger.Debugf("Creating Cache-MSP instance")
	if o == nil {
		return nil, fmt.Errorf("Invalid passed MSP. It must be different from nil.")
	}

	theMsp := &cachedMSP{MSP: o}
	theMsp.deserializeIdentityCache = lru.New(deserializeIdentityCacheSize)
	theMsp.satisfiesPrincipalCache = lru.New(satisfiesPrincipalCacheSize)
	theMsp.validateIdentityCache = lru.New(validateIdentityCacheSize)

	return theMsp, nil
}

type cachedMSP struct {
	msp.MSP

	// cache for DeserializeIdentity.
	deserializeIdentityCache *lru.Cache

	dicMutex sync.Mutex // synchronize access to cache

	// cache for validateIdentity
	validateIdentityCache *lru.Cache

	vicMutex sync.Mutex // synchronize access to cache

	// basically a map of principals=>identities=>stringified to booleans
	// specifying whether this identity satisfies this principal
	satisfiesPrincipalCache *lru.Cache

	spcMutex sync.Mutex // synchronize access to cache
}

func (c *cachedMSP) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	c.dicMutex.Lock()
	cached, ok := c.deserializeIdentityCache.Get(string(serializedIdentity))
	c.dicMutex.Unlock()
	if ok {
		return cached.(msp.Identity), nil
	}

	id, err := c.MSP.DeserializeIdentity(serializedIdentity)
	if err == nil {
		c.dicMutex.Lock()
		defer c.dicMutex.Unlock()
		c.deserializeIdentityCache.Add(string(serializedIdentity), id)
	}
	return id, err
}

func (c *cachedMSP) Setup(config *pmsp.MSPConfig) error {
	c.cleanCash()

	return c.MSP.Setup(config)
}

func (c *cachedMSP) Validate(id msp.Identity) error {
	identifier := id.GetIdentifier()
	key := string(identifier.Mspid + ":" + identifier.Id)

	c.vicMutex.Lock()
	_, ok := c.validateIdentityCache.Get(key)
	c.vicMutex.Unlock()
	if ok {
		// cache only stores if the identity is valid.
		return nil
	}

	err := c.MSP.Validate(id)
	if err == nil {
		c.vicMutex.Lock()
		defer c.vicMutex.Unlock()
		c.validateIdentityCache.Add(key, true)
	}

	return err
}

func (c *cachedMSP) SatisfiesPrincipal(id msp.Identity, principal *pmsp.MSPPrincipal) error {
	identifier := id.GetIdentifier()
	identityKey := string(identifier.Mspid + ":" + identifier.Id)
	principalKey := string(principal.PrincipalClassification) + string(principal.Principal)
	key := identityKey + principalKey

	c.spcMutex.Lock()
	v, ok := c.satisfiesPrincipalCache.Get(key)
	c.spcMutex.Unlock()
	if ok {
		if v == nil {
			return nil
		}

		return v.(error)
	}

	err := c.MSP.SatisfiesPrincipal(id, principal)

	c.spcMutex.Lock()
	defer c.spcMutex.Unlock()
	c.satisfiesPrincipalCache.Add(key, err)
	return err
}

func (c *cachedMSP) cleanCash() error {
	c.deserializeIdentityCache = lru.New(deserializeIdentityCacheSize)
	c.satisfiesPrincipalCache = lru.New(satisfiesPrincipalCacheSize)
	c.validateIdentityCache = lru.New(validateIdentityCacheSize)

	return nil
}
