/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapolicy

import (
	"math"
	"sync"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
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
	collectionStore privdata.CollectionStore
	cache           map[btlkey]uint64
	lock            sync.Mutex
}

type btlkey struct {
	ns   string
	coll string
}

// NewBTLPolicy constructs an instance of LSCCBasedBTLPolicy
func NewBTLPolicy(ledger ledger.PeerLedger) BTLPolicy {
	return ConstructBTLPolicy(privdata.NewSimpleCollectionStore(&collectionSupport{lgr: ledger}))
}

// ConstructBTLPolicy constructs an instance of LSCCBasedBTLPolicy
func ConstructBTLPolicy(collectionStore privdata.CollectionStore) BTLPolicy {
	return &LSCCBasedBTLPolicy{
		collectionStore: collectionStore,
		cache:           make(map[btlkey]uint64)}
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
		persistenceConf, err := p.collectionStore.RetrieveCollectionPersistenceConfigs(
			common.CollectionCriteria{Namespace: namesapce, Collection: collection})
		if err != nil {
			return 0, err
		}
		btlConfigured := persistenceConf.BlockToLive()
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

type collectionSupport struct {
	lgr ledger.PeerLedger
}

func (cs *collectionSupport) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	return cs.lgr.NewQueryExecutor()
}

func (*collectionSupport) GetIdentityDeserializer(chainID string) msp.IdentityDeserializer {
	return nil
}
