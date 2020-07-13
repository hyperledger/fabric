/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bookkeeping

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	testEnv := NewTestEnv(t)
	defer testEnv.Cleanup()
	p := testEnv.TestProvider

	pvtdataExpiryDB := p.GetDBHandle("TestLedger", PvtdataExpiry)
	require.NoError(t, pvtdataExpiryDB.Put([]byte("key1"), []byte("value1"), true))
	val, err := pvtdataExpiryDB.Get([]byte("key1"))
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), val)

	metadataIndicatorDB := p.GetDBHandle("TestLedger", MetadataPresenceIndicator)
	require.NoError(t, metadataIndicatorDB.Put([]byte("key2"), []byte("value2"), true))
	val, err = metadataIndicatorDB.Get([]byte("key2"))
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), val)

	snapshotRequestDB := p.GetDBHandle("TestLedger", SnapshotRequest)
	require.NoError(t, snapshotRequestDB.Put([]byte("key3"), []byte("value3"), true))
	val, err = snapshotRequestDB.Get([]byte("key3"))
	require.NoError(t, err)
	require.Equal(t, []byte("value3"), val)

	require.NoError(t, p.Drop("TestLedger"))

	val, err = pvtdataExpiryDB.Get([]byte("key1"))
	require.NoError(t, err)
	require.Nil(t, val)
	val, err = metadataIndicatorDB.Get([]byte("key2"))
	require.NoError(t, err)
	require.Nil(t, val)
	val, err = snapshotRequestDB.Get([]byte("key3"))
	require.NoError(t, err)
	require.Nil(t, val)

	// drop again is not an error
	require.NoError(t, p.Drop("TestLedger"))

	p.Close()
	require.EqualError(t, p.Drop("TestLedger"), "internal leveldb error while obtaining db iterator: leveldb: closed")
}
