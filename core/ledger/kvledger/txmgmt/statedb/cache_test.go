/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"testing"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/stretchr/testify/assert"
)

func TestNewCache(t *testing.T) {
	cache := New(10)
	expectedCache := &Cache{
		sysCache: fastcache.New(64 * 1024 * 1024),
		usrCache: fastcache.New(10 * 1024 * 1024),
	}
	assert.Equal(t, expectedCache, cache)

	cache = New(0)
	expectedCache = &Cache{
		sysCache: fastcache.New(64 * 1024 * 1024),
		usrCache: nil,
	}
	assert.Equal(t, expectedCache, cache)
}
