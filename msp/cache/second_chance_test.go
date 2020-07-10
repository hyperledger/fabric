/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecondChanceCache(t *testing.T) {
	cache := newSecondChanceCache(2)
	require.NotNil(t, cache)

	cache.add("a", "xyz")

	cache.add("b", "123")
	// get b, b referenced bit is set to true
	obj, ok := cache.get("b")
	require.True(t, ok)
	require.Equal(t, "123", obj.(string))

	// add c. victim scan: delete a and set b as the next candidate of a victim
	cache.add("c", "777")

	// check a is deleted
	_, ok = cache.get("a")
	require.False(t, ok)

	// add d. victim scan: b referenced bit is set to false and delete c
	cache.add("d", "555")

	// check c is deleted
	_, ok = cache.get("c")
	require.False(t, ok)

	// check b and d
	obj, ok = cache.get("b")
	require.True(t, ok)
	require.Equal(t, "123", obj.(string))
	obj, ok = cache.get("d")
	require.True(t, ok)
	require.Equal(t, "555", obj.(string))
}

func TestSecondChanceCacheConcurrent(t *testing.T) {
	cache := newSecondChanceCache(25)

	workers := 16
	wg := sync.WaitGroup{}
	wg.Add(workers)

	key1 := "key1"
	val1 := key1

	for i := 0; i < workers; i++ {
		id := i
		key2 := fmt.Sprintf("key2-%d", i)
		val2 := key2

		go func() {
			for j := 0; j < 10000; j++ {
				key3 := fmt.Sprintf("key3-%d-%d", id, j)
				val3 := key3
				cache.add(key3, val3)

				val, ok := cache.get(key1)
				if ok {
					require.Equal(t, val1, val.(string))
				}
				cache.add(key1, val1)

				val, ok = cache.get(key2)
				if ok {
					require.Equal(t, val2, val.(string))
				}
				cache.add(key2, val2)

				key4 := fmt.Sprintf("key4-%d", j)
				val4 := key4
				val, ok = cache.get(key4)
				if ok {
					require.Equal(t, val4, val.(string))
				}
				cache.add(key4, val4)

				val, ok = cache.get(key3)
				if ok {
					require.Equal(t, val3, val.(string))
				}
			}

			wg.Done()
		}()
	}
	wg.Wait()
}
