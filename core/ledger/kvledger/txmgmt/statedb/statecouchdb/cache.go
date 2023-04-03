/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"github.com/VictoriaMetrics/fastcache"
	"github.com/golang/protobuf/proto"
)

var keySep = []byte{0x00}

// cache holds both the system and user cache
type cache struct {
	sysCache      *fastcache.Cache
	usrCache      *fastcache.Cache
	sysNamespaces []string
}

// newCache creates a Cache. The cache consists of both system state cache (for lscc, _lifecycle)
// and user state cache (for all user deployed chaincodes). The size of the
// system state cache is 64 MB, by default. The size of the user state cache, in terms of MB, is
// specified via usrCacheSize parameter. Note that the maximum memory consumption of fastcache
// would be in the multiples of 32 MB (due to 512 buckets & an equal number of 64 KB chunks per bucket).
// If the usrCacheSizeMBs is not a multiple of 32 MB, the fastcache would round the size
// to the next multiple of 32 MB.
func newCache(usrCacheSizeMBs int, sysNamespaces []string) *cache {
	cache := &cache{}
	// By default, 64 MB is allocated for the system cache
	cache.sysCache = fastcache.New(64 * 1024 * 1024)
	cache.sysNamespaces = sysNamespaces

	// User passed size is used to allocate memory for the user cache
	if usrCacheSizeMBs <= 0 {
		return cache
	}
	cache.usrCache = fastcache.New(usrCacheSizeMBs * 1024 * 1024)
	return cache
}

// enabled returns true if the cache is enabled for a given namespace.
// Namespace can be of two types: system namespace (such as lscc) and user
// namespace (all user's chaincode states).
func (c *cache) enabled(namespace string) bool {
	for _, ns := range c.sysNamespaces {
		if namespace == ns {
			return true
		}
	}
	return c.usrCache != nil
}

// getState returns the value for a given namespace and key from
// a cache associated with the chainID.
func (c *cache) getState(chainID, namespace, key string) (*CacheValue, error) {
	cache := c.getCache(namespace)
	if cache == nil {
		return nil, nil
	}

	cacheKey := constructCacheKey(chainID, namespace, key)

	valBytes, exist := cache.HasGet(nil, cacheKey)
	if !exist {
		return nil, nil
	}

	cacheValue := &CacheValue{}
	if err := proto.Unmarshal(valBytes, cacheValue); err != nil {
		return nil, err
	}
	return cacheValue, nil
}

// PutState stores a given value in a cache associated with the chainID.
func (c *cache) putState(chainID, namespace, key string, cacheValue *CacheValue) error {
	cache := c.getCache(namespace)
	if cache == nil {
		return nil
	}

	cacheKey := constructCacheKey(chainID, namespace, key)
	valBytes, err := proto.Marshal(cacheValue)
	if err != nil {
		return err
	}

	if cache.Has(cacheKey) {
		cache.Del(cacheKey)
	}

	cache.Set(cacheKey, valBytes)
	return nil
}

// CacheUpdates is a map from a namespace to a set of cache KV updates
type cacheUpdates map[string]cacheKVs

// CacheKVs is a map from a key to a cache value
type cacheKVs map[string]*CacheValue

// Add adds the given cacheKVs to the CacheUpdates
func (u cacheUpdates) add(namespace string, ckvs cacheKVs) {
	nsu, ok := u[namespace]
	if !ok {
		nsu = cacheKVs{}
		u[namespace] = nsu
	}

	for k, v := range ckvs {
		nsu[k] = v
	}
}

// UpdateStates updates only the existing entries in the cache associated with
// the chainID.
func (c *cache) UpdateStates(chainID string, updates cacheUpdates) error {
	for ns, kvs := range updates {
		cache := c.getCache(ns)
		if cache == nil {
			continue
		}

		for key, newVal := range kvs {
			cacheKey := constructCacheKey(chainID, ns, key)
			if newVal == nil {
				cache.Del(cacheKey)
				continue
			}
			if cache.Has(cacheKey) {
				newValBytes, err := proto.Marshal(newVal)
				if err != nil {
					return err
				}
				cache.Del(cacheKey)
				cache.Set(cacheKey, newValBytes)
			}
		}
	}
	return nil
}

// Reset removes all the items from the cache.
func (c *cache) Reset() {
	c.sysCache.Reset()
	if c.usrCache != nil {
		c.usrCache.Reset()
	}
}

func (c *cache) getCache(namespace string) *fastcache.Cache {
	for _, ns := range c.sysNamespaces {
		if namespace == ns {
			return c.sysCache
		}
	}
	return c.usrCache
}

func constructCacheKey(chainID, namespace, key string) []byte {
	var cacheKey []byte
	cacheKey = append(cacheKey, []byte(chainID)...)
	cacheKey = append(cacheKey, keySep...)
	cacheKey = append(cacheKey, []byte(namespace)...)
	cacheKey = append(cacheKey, keySep...)
	return append(cacheKey, []byte(key)...)
}
