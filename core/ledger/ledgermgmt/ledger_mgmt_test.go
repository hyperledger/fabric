/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ledgermgmt

import (
	"fmt"
	"testing"

	"os"

	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/ledgermgmt")
	os.Exit(m.Run())
}

func TestLedgerMgmt(t *testing.T) {
	// Check for error when creating/opening ledger without initialization.
	gb, _ := test.MakeGenesisBlock(constructTestLedgerID(0))
	l, err := CreateLedger(gb)
	testutil.AssertNil(t, l)
	testutil.AssertEquals(t, err, ErrLedgerMgmtNotInitialized)

	ledgerID := constructTestLedgerID(2)
	l, err = OpenLedger(ledgerID)
	testutil.AssertNil(t, l)
	testutil.AssertEquals(t, err, ErrLedgerMgmtNotInitialized)

	ids, err := GetLedgerIDs()
	testutil.AssertNil(t, ids)
	testutil.AssertEquals(t, err, ErrLedgerMgmtNotInitialized)

	Close()

	InitializeTestEnv()
	defer CleanupTestEnv()

	numLedgers := 10
	ledgers := make([]ledger.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		gb, _ := test.MakeGenesisBlock(constructTestLedgerID(i))
		l, _ := CreateLedger(gb)
		ledgers[i] = l
	}

	ids, _ = GetLedgerIDs()
	testutil.AssertEquals(t, len(ids), numLedgers)
	for i := 0; i < numLedgers; i++ {
		testutil.AssertEquals(t, ids[i], constructTestLedgerID(i))
	}

	ledgerID = constructTestLedgerID(2)
	t.Logf("Ledger selected for test = %s", ledgerID)
	_, err = OpenLedger(ledgerID)
	testutil.AssertEquals(t, err, ErrLedgerAlreadyOpened)

	l = ledgers[2]
	l.Close()
	l, err = OpenLedger(ledgerID)
	testutil.AssertNoError(t, err, "")

	l, err = OpenLedger(ledgerID)
	testutil.AssertEquals(t, err, ErrLedgerAlreadyOpened)

	// close all opened ledgers and ledger mgmt
	Close()

	// Restart ledger mgmt with existing ledgers
	Initialize()
	l, err = OpenLedger(ledgerID)
	testutil.AssertNoError(t, err, "")
	Close()
}

func constructTestLedgerID(i int) string {
	return fmt.Sprintf("ledger_%06d", i)
}
