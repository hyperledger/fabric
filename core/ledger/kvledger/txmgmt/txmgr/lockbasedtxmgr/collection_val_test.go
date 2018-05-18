/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/stretchr/testify/assert"
)

func TestCollectionValidation(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr.(*LockBasedTxMgr),
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll2"},
			{"ns2", "coll1"},
			{"ns2", "coll2"},
		},
		version.NewHeight(1, 1),
	)

	sim, err := txMgr.NewTxSimulator("tx-id1")
	assert.NoError(t, err)

	_, err = sim.GetPrivateData("ns3", "coll1", "key1")
	_, ok := err.(*errCollConfigNotDefined)
	assert.True(t, ok)

	err = sim.SetPrivateData("ns3", "coll1", "key1", []byte("val1"))
	_, ok = err.(*errCollConfigNotDefined)
	assert.True(t, ok)

	_, err = sim.GetPrivateData("ns1", "coll3", "key1")
	_, ok = err.(*errInvalidCollName)
	assert.True(t, ok)

	err = sim.SetPrivateData("ns1", "coll3", "key1", []byte("val1"))
	_, ok = err.(*errInvalidCollName)
	assert.True(t, ok)

	err = sim.SetPrivateData("ns1", "coll1", "key1", []byte("val1"))
	assert.NoError(t, err)
}
