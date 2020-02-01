/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"testing"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
)

var sysNamespaces = []string{"lscc", "_lifecycle"}

func TestNewCache(t *testing.T) {
	cache := NewCache(32, sysNamespaces)
	expectedCache := &Cache{
		sysCache:      fastcache.New(64 * 1024 * 1024),
		usrCache:      fastcache.New(32 * 1024 * 1024),
		sysNamespaces: sysNamespaces,
	}
	require.Equal(t, expectedCache, cache)
	require.True(t, cache.Enabled("lscc"))
	require.True(t, cache.Enabled("_lifecycle"))
	require.True(t, cache.Enabled("xyz"))

	cache = NewCache(0, sysNamespaces)
	expectedCache = &Cache{
		sysCache:      fastcache.New(64 * 1024 * 1024),
		usrCache:      nil,
		sysNamespaces: sysNamespaces,
	}
	require.Equal(t, expectedCache, cache)
	require.True(t, cache.Enabled("lscc"))
	require.True(t, cache.Enabled("_lifecycle"))
	require.False(t, cache.Enabled("xyz"))
}

func TestGetPutState(t *testing.T) {
	cache := NewCache(32, sysNamespaces)

	// test GetState
	v, err := cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	// test PutState
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.PutState("ch1", "ns1", "k1", expectedValue1))

	v, err = cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v))
}

func TestUpdateStates(t *testing.T) {
	cache := NewCache(32, sysNamespaces)

	// create states for three namespaces (ns1, ns2, ns3)
	// each with two keys (k1, k2)
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.PutState("ch1", "ns1", "k1", expectedValue1))
	expectedValue2 := &CacheValue{Value: []byte("value2")}
	require.NoError(t, cache.PutState("ch1", "ns1", "k2", expectedValue2))
	expectedValue3 := &CacheValue{Value: []byte("value3")}
	require.NoError(t, cache.PutState("ch1", "ns2", "k1", expectedValue3))
	expectedValue4 := &CacheValue{Value: []byte("value4")}
	require.NoError(t, cache.PutState("ch1", "ns2", "k2", expectedValue4))
	expectedValue5 := &CacheValue{Value: []byte("value5")}
	require.NoError(t, cache.PutState("ch1", "ns3", "k1", expectedValue5))
	expectedValue6 := &CacheValue{Value: []byte("value6")}
	require.NoError(t, cache.PutState("ch1", "ns3", "k2", expectedValue6))

	v1, err := cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v1))
	v2, err := cache.GetState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue2, v2))
	v3, err := cache.GetState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue3, v3))
	v4, err := cache.GetState("ch1", "ns2", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue4, v4))
	v5, err := cache.GetState("ch1", "ns3", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue5, v5))
	v6, err := cache.GetState("ch1", "ns3", "k2")
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
	updates := CacheUpdates{
		"ns1": CacheKVs{
			"k1": expectedValue7,
			"k2": expectedValue8,
		},
		"ns2": CacheKVs{
			"k1": nil,
			"k2": expectedValue9,
		},
		"ns3": CacheKVs{
			"k1": nil,
			"k2": nil,
			"k3": expectedValue10,
		},
	}

	require.NoError(t, cache.UpdateStates("ch1", updates))

	v7, err := cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue7, v7))

	v8, err := cache.GetState("ch1", "ns1", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue8, v8))

	v9, err := cache.GetState("ch1", "ns2", "k2")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue9, v9))

	v, err := cache.GetState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.GetState("ch1", "ns3", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.GetState("ch1", "ns3", "k2")
	require.NoError(t, err)
	require.Nil(t, v)
	v, err = cache.GetState("ch1", "ns3", "k3")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestCacheReset(t *testing.T) {
	cache := NewCache(32, sysNamespaces)

	// create states for three namespaces (ns1, ns2, ns3)
	// each with two keys (k1, k2)
	expectedValue1 := &CacheValue{Value: []byte("value1")}
	require.NoError(t, cache.PutState("ch1", "ns1", "k1", expectedValue1))

	expectedValue2 := &CacheValue{Value: []byte("value2")}
	require.NoError(t, cache.PutState("ch1", "ns2", "k1", expectedValue2))

	expectedValue3 := &CacheValue{Value: []byte("value3")}
	require.NoError(t, cache.PutState("ch1", "lscc", "k1", expectedValue3))

	v1, err := cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue1, v1))

	v2, err := cache.GetState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue2, v2))

	v3, err := cache.GetState("ch1", "lscc", "k1")
	require.NoError(t, err)
	require.True(t, proto.Equal(expectedValue3, v3))

	cache.Reset()

	v, err := cache.GetState("ch1", "ns1", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = cache.GetState("ch1", "ns2", "k1")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = cache.GetState("ch1", "lscc", "k1")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestCacheUpdates(t *testing.T) {
	u := make(CacheUpdates)
	u.Add("ns1", CacheKVs{
		"k1": &CacheValue{Value: []byte("v1")},
		"k2": &CacheValue{Value: []byte("v2")},
	})

	u.Add("ns1", CacheKVs{
		"k3": &CacheValue{Value: []byte("v1")},
		"k4": &CacheValue{Value: []byte("v2")},
	})

	u.Add("ns2", CacheKVs{
		"k1": &CacheValue{Value: []byte("v1")},
		"k2": &CacheValue{Value: []byte("v2")},
	})

	expectedCacheUpdates := CacheUpdates{
		"ns1": CacheKVs{
			"k1": &CacheValue{Value: []byte("v1")},
			"k2": &CacheValue{Value: []byte("v2")},
			"k3": &CacheValue{Value: []byte("v1")},
			"k4": &CacheValue{Value: []byte("v2")},
		},

		"ns2": CacheKVs{
			"k1": &CacheValue{Value: []byte("v1")},
			"k2": &CacheValue{Value: []byte("v2")},
		},
	}

	require.Equal(t, expectedCacheUpdates, u)
}
