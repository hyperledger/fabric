/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("lockbasedtxmgr", "debug")
	flogging.SetModuleLevel("statevalidator", "debug")
	flogging.SetModuleLevel("statebasedval", "debug")
	flogging.SetModuleLevel("statecouchdb", "debug")
	flogging.SetModuleLevel("valimpl", "debug")
	flogging.SetModuleLevel("pvtstatepurgemgmt", "debug")
	flogging.SetModuleLevel("confighistory", "debug")
	flogging.SetModuleLevel("kvledger", "debug")

	os.Exit(m.Run())
}

func TestLedgerAPIs(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()

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
