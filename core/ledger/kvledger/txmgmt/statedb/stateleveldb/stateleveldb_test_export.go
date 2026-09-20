/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVDBEnv provides a level db backed versioned db for testing
type TestVDBEnv struct {
	t          testing.TB
	DBProvider *VersionedDBProvider
	dbPath     string
}

// NewTestVDBEnv instantiates and new level db backed TestVDB
func NewTestVDBEnv(t testing.TB) *TestVDBEnv {
	t.Logf("Creating new TestVDBEnv")
	dbPath := t.TempDir()
	dbProvider, err := NewVersionedDBProvider(dbPath)
	require.NoError(t, err)
	return &TestVDBEnv{t, dbProvider, dbPath}
}

// Cleanup closes the db and removes the db folder
func (env *TestVDBEnv) Cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	env.DBProvider.Close()
}
