/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/asn1"
	"encoding/hex"
	"sync"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	defaultMaxCacheSize   = 1000
	defaultRetentionRatio = 0.75
)

// asBytes is the function that is used to marshal protoutil.SignedData to bytes
var asBytes = asn1.Marshal

type acSupport interface {
	// Eligible returns whether the given peer is eligible for receiving
	// service from the discovery service for a given channel
	EligibleForService(channel string, data protoutil.SignedData) error

	// ConfigSequence returns the configuration sequence of the given channel
	ConfigSequence(channel string) uint64
}

type authCacheConfig struct {
	enabled bool
	// maxCacheSize is the maximum size of the cache, after which
	// a purge takes place
	maxCacheSize int
	// purgeRetentionRatio is the % of entries that remain in the cache
	// after the cache is purged due to overpopulation
	purgeRetentionRatio float64
}

// authCache defines an interface that authenticates a request in a channel context,
// and does memoization of invocations
type authCache struct {
	credentialCache map[string]*accessCache
	acSupport
	sync.RWMutex
	conf authCacheConfig
}

func newAuthCache(s acSupport, conf authCacheConfig) *authCache {
	return &authCache{
		acSupport:       s,
		credentialCache: make(map[string]*accessCache),
		conf:            conf,
	}
}

// Eligible returns whether the given peer is eligible for receiving
// service from the discovery service for a given channel
func (ac *authCache) EligibleForService(channel string, data protoutil.SignedData) error {
	if !ac.conf.enabled {
		return ac.acSupport.EligibleForService(channel, data)
	}
	// Check whether we already have a cache for this channel
	ac.RLock()
	cache := ac.credentialCache[channel]
	ac.RUnlock()
	if cache == nil {
		// Cache for given channel wasn't found, so create a new one
		ac.Lock()
		cache = ac.newAccessCache(channel)
		// And store the cache instance.
		ac.credentialCache[channel] = cache
		ac.Unlock()
	}
	return cache.EligibleForService(data)
}

type accessCache struct {
	sync.RWMutex
	channel      string
	ac           *authCache
	lastSequence uint64
	entries      map[string]error
}

func (ac *authCache) newAccessCache(channel string) *accessCache {
	return &accessCache{
		channel: channel,
		ac:      ac,
		entries: make(map[string]error),
	}
}

func (cache *accessCache) EligibleForService(data protoutil.SignedData) error {
	key, err := signedDataToKey(data)
	if err != nil {
		logger.Warningf("Failed computing key of signed data: +%v", err)
		return errors.Wrap(err, "failed computing key of signed data")
	}
	currSeq := cache.ac.acSupport.ConfigSequence(cache.channel)
	if cache.isValid(currSeq) {
		foundInCache, isEligibleErr := cache.lookup(key)
		if foundInCache {
			return isEligibleErr
		}
	} else {
		cache.configChange(currSeq)
	}

	// Make sure the cache doesn't overpopulate.
	// It might happen that it overgrows the maximum size due to concurrent
	// goroutines waiting on the lock above, but that's acceptable.
	cache.purgeEntriesIfNeeded()

	// Compute the eligibility of the client for the service
	err = cache.ac.acSupport.EligibleForService(cache.channel, data)
	cache.Lock()
	defer cache.Unlock()
	// Check if the sequence hasn't changed since last time
	if currSeq != cache.ac.acSupport.ConfigSequence(cache.channel) {
		// The sequence at which we computed the eligibility might have changed,
		// so we can't put it into the cache because a more fresh computation result
		// might already be present in the cache by now, and we don't want to override it
		// with a stale computation result, so just return the result.
		return err
	}
	// Else, the eligibility of the client has been computed under the latest sequence,
	// so store the computation result in the cache
	cache.entries[key] = err
	return err
}

func (cache *accessCache) isPurgeNeeded() bool {
	cache.RLock()
	defer cache.RUnlock()
	return len(cache.entries)+1 > cache.ac.conf.maxCacheSize
}

func (cache *accessCache) purgeEntriesIfNeeded() {
	if !cache.isPurgeNeeded() {
		return
	}

	cache.Lock()
	defer cache.Unlock()

	maxCacheSize := cache.ac.conf.maxCacheSize
	purgeRatio := cache.ac.conf.purgeRetentionRatio
	entries2evict := maxCacheSize - int(purgeRatio*float64(maxCacheSize))

	for key := range cache.entries {
		if entries2evict == 0 {
			return
		}
		entries2evict--
		delete(cache.entries, key)
	}
}

func (cache *accessCache) isValid(currSeq uint64) bool {
	cache.RLock()
	defer cache.RUnlock()
	return currSeq == cache.lastSequence
}

func (cache *accessCache) configChange(currSeq uint64) {
	cache.Lock()
	defer cache.Unlock()
	cache.lastSequence = currSeq
	// Invalidate entries
	cache.entries = make(map[string]error)
}

func (cache *accessCache) lookup(key string) (cacheHit bool, lookupResult error) {
	cache.RLock()
	defer cache.RUnlock()

	lookupResult, cacheHit = cache.entries[key]
	return
}

func signedDataToKey(data protoutil.SignedData) (string, error) {
	b, err := asBytes(data)
	if err != nil {
		return "", errors.Wrap(err, "failed marshaling signed data")
	}
	return hex.EncodeToString(util.ComputeSHA256(b)), nil
}
