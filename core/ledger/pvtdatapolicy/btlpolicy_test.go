/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapolicy

import (
	"math"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestBTLPolicy(t *testing.T) {
	viper.Set("ledger.pvtdata.btlpolicy.ch1.ns1.coll1", 1)
	viper.Set("ledger.pvtdata.btlpolicy.ch1.ns2.coll2", 2)
	viper.Set("ledger.pvtdata.btlpolicy.ch2.ns2.coll2", 3)
	viper.Set("ledger.pvtdata.btlpolicy.ch2.ns3.coll4", 4)

	btlCh1, _ := GetBTLPolicy("ch1")
	btlCh2, _ := GetBTLPolicy("ch2")

	assert.Equal(t, uint64(1), btlCh1.GetBTL("ns1", "coll1"))
	assert.Equal(t, uint64(4), btlCh2.GetBTL("ns3", "coll4"))
	assert.Equal(t, defaultBLT, btlCh2.GetBTL("ns1", "coll5"))

	assert.Equal(t, uint64(5), btlCh1.GetExpiringBlock("ns1", "coll1", 3))
	assert.Equal(t, uint64(8), btlCh2.GetExpiringBlock("ns3", "coll4", 3))
	assert.Equal(t, uint64(math.MaxUint64), btlCh2.GetExpiringBlock("ns3", "coll4", math.MaxUint64-uint64(2)))
	assert.Equal(t, uint64(math.MaxUint64), btlCh2.GetExpiringBlock("ns1", "coll5", math.MaxUint64-uint64(2)))
}
