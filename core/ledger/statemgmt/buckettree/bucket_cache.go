/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package buckettree

import (
	"sync"
	"time"
	"unsafe"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/perfstat"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
)

var defaultBucketCacheMaxSize = 100 // MBs

// We can create a cache and keep all the bucket nodes pre-loaded.
// Since, the bucket nodes do not contain actual data and max possible
// buckets are pre-determined, the memory demand may not be very high or can easily
// be controlled - by keeping seletive buckets in the cache (most likely first few levels of the bucket tree - because,
// higher the level of the bucket, more are the chances that the bucket would be required for recomputation of hash)
type bucketCache struct {
	isEnabled bool
	c         map[bucketKey]*bucketNode
	lock      sync.RWMutex
	size      uint64
	maxSize   uint64
}

func newBucketCache(maxSizeMBs int) *bucketCache {
	isEnabled := true
	if maxSizeMBs <= 0 {
		isEnabled = false
	} else {
		logger.Infof("Constructing bucket-cache with max bucket cache size = [%d] MBs", maxSizeMBs)
	}
	return &bucketCache{c: make(map[bucketKey]*bucketNode), maxSize: uint64(maxSizeMBs * 1024 * 1024), isEnabled: isEnabled}
}

func (cache *bucketCache) loadAllBucketNodesFromDB() {
	if !cache.isEnabled {
		return
	}
	openchainDB := db.GetDBHandle()
	itr := openchainDB.GetStateCFIterator()
	defer itr.Close()
	itr.Seek([]byte{byte(0)})
	count := 0
	cache.lock.Lock()
	defer cache.lock.Unlock()
	for ; itr.Valid(); itr.Next() {
		key := itr.Key().Data()
		if key[0] != byte(0) {
			itr.Key().Free()
			itr.Value().Free()
			break
		}
		bKey := decodeBucketKey(statemgmt.Copy(itr.Key().Data()))
		nodeBytes := statemgmt.Copy(itr.Value().Data())
		bucketNode := unmarshalBucketNode(&bKey, nodeBytes)
		size := bKey.size() + bucketNode.size()
		cache.size += size
		if cache.size >= cache.maxSize {
			cache.size -= size
			break
		}
		cache.c[bKey] = bucketNode
		itr.Key().Free()
		itr.Value().Free()
		count++
	}
	logger.Infof("Loaded buckets data in cache. Total buckets in DB = [%d]. Total cache size:=%d", count, cache.size)
}

func (cache *bucketCache) putWithoutLock(key bucketKey, node *bucketNode) {
	if !cache.isEnabled {
		return
	}
	node.markedForDeletion = false
	node.childrenUpdated = nil
	existingNode, ok := cache.c[key]
	size := uint64(0)
	if ok {
		size = node.size() - existingNode.size()
		cache.size += size
		if cache.size > cache.maxSize {
			delete(cache.c, key)
			cache.size -= (key.size() + existingNode.size())
		} else {
			cache.c[key] = node
		}
	} else {
		size = node.size()
		cache.size += size
		if cache.size > cache.maxSize {
			return
		}
		cache.c[key] = node
	}
}

func (cache *bucketCache) get(key bucketKey) (*bucketNode, error) {
	defer perfstat.UpdateTimeStat("timeSpent", time.Now())
	if !cache.isEnabled {
		return fetchBucketNodeFromDB(&key)
	}
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	bucketNode := cache.c[key]
	if bucketNode == nil {
		return fetchBucketNodeFromDB(&key)
	}
	return bucketNode, nil
}

func (cache *bucketCache) removeWithoutLock(key bucketKey) {
	if !cache.isEnabled {
		return
	}
	node, ok := cache.c[key]
	if ok {
		cache.size -= (key.size() + node.size())
		delete(cache.c, key)
	}
}

func (bk bucketKey) size() uint64 {
	return uint64(unsafe.Sizeof(bk))
}

func (bNode *bucketNode) size() uint64 {
	size := uint64(unsafe.Sizeof(*bNode))
	numChildHashes := len(bNode.childrenCryptoHash)
	if numChildHashes > 0 {
		size += uint64(numChildHashes * len(bNode.childrenCryptoHash[0]))
	}
	return size
}
