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

package kvledger

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
)

func TestLedgerProvider(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	numLedgers := 10
	provider, _ := NewProvider()
	existingLedgerIDs, err := provider.List()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, len(existingLedgerIDs), 0)
	for i := 0; i < numLedgers; i++ {
		provider.Create(constructTestLedgerID(i))
	}
	existingLedgerIDs, err = provider.List()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, len(existingLedgerIDs), numLedgers)

	provider.Close()

	provider, _ = NewProvider()
	defer provider.Close()
	ledgerIds, _ := provider.List()
	testutil.AssertEquals(t, len(ledgerIds), numLedgers)
	t.Logf("ledgerIDs=%#v", ledgerIds)
	for i := 0; i < numLedgers; i++ {
		testutil.AssertEquals(t, ledgerIds[i], constructTestLedgerID(i))
	}
	_, err = provider.Create(constructTestLedgerID(2))
	testutil.AssertEquals(t, err, ErrLedgerIDExists)

	_, err = provider.Open(constructTestLedgerID(numLedgers))
	testutil.AssertEquals(t, err, ErrNonExistingLedgerID)
}

func TestMultipleLedgerBasicRW(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	numLedgers := 10
	provider, _ := NewProvider()
	ledgers := make([]ledger.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		l, err := provider.Create(constructTestLedgerID(i))
		testutil.AssertNoError(t, err, "")
		ledgers[i] = l
	}

	for i, l := range ledgers {
		s, _ := l.NewTxSimulator()
		err := s.SetState("ns", "testKey", []byte(fmt.Sprintf("testValue_%d", i)))
		s.Done()
		testutil.AssertNoError(t, err, "")
		res, err := s.GetTxSimulationResults()
		testutil.AssertNoError(t, err, "")
		b := testutil.ConstructBlock(t, [][]byte{res}, false)
		err = l.Commit(b)
		l.Close()
		testutil.AssertNoError(t, err, "")
	}

	provider.Close()

	provider, _ = NewProvider()
	defer provider.Close()
	ledgers = make([]ledger.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		l, err := provider.Open(constructTestLedgerID(i))
		testutil.AssertNoError(t, err, "")
		ledgers[i] = l
	}

	for i, l := range ledgers {
		q, _ := l.NewQueryExecutor()
		val, err := q.GetState("ns", "testKey")
		q.Done()
		testutil.AssertNoError(t, err, "")
		testutil.AssertEquals(t, val, []byte(fmt.Sprintf("testValue_%d", i)))
		l.Close()
	}
}

func constructTestLedgerID(i int) string {
	return fmt.Sprintf("ledger_%06d", i)
}
