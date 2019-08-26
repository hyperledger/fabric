/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import "testing"

// TestGenerateSampleDataForV12BackwardCompatibility is very similar to TestLedgerAPIs
// however, this doesnot remove the ledgers created by the test. This is because, the sole
// purpose of this test is to generate the ledgers from v1.1 code so that they can manually be
// copied to v1.2 codebase to make sure that v1.2 code can work with and can rebuild different
// ledger components (state/indexes etc.) for the ledger that is built using v1.1 codebase
// So, this test is marked as skipped in general.
// For producing the v1.1 ledger,
// 1) Make sure that the path '<peer.fileSystemPath>/ledgersData' ('/tmp/fabric/ledgertests/ledgersData') does not exist
// 2) commentout the line 't.Skip()' and run this test in isolation
// 3) move the folder '<peer.fileSystemPath>/ledgersData' to a separate place for copying over to v1.2 codebase
// See a test in the same folder in v1.2 codebase for how this 'ledgersData' is used for testing backward compatibility
func TestGenerateSampleDataForV12BackwardCompatibility(t *testing.T) {
	t.Skip()
	c := config{}
	for k, v := range defaultConfig {
		c[k] = v
	}
	c["ledger.history.enableHistoryDatabase"] = true
	newEnv(c, t)

	// create two ledgers
	h1 := newTestHelperCreateLgr("ledger1", t)
	h2 := newTestHelperCreateLgr("ledger2", t)

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	dataHelper.populateLedger(h1)
	dataHelper.populateLedger(h2)

	// verify contents in both the ledgers
	dataHelper.verifyLedger(h1)
	dataHelper.verifyLedger(h2)
}
