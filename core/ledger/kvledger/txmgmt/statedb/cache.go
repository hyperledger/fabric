/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"github.com/VictoriaMetrics/fastcache"
)

// Cache holds both the system and user cache
type Cache struct {
	sysCache *fastcache.Cache
	usrCache *fastcache.Cache
}

// New creates a Cache. The cache consists of both system state cache (for lscc, _lifecycle)
// and user state cache (for all user deployed chaincodes). The size of the
// system state cache is 64 MB, by default. The size of the user state cache, in terms of MB, is
// specified via usrCacheSize parameter. Note that the fastcache allocates memory
// only in the multiples of 32 MB (due to 512 buckets & an equal number of 64 KB chunks per bucket).
// If the usrCacheSize is not a multiple of 32 MB, the fastcache would round the size
// to the next multiple of 32 MB.
func New(usrCacheSize int) *Cache {
	cache := &Cache{}
	// By default, 64 MB is allocated for the system cache
	cache.sysCache = fastcache.New(64 * 1024 * 1024)

	// User passed size is used to allocate memory for the user cache
	if usrCacheSize <= 0 {
		return cache
	}
	cache.usrCache = fastcache.New(usrCacheSize * 1024 * 1024)
	return cache
}
