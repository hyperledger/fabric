/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bookkeeping

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
