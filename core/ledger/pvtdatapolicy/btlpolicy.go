/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapolicy

import (
	"math"
	"sync"

	"github.com/spf13/viper"
)

var defaultBLT uint64 = math.MaxUint64
var btlPolicyMap map[string]BTLPolicy
var lock = &sync.Mutex{}

func init() {
	btlPolicyMap = make(map[string]BTLPolicy)
}

// BTLPolicy BlockToLive policy for the pvt data
type BTLPolicy interface {
	// init initializes BTLPolicy for the given channelName
	init(channelName string) error
	// GetBTL returns BlockToLive for a given namespace and collection
	GetBTL(ns string, coll string) uint64
	// GetExpiringBlock returns the block number by which the pvtdata for given namespace,collection, and committingBlock should expire
	GetExpiringBlock(namesapce string, collection string, committingBlock uint64) uint64
}

// GetBTLPolicy constructs (if not already done) and returns the BTLPolicy for the given channel
func GetBTLPolicy(channelName string) (BTLPolicy, error) {
	lock.Lock()
	defer lock.Unlock()
	m := btlPolicyMap[channelName]
	if m == nil {
		m = &configBasedBTLPolicy{}
		m.init(channelName)
		btlPolicyMap[channelName] = m
	}
	return m, nil
}

// configBasedBTLPolicy implements interface BTLPolicy.
// This implementation loads the BTL policy from configuration. This implementation is meant as a stop gap arrangement
// until the config transaction framework is in place. This is becasue, the BTL policy should be consistent across peer
// and should not change across different replays of transactions in the chain and hence it should be set via a config
// transaction only. Later, another implementation should be provided that loads the BTL policy from config transactions
type configBasedBTLPolicy struct {
	channelName string
	cache       map[btlkey]uint64
}

type btlkey struct {
	ns   string
	coll string
}

// Init implements corresponding function in interface `BTLPolicyMgr`
func (m *configBasedBTLPolicy) init(channelName string) error {
	m.channelName = channelName
	m.cache = make(map[btlkey]uint64)
	return nil
}

// GetBTL implements corresponding function in interface `BTLPolicyMgr`
func (m *configBasedBTLPolicy) GetBTL(namesapce string, collection string) uint64 {
	var btl uint64
	var ok bool
	key := btlkey{namesapce, collection}
	btl, ok = m.cache[key]
	if !ok {
		btlConfigured := viper.GetInt("ledger.pvtdata.btlpolicy." + m.channelName + "." + namesapce + "." + collection)
		if btlConfigured > 0 {
			btl = uint64(btlConfigured)
		} else {
			btl = defaultBLT
		}
		m.cache[key] = btl
	}
	return btl
}

func (m *configBasedBTLPolicy) GetExpiringBlock(namesapce string, collection string, committingBlock uint64) uint64 {
	btl := m.GetBTL(namesapce, collection)
	expiryBlk := committingBlock + btl + uint64(1)
	if expiryBlk <= committingBlock { // committingBlk + btl overflows uint64-max
		expiryBlk = math.MaxUint64
	}
	return expiryBlk
}
