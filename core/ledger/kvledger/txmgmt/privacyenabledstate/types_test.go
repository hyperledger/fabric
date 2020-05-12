/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()
	for _, testEnv := range testEnvs {
		testEnv.StopExternalResource()
	}
	os.Exit(exitCode)
}

func TestBatch(t *testing.T) {
	batch := UpdateMap(make(map[string]NsBatch))
	v := version.NewHeight(1, 1)
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			for k := 0; k < 5; k++ {
				batch.Put(fmt.Sprintf("ns-%d", i), fmt.Sprintf("collection-%d", j), fmt.Sprintf("key-%d", k),
					[]byte(fmt.Sprintf("value-%d-%d-%d", i, j, k)), v)
			}
		}
	}
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			for k := 0; k < 5; k++ {
				vv := batch.Get(fmt.Sprintf("ns-%d", i), fmt.Sprintf("collection-%d", j), fmt.Sprintf("key-%d", k))
				assert.NotNil(t, vv)
				assert.Equal(t,
					&statedb.VersionedValue{Value: []byte(fmt.Sprintf("value-%d-%d-%d", i, j, k)), Version: v},
					vv)
			}
		}
	}
	assert.Nil(t, batch.Get("ns-1", "collection-1", "key-5"))
	assert.Nil(t, batch.Get("ns-1", "collection-5", "key-1"))
	assert.Nil(t, batch.Get("ns-5", "collection-1", "key-1"))
}

func TestHashBatchContains(t *testing.T) {
	batch := NewHashedUpdateBatch()
	batch.Put("ns1", "coll1", []byte("key1"), []byte("val1"), version.NewHeight(1, 1))
	assert.True(t, batch.Contains("ns1", "coll1", []byte("key1")))
	assert.False(t, batch.Contains("ns1", "coll1", []byte("key2")))
	assert.False(t, batch.Contains("ns1", "coll2", []byte("key1")))
	assert.False(t, batch.Contains("ns2", "coll1", []byte("key1")))

	batch.Delete("ns1", "coll1", []byte("deleteKey"), version.NewHeight(1, 1))
	assert.True(t, batch.Contains("ns1", "coll1", []byte("deleteKey")))
	assert.False(t, batch.Contains("ns1", "coll1", []byte("deleteKey1")))
	assert.False(t, batch.Contains("ns1", "coll2", []byte("deleteKey")))
	assert.False(t, batch.Contains("ns2", "coll1", []byte("deleteKey")))
}

func TestCompositeKeyMap(t *testing.T) {
	b := NewPvtUpdateBatch()
	b.Put("ns1", "coll1", "key1", []byte("testVal1"), nil)
	b.Delete("ns1", "coll2", "key2", nil)
	b.Put("ns2", "coll1", "key1", []byte("testVal3"), nil)
	b.Put("ns2", "coll2", "key2", []byte("testVal4"), nil)
	m := b.ToCompositeKeyMap()
	assert.Len(t, m, 4)
	vv, ok := m[PvtdataCompositeKey{"ns1", "coll1", "key1"}]
	assert.True(t, ok)
	assert.Equal(t, []byte("testVal1"), vv.Value)
	vv, ok = m[PvtdataCompositeKey{"ns1", "coll2", "key2"}]
	assert.Nil(t, vv.Value)
	assert.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll1", "key1"}]
	assert.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll2", "key2"}]
	assert.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll1", "key8888"}]
	assert.False(t, ok)
}
