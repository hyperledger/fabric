/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"math/rand"
	"testing"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
)

var sysNamespaces = []string{"lscc", "_lifecycle"}

func TestNewCache(t *testing.T) {
	c := newCache(32, sysNamespaces)
	expectedCache := &cache{
		sysCache:      fastcache.New(64 * 1024 * 1024),
		usrCache:      fastcache.New(32 * 1024 * 1024),
		sysNamespaces: sysNamespaces,
	}
	require.Equal(t, expectedCache, c)
	require.True(t, c.enabled("lscc"))
	require.True(t, c.enabled("_lifecycle"))
	require.True(t, c.enabled("xyz"))

	c = newCache(0, sysNamespaces)
	expectedCache = &cache{
		sysCache:      fastcache.New(64 * 1024 * 1024),
		usrCache:      nil,
		sysNamespaces: sysNamespaces,
	}
	require.Equal(t, expectedCache, c)
	require.True(t, c.enabled("lscc"))
	require.True(t, c.enabled("_lifecycle"))
	require.False(t, c.enabled("xyz"))
}

func TestGetPutState(t *testing.T) {
	cache := newCache(32, sysNamespaces)

	// test GetState
	v, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	// test PutState
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue1))

	v, err = cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v))
}

func TestGetPutStateWithBigPayloadIfKeyDoesNotExist(t *testing.T) {
	cache := newCache(32, sysNamespaces)

	expectedValue := &CacheValue{Value: []byte("value")}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue))
	v, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue, v))

	// test PutState with BigPayload
	token := make([]byte, (64*1024)+1)
	rand.Read(token)
	expectedValue1 := &CacheValue{Value: token}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue1))

	v, err = cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	// actually bigPayloads are not saved in cache, should return nil/nothing
	require.Nil(t, v)
}

func TestUpdateStatesWithSingleSmallAndSingleBigPayloads(t *testing.T) {
	cache := newCache(32, sysNamespaces)

	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue1))

	expectedValue2 := &CacheValue{Value: []byte("value2")}
	require.NoError(t, cache.putState("ch1", "ns1", "k2", expectedValue2))

	v1, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v1))

	v2, err := cache.getState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue2, v2))

	token := make([]byte, (64*1024)+1)
	rand.Read(token)

	expectedValue3 := &CacheValue{Value: []byte("value3")}
	expectedValue4 := &CacheValue{Value: token}

	updates := cacheUpdates{
		"ns1": cacheKVs{
			"k1": expectedValue3,
			"k2": expectedValue4,
		},
	}
	require.NoError(t, cache.UpdateStates("ch1", updates))

	v3, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue3, v3))

	v4, err := cache.getState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.Nil(t, v4)
}

func TestUpdateStates(t *testing.T) {
	cache := newCache(32, sysNamespaces)

	// create states for three namespaces (ns1, ns2, ns3)
	// each with two keys (k1, k2)
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue1))
	expectedValue2 := &CacheValue{Value: []byte("value2")}
	require.NoError(t, cache.putState("ch1", "ns1", "k2", expectedValue2))
	expectedValue3 := &CacheValue{Value: []byte("value3")}
	require.NoError(t, cache.putState("ch1", "ns2", "k1", expectedValue3))
	expectedValue4 := &CacheValue{Value: []byte("value4")}
	require.NoError(t, cache.putState("ch1", "ns2", "k2", expectedValue4))
	expectedValue5 := &CacheValue{Value: []byte("value5")}
	require.NoError(t, cache.putState("ch1", "ns3", "k1", expectedValue5))
	expectedValue6 := &CacheValue{Value: []byte("value6")}
	require.NoError(t, cache.putState("ch1", "ns3", "k2", expectedValue6))

	v1, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v1))
	v2, err := cache.getState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue2, v2))
	v3, err := cache.getState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue3, v3))
	v4, err := cache.getState("ch1", "ns2", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue4, v4))
	v5, err := cache.getState("ch1", "ns3", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue5, v5))
	v6, err := cache.getState("ch1", "ns3", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue6, v6))

	// delete (ns2, k1), (ns3, k1), and (ns3, k2) while updating others.
	// nil value represents a delete operation. A new entry (ns3, k3)
	// is also being passed but would not get added to the cache as the
	// entry does not exist in cache already.
	expectedValue7 := &CacheValue{Value: []byte("value7")}
	expectedValue8 := &CacheValue{Value: []byte("value8")}
	expectedValue9 := &CacheValue{Value: []byte("value9")}
	expectedValue10 := &CacheValue{Value: []byte("value10")}
	updates := cacheUpdates{
		"ns1": cacheKVs{
			"k1": expectedValue7,
			"k2": expectedValue8,
		},
		"ns2": cacheKVs{
			"k1": nil,
			"k2": expectedValue9,
		},
		"ns3": cacheKVs{
			"k1": nil,
			"k2": nil,
			"k3": expectedValue10,
		},
	}

	require.NoError(t, cache.UpdateStates("ch1", updates))

	v7, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue7, v7))

	v8, err := cache.getState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue8, v8))

	v9, err := cache.getState("ch1", "ns2", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue9, v9))

	v, err := cache.getState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.getState("ch1", "ns3", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.getState("ch1", "ns3", "k2")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.getState("ch1", "ns3", "k3")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestCacheReset(t *testing.T) {
	cache := newCache(32, sysNamespaces)

	// create states for three namespaces (ns1, ns2, ns3)
	// each with two keys (k1, k2)
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.putState("ch1", "ns1", "k1", expectedValue1))

	expectedValue2 := &CacheValue{Value: []byte("value2")}
	require.NoError(t, cache.putState("ch1", "ns2", "k1", expectedValue2))

	expectedValue3 := &CacheValue{Value: []byte("value3")}
	require.NoError(t, cache.putState("ch1", "lscc", "k1", expectedValue3))

	v1, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v1))

	v2, err := cache.getState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue2, v2))

	v3, err := cache.getState("ch1", "lscc", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue3, v3))

	cache.Reset()

	v, err := cache.getState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = cache.getState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = cache.getState("ch1", "lscc", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestCacheUpdates(t *testing.T) {
	u := make(cacheUpdates)
	u.add("ns1", cacheKVs{
		"k1": &CacheValue{Value: []byte("v1")},
		"k2": &CacheValue{Value: []byte("v2")},
	})

	u.add("ns1", cacheKVs{
		"k3": &CacheValue{Value: []byte("v1")},
		"k4": &CacheValue{Value: []byte("v2")},
	})

	u.add("ns2", cacheKVs{
		"k1": &CacheValue{Value: []byte("v1")},
		"k2": &CacheValue{Value: []byte("v2")},
	})

	expectedCacheUpdates := cacheUpdates{
		"ns1": cacheKVs{
			"k1": &CacheValue{Value: []byte("v1")},
			"k2": &CacheValue{Value: []byte("v2")},
			"k3": &CacheValue{Value: []byte("v1")},
			"k4": &CacheValue{Value: []byte("v2")},
		},

		"ns2": cacheKVs{
			"k1": &CacheValue{Value: []byte("v1")},
			"k2": &CacheValue{Value: []byte("v2")},
		},
	}

	require.Equal(t, expectedCacheUpdates, u)
}
