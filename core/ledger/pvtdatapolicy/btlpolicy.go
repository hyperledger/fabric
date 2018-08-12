/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapolicy

import (
	"math"
	"sync"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/protos/common"
)

var defaultBTL uint64 = math.MaxUint64

// BTLPolicy BlockToLive policy for the pvt data
type BTLPolicy interface {
	// GetBTL returns BlockToLive for a given namespace and collection
	GetBTL(ns string, coll string) (uint64, error)
	// GetExpiringBlock returns the block number by which the pvtdata for given namespace,collection, and committingBlock should expire
	GetExpiringBlock(namesapce string, collection string, committingBlock uint64) (uint64, error)
}

// LSCCBasedBTLPolicy implements interface BTLPolicy.
// This implementation loads the BTL policy from lscc namespace which is populated
// with the collection configuration during chaincode initialization
type LSCCBasedBTLPolicy struct {
	collInfoProvider collectionInfoProvider
	cache            map[btlkey]uint64
	lock             sync.Mutex
}

type btlkey struct {
	ns   string
	coll string
}

// ConstructBTLPolicy constructs an instance of LSCCBasedBTLPolicy
func ConstructBTLPolicy(collInfoProvider collectionInfoProvider) BTLPolicy {
	return &LSCCBasedBTLPolicy{
		collInfoProvider: collInfoProvider,
		cache:            make(map[btlkey]uint64),
	}
}

// GetBTL implements corresponding function in interface `BTLPolicyMgr`
func (p *LSCCBasedBTLPolicy) GetBTL(namesapce string, collection string) (uint64, error) {
	var btl uint64
	var ok bool
	key := btlkey{namesapce, collection}
	p.lock.Lock()
	defer p.lock.Unlock()
	btl, ok = p.cache[key]
	if !ok {
		collConfig, err := p.collInfoProvider.CollectionInfo(namesapce, collection)
		if err != nil {
			return 0, err
		}
		if collConfig == nil {
			return 0, privdata.NoSuchCollectionError{Namespace: namesapce, Collection: collection}
		}
		btlConfigured := collConfig.BlockToLive
		if btlConfigured > 0 {
			btl = uint64(btlConfigured)
		} else {
			btl = defaultBTL
		}
		p.cache[key] = btl
	}
	return btl, nil
}

// GetExpiringBlock implements function from the interface `BTLPolicy`
func (p *LSCCBasedBTLPolicy) GetExpiringBlock(namesapce string, collection string, committingBlock uint64) (uint64, error) {
	btl, err := p.GetBTL(namesapce, collection)
	if err != nil {
		return 0, err
	}
	expiryBlk := committingBlock + btl + uint64(1)
	if expiryBlk <= committingBlock { // committingBlk + btl overflows uint64-max
		expiryBlk = math.MaxUint64
	}
	return expiryBlk, nil
}

type collectionInfoProvider interface {
	CollectionInfo(chaincodeName, collectionName string) (*common.StaticCollectionConfig, error)
}

//go:generate counterfeiter -o mock/coll_info_provider.go -fake-name CollectionInfoProvider . collectionInfoProvider
