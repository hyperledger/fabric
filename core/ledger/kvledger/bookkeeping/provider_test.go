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
	db := p.GetDBHandle("TestLedger", PvtdataExpiry)
	require.NoError(t, db.Put([]byte("key"), []byte("value"), true))
	val, err := db.Get([]byte("key"))
	require.NoError(t, err)
	require.Equal(t, []byte("value"), val)
}
