/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bookkeeping

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/bookkeeping")
	os.Exit(m.Run())
}

func TestProvider(t *testing.T) {
	testEnv := NewTestEnv(t)
	defer testEnv.Cleanup()
	p := testEnv.TestProvider
	db := p.GetDBHandle("TestLedger", PvtdataExpiry)
	assert.NoError(t, db.Put([]byte("key"), []byte("value"), true))
	val, err := db.Get([]byte("key"))
	assert.NoError(t, err)
	assert.Equal(t, []byte("value"), val)
}
