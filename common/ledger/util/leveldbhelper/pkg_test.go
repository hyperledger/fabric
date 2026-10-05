/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package leveldbhelper

import (
	"os"
	"testing"

	db "github.com/hyperledger/fabric/common/ledger"
	"github.com/stretchr/testify/require"
)

const testDBPath = "/tmp/fabric/ledgertests/util/leveldbhelper"

type testDBEnv struct {
	t    *testing.T
	path string
	db   db.DB
}

type testDBProviderEnv struct {
	t        *testing.T
	path     string
	provider db.Provider
}

func newTestDBEnv(t *testing.T, path string) *testDBEnv {
	testDBEnv := &testDBEnv{t: t, path: path}
	testDBEnv.cleanup()
	testDBEnv.db = CreateDB(&Conf{DBPath: path})
	return testDBEnv
}

func newTestProviderEnv(t *testing.T, path string) *testDBProviderEnv {
	testProviderEnv := &testDBProviderEnv{t: t, path: path}
	testProviderEnv.cleanup()
	var err error
	testProviderEnv.provider, err = NewProvider(&Conf{DBPath: path})
	if err != nil {
		panic(err)
	}
	return testProviderEnv
}

func (dbEnv *testDBEnv) cleanup() {
	if dbEnv.db != nil {
		dbEnv.db.Close()
	}
	require.NoError(dbEnv.t, os.RemoveAll(dbEnv.path))
}

func (providerEnv *testDBProviderEnv) cleanup() {
	if providerEnv.provider != nil {
		providerEnv.provider.Close()
	}
	require.NoError(providerEnv.t, os.RemoveAll(providerEnv.path))
}
