/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("lockbasedtxmgr,statevalidator,statebasedval,statecouchdb,valimpl,pvtstatepurgemgmt,confighistory,kvledger,leveldbhelper=debug")
	if err := msptesttools.LoadMSPSetupForTesting(); err != nil {
		panic(fmt.Errorf("Could not load msp config, err %s", err))
	}
	os.Exit(m.Run())
}

func TestLedgerAPIs(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	// create two ledgers
	testLedger1 := env.createTestLedger("ledger1", t)
	testLedger2 := env.createTestLedger("ledger2", t)

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	dataHelper.populateLedger(testLedger1)
	dataHelper.populateLedger(testLedger2)

	// verify contents in both the ledgers
	dataHelper.verifyLedgerContent(testLedger1)
	dataHelper.verifyLedgerContent(testLedger2)
}
