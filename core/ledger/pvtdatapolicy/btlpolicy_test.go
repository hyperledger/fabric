/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapolicy

import (
	"testing"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBTLPolicy(t *testing.T) {
	mockCollectionStore := testutil.NewMockCollectionStore()
	mockCollectionStore.SetBTL("ns1", "coll1", 100)
	mockCollectionStore.SetBTL("ns1", "coll2", 200)
	mockCollectionStore.SetBTL("ns1", "coll3", 0)

	btlPolicy := ConstructBTLPolicy(mockCollectionStore)
	btl1, err := btlPolicy.GetBTL("ns1", "coll1")
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), btl1)

	btl2, err := btlPolicy.GetBTL("ns1", "coll2")
	assert.NoError(t, err)
	assert.Equal(t, uint64(200), btl2)

	btl3, err := btlPolicy.GetBTL("ns1", "coll3")
	assert.NoError(t, err)
	assert.Equal(t, defaultBTL, btl3)

	_, err = btlPolicy.GetBTL("ns1", "coll4")
	_, ok := err.(privdata.NoSuchCollectionError)
	assert.True(t, ok)
}
