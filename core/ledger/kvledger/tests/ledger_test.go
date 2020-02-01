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
	h1 := env.newTestHelperCreateLgr("ledger1", t)
	h2 := env.newTestHelperCreateLgr("ledger2", t)

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	dataHelper.populateLedger(h1)
	dataHelper.populateLedger(h2)

	// verify contents in both the ledgers
	dataHelper.verifyLedgerContent(h1)
	dataHelper.verifyLedgerContent(h2)
}
