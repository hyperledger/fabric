/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import "testing"

// TestGenerate13SampleDataForBackwardCompatibility is very similar to TestLedgerAPIs
// however, this doesnot remove the ledgers created by the test. This is because, the sole
// purpose of this test is to generate the ledgers from v1.3 code so that they can manually be
// copied to future releases to make sure that the future releases can work with and can rebuild different
// ledger components (state/indexes etc.) for the ledger that is built using v1.3 codebase
// So, this test is marked as skipped in general.
// For producing the v1.3 ledger,
// 1) Make sure that the path '<peer.fileSystemPath>/ledgersData' ('/tmp/fabric/ledgertests/ledgersData') does not exist
// 2) commentout the line 't.Skip()' and run this test in isolation
// 3) move the folder '<peer.fileSystemPath>/ledgersData' to a separate place for copying over to a future release codebase
func TestGenerate13SampleDataForBackwardCompatibility(t *testing.T) {
	t.Skip()
	newEnv(defaultConfig, t)

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
