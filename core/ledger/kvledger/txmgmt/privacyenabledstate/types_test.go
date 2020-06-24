/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("privacyenabledstate=debug")
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
				require.NotNil(t, vv)
				require.Equal(t,
					&statedb.VersionedValue{Value: []byte(fmt.Sprintf("value-%d-%d-%d", i, j, k)), Version: v},
					vv)
			}
		}
	}
	require.Nil(t, batch.Get("ns-1", "collection-1", "key-5"))
	require.Nil(t, batch.Get("ns-1", "collection-5", "key-1"))
	require.Nil(t, batch.Get("ns-5", "collection-1", "key-1"))
}

func TestHashBatchContains(t *testing.T) {
	batch := NewHashedUpdateBatch()
	batch.Put("ns1", "coll1", []byte("key1"), []byte("val1"), version.NewHeight(1, 1))
	require.True(t, batch.Contains("ns1", "coll1", []byte("key1")))
	require.False(t, batch.Contains("ns1", "coll1", []byte("key2")))
	require.False(t, batch.Contains("ns1", "coll2", []byte("key1")))
	require.False(t, batch.Contains("ns2", "coll1", []byte("key1")))

	batch.Delete("ns1", "coll1", []byte("deleteKey"), version.NewHeight(1, 1))
	require.True(t, batch.Contains("ns1", "coll1", []byte("deleteKey")))
	require.False(t, batch.Contains("ns1", "coll1", []byte("deleteKey1")))
	require.False(t, batch.Contains("ns1", "coll2", []byte("deleteKey")))
	require.False(t, batch.Contains("ns2", "coll1", []byte("deleteKey")))
}

func TestCompositeKeyMap(t *testing.T) {
	b := NewPvtUpdateBatch()
	b.Put("ns1", "coll1", "key1", []byte("testVal1"), nil)
	b.Delete("ns1", "coll2", "key2", nil)
	b.Put("ns2", "coll1", "key1", []byte("testVal3"), nil)
	b.Put("ns2", "coll2", "key2", []byte("testVal4"), nil)
	m := b.ToCompositeKeyMap()
	require.Len(t, m, 4)
	vv, ok := m[PvtdataCompositeKey{"ns1", "coll1", "key1"}]
	require.True(t, ok)
	require.Equal(t, []byte("testVal1"), vv.Value)
	vv, ok = m[PvtdataCompositeKey{"ns1", "coll2", "key2"}]
	require.Nil(t, vv.Value)
	require.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll1", "key1"}]
	require.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll2", "key2"}]
	require.True(t, ok)
	_, ok = m[PvtdataCompositeKey{"ns2", "coll1", "key8888"}]
	require.False(t, ok)
}
