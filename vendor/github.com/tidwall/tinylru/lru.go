// Copyright 2020 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tinylru

import "sync"

// DefaultSize is the default maximum size of an LRU cache before older items
// get automatically evicted.
const DefaultSize = 256

type lruItem struct {
	key   interface{} // user-defined key
	value interface{} // user-defined value
	prev  *lruItem    // prev item in list. More recently used
	next  *lruItem    // next item in list. Less recently used
}

// LRU implements an LRU cache
type LRU struct {
	mu    sync.Mutex               // protect all things
	size  int                      // max number of items.
	items map[interface{}]*lruItem // active items
	head  *lruItem                 // head of list
	tail  *lruItem                 // tail of list
}

//go:noinline
func (lru *LRU) init() {
	lru.items = make(map[interface{}]*lruItem)
	lru.head = new(lruItem)
	lru.tail = new(lruItem)
	lru.head.next = lru.tail
	lru.tail.prev = lru.head
	if lru.size == 0 {
		lru.size = DefaultSize
	}
}

func (lru *LRU) evict() *lruItem {
	item := lru.tail.prev
	lru.pop(item)
	delete(lru.items, item.key)
	return item
}

func (lru *LRU) pop(item *lruItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

func (lru *LRU) push(item *lruItem) {
	lru.head.next.prev = item
	item.next = lru.head.next
	item.prev = lru.head
	lru.head.next = item
}

// Resize sets the maximum size of an LRU cache. If this value is less than
// the number of items currently in the cache, then items will be evicted.
// Returns evicted items.
// This operation will panic if the size is less than one.
func (lru *LRU) Resize(size int) (evictedKeys []interface{},
	evictedValues []interface{}) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	if size <= 0 {
		panic("invalid size")
	}
	for size < len(lru.items) {
		item := lru.evict()
		evictedKeys = append(evictedKeys, item.key)
		evictedValues = append(evictedValues, item.value)
	}
	lru.size = size
	return evictedKeys, evictedValues
}

// Len returns the length of the lru cache
func (lru *LRU) Len() int {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	return len(lru.items)
}

// SetEvicted sets or replaces a value for a key. If this operation causes an
// eviction then the evicted item is returned.
func (lru *LRU) SetEvicted(key interface{}, value interface{}) (
	prev interface{}, replaced bool, evictedKey interface{},
	evictedValue interface{}, evicted bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	if lru.items == nil {
		lru.init()
	}
	item := lru.items[key]
	if item == nil {
		if len(lru.items) == lru.size {
			item = lru.evict()
			evictedKey, evictedValue, evicted = item.key, item.value, true
		} else {
			item = new(lruItem)
		}
		item.key = key
		item.value = value
		lru.push(item)
		lru.items[key] = item
	} else {
		prev, replaced = item.value, true
		item.value = value
		if lru.head.next != item {
			lru.pop(item)
			lru.push(item)
		}
	}
	return prev, replaced, evictedKey, evictedValue, evicted
}

// Set or replace a value for a key.
func (lru *LRU) Set(key interface{}, value interface{}) (prev interface{},
	replaced bool) {
	prev, replaced, _, _, _ = lru.SetEvicted(key, value)
	return prev, replaced
}

// Get a value for key
func (lru *LRU) Get(key interface{}) (value interface{}, ok bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	item := lru.items[key]
	if item == nil {
		return nil, false
	}
	if lru.head.next != item {
		lru.pop(item)
		lru.push(item)
	}
	return item.value, true
}

// Delete a value for a key
func (lru *LRU) Delete(key interface{}) (prev interface{}, deleted bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	item := lru.items[key]
	if item == nil {
		return nil, false
	}
	delete(lru.items, key)
	lru.pop(item)
	return item.value, true
}

// Range iterates over all key/values in the order of most recently to
// least recently used items.
// It's not safe to call other LRU operations while ranging.
func (lru *LRU) Range(iter func(key interface{}, value interface{}) bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	if head := lru.head; head != nil {
		item := head.next
		for item != lru.tail {
			if !iter(item.key, item.value) {
				return
			}
			item = item.next
		}
	}
}

// Reverse iterates over all key/values in the order of least recently to
// most recently used items.
// It's not safe to call other LRU operations while ranging.
func (lru *LRU) Reverse(iter func(key interface{}, value interface{}) bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	if tail := lru.tail; tail != nil {
		item := tail.prev
		for item != lru.head {
			if !iter(item.key, item.value) {
				return
			}
			item = item.prev
		}
	}
}
