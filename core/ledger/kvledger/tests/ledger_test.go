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
	flogging.ActivateSpec("lockbasedtxmgr,statevalidator,statebasedval,statecouchdb,valimpl,pvtstatepurgemgmt,confighistory,kvledger,leveldbhelper=debug")
	os.Exit(m.Run())
}

func TestLedgerAPIs(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	// create two ledgers
	testLedger1 := env.createTestLedgerFromGenesisBlk("ledger1")
	testLedger2 := env.createTestLedgerFromGenesisBlk("ledger2")

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	dataHelper.populateLedger(testLedger1)
	dataHelper.populateLedger(testLedger2)

	// verify contents in both the ledgers
	dataHelper.verifyLedgerContent(testLedger1)
	dataHelper.verifyLedgerContent(testLedger2)
}
