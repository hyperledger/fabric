/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/stretchr/testify/require"
)

// TestGenerate13SampleDataForBackwardCompatibility is very similar to TestLedgerAPIs
// however, this doesnot remove the ledgers created by the test. This is because, the sole
// purpose of this test is to generate the ledgers from v1.3 code so that they can manually be
// copied to future releases to make sure that the future releases can work with and can rebuild different
// ledger components (state/indexes etc.) for the ledger that is built using v1.3 codebase.
// This test is marked as skipped in general. To produce the v1.3 ledger,
// 1) Make sure that the path '<leveldbTestDir>' does not exist
// 2) commentout the line 't.Skip()' and run this test in isolation
// 3) move <leveldbTestDir>/ledgersData.zip to a future release codebase
func TestGenerate13SampleDataForBackwardCompatibility(t *testing.T) {
	t.Skip()

	// verify the test has a clean env - i.e. ledger root dir does not exist
	_, err := os.Stat(leveldbTestDir)
	require.Equalf(t, true, os.IsNotExist(err), "ledger test dir %s should not exist", leveldbTestDir)

	newEnv(defaultConfig, t)

	generateSampleData(t)

	require.NoError(t, createZipFile(filepath.Join(leveldbTestDir, "ledgersData"), filepath.Join(leveldbTestDir, "ledgersData.zip")))
}

// TestGenerate13SampleDataCouchdbForBackwardCompatibility is similar to TestGenerate13SampleDataForBackwardCompatibility
// except that it uses CouchDB as the state DB. It does not delete ledger data or couchDB data at the end of the test so that
// the generated ledger data and couchdb data under '<couchdbHostDir>' can be tested in a newer release for backward compatibility.
// This test is marked as skipped in general. To produce the v1.3 ledger,
// 1) Make sure that the path '<couchdbTestDir>' does not exist
// 2) comment out the line 't.Skip()' and run this test in isolation
// 3) move <couchdbTestDir>/ledgersData.zip and <couchdbTestDir>/couchdbData.zip directory to a future release codebase
func TestGenerate13SampleDataCouchdbForBackwardCompatibility(t *testing.T) {
	t.Skip()

	// verify that the test dir does not exist - i.e., ledger data, couchdb host dir, and local.d dir do not exist
	_, err := os.Stat(couchdbTestDir)
	require.Equalf(t, true, os.IsNotExist(err), "ledger test dir %s should not exist", couchdbTestDir)

	couchdbHostDir := filepath.Join(couchdbTestDir, "couchdbData")
	require.NoError(t, os.MkdirAll(couchdbHostDir, os.ModePerm), "create host directory for couchdb mount point")

	// localdHostDir will be mounted to couchdb so that it can overwrite the number of shards (q=1) and nodes (n=1).
	// The purpose is to minimize the size of the generated couchdb data that will be stored in github as testdata.
	testutil.CopyDir("testdata/v13/couchdb_etc/local.d", couchdbTestDir)
	localdHostDir := filepath.Join(couchdbTestDir, "local.d")

	// start couchdb with a host directory mounted to the couchdb container
	address, stopDB := couchDBSetup(t, couchdbHostDir, localdHostDir)
	defer stopDB()
	defaultConfigWithStateCouchdb["ledger.state.couchDBConfig.couchDBAddress"] = address

	newEnv(defaultConfigWithStateCouchdb, t)

	generateSampleData(t)

	require.NoError(t, createZipFile(filepath.Join(couchdbTestDir, "ledgersData"), filepath.Join(couchdbTestDir, "ledgersData.zip")))
	require.NoError(t, createZipFile(filepath.Join(couchdbTestDir, "couchdbData"), filepath.Join(couchdbTestDir, "couchdbData.zip")))
}

func generateSampleData(t *testing.T) {
	// create two ledgers
	h1 := newTestHelperCreateLgr("ledger1", t)
	h2 := newTestHelperCreateLgr("ledger2", t)

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	dataHelper.populateLedger(h1)
	dataHelper.populateLedger(h2)

	// verify contents in both the ledgers
	dataHelper.verifyLedgerContent(h1)
	dataHelper.verifyLedgerContent(h2)
}

func couchDBSetup(t *testing.T, couchdbHostDir, localdHostDir string) (addr string, cleanup func()) {
	couchDB := &runner.CouchDB{
		Binds: []string{
			fmt.Sprintf("%s:%s", couchdbHostDir, "/opt/couchdb/data"),
			fmt.Sprintf("%s:%s", localdHostDir, "/opt/couchdb/etc/local.d"),
		},
	}

	err := couchDB.Start()
	require.NoError(t, err)

	return couchDB.Address(), func() { couchDB.Stop() }
}
