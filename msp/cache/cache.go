/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	pmsp "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
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
		return nil, errors.Errorf("Invalid passed MSP. It must be different from nil.")
	}

	theMsp := &cachedMSP{MSP: o}
	theMsp.deserializeIdentityCache = newSecondChanceCache(deserializeIdentityCacheSize)
	theMsp.satisfiesPrincipalCache = newSecondChanceCache(satisfiesPrincipalCacheSize)
	theMsp.validateIdentityCache = newSecondChanceCache(validateIdentityCacheSize)

	return theMsp, nil
}

type cachedMSP struct {
	msp.MSP

	// cache for DeserializeIdentity.
	deserializeIdentityCache *secondChanceCache

	// cache for validateIdentity
	validateIdentityCache *secondChanceCache

	// basically a map of principals=>identities=>stringified to booleans
	// specifying whether this identity satisfies this principal
	satisfiesPrincipalCache *secondChanceCache
}

type cachedIdentity struct {
	msp.Identity
	cache *cachedMSP
}

func (id *cachedIdentity) SatisfiesPrincipal(principal *pmsp.MSPPrincipal) error {
	return id.cache.SatisfiesPrincipal(id.Identity, principal)
}

func (id *cachedIdentity) Validate() error {
	return id.cache.Validate(id.Identity)
}

func (c *cachedMSP) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	id, ok := c.deserializeIdentityCache.get(string(serializedIdentity))
	if ok {
		return &cachedIdentity{
			cache:    c,
			Identity: id.(msp.Identity),
		}, nil
	}

	id, err := c.MSP.DeserializeIdentity(serializedIdentity)
	if err == nil {
		c.deserializeIdentityCache.add(string(serializedIdentity), id)
		return &cachedIdentity{
			cache:    c,
			Identity: id.(msp.Identity),
		}, nil
	}
	return nil, err
}

func (c *cachedMSP) Setup(config *pmsp.MSPConfig) error {
	c.cleanCache()
	return c.MSP.Setup(config)
}

func (c *cachedMSP) Validate(id msp.Identity) error {
	identifier := id.GetIdentifier()
	key := identifier.Mspid + ":" + identifier.Id

	_, ok := c.validateIdentityCache.get(key)
	if ok {
		// cache only stores if the identity is valid.
		return nil
	}

	err := c.MSP.Validate(id)
	if err == nil {
		c.validateIdentityCache.add(key, true)
	}

	return err
}

func (c *cachedMSP) SatisfiesPrincipal(id msp.Identity, principal *pmsp.MSPPrincipal) error {
	identifier := id.GetIdentifier()
	identityKey := identifier.Mspid + ":" + identifier.Id
	principalKey := string(principal.PrincipalClassification) + string(principal.Principal)
	key := identityKey + principalKey

	v, ok := c.satisfiesPrincipalCache.get(key)
	if ok {
		if v == nil {
			return nil
		}

		return v.(error)
	}

	err := c.MSP.SatisfiesPrincipal(id, principal)

	c.satisfiesPrincipalCache.add(key, err)
	return err
}

func (c *cachedMSP) cleanCache() {
	c.deserializeIdentityCache = newSecondChanceCache(deserializeIdentityCacheSize)
	c.satisfiesPrincipalCache = newSecondChanceCache(satisfiesPrincipalCacheSize)
	c.validateIdentityCache = newSecondChanceCache(validateIdentityCacheSize)
}
