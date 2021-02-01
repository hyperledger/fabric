/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/stretchr/testify/require"
)

func TestCollectionValidation(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll2"},
			{"ns2", "coll1"},
			{"ns2", "coll2"},
		},
		version.NewHeight(1, 1),
	)

	sim, err := txMgr.NewTxSimulator("tx-id1")
	require.NoError(t, err)

	_, err = sim.GetPrivateData("ns3", "coll1", "key1")
	_, ok := err.(*ledger.CollConfigNotDefinedError)
	require.True(t, ok)

	err = sim.SetPrivateData("ns3", "coll1", "key1", []byte("val1"))
	_, ok = err.(*ledger.CollConfigNotDefinedError)
	require.True(t, ok)

	_, err = sim.GetPrivateData("ns1", "coll3", "key1")
	_, ok = err.(*ledger.InvalidCollNameError)
	require.True(t, ok)

	err = sim.SetPrivateData("ns1", "coll3", "key1", []byte("val1"))
	_, ok = err.(*ledger.InvalidCollNameError)
	require.True(t, ok)

	err = sim.SetPrivateData("ns1", "coll1", "key1", []byte("val1"))
	require.NoError(t, err)
}

func TestPvtGetNoCollection(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "test-pvtdata-get-no-collection", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	qe := newQueryExecutor(txMgr, "", nil, true, testHashFunc)
	valueHash, metadataBytes, err := qe.getPrivateDataValueHash("cc", "coll", "key")
	require.Nil(t, valueHash)
	require.Nil(t, metadataBytes)
	require.Error(t, err)
	require.IsType(t, &ledger.CollConfigNotDefinedError{}, err)
}

func TestPvtPutNoCollection(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "test-pvtdata-put-no-collection", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	txsim, err := txMgr.NewTxSimulator("txid")
	require.NoError(t, err)
	err = txsim.SetPrivateDataMetadata("cc", "coll", "key", map[string][]byte{})
	require.Error(t, err)
	require.IsType(t, &ledger.CollConfigNotDefinedError{}, err)
}

func TestNoCollectionValidationCheck(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "test-no-collection-validation-check", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	qe, err := txMgr.NewQueryExecutorNoCollChecks()
	require.NoError(t, err)
	valueHash, err := qe.GetPrivateDataHash("cc", "coll", "key")
	require.Nil(t, valueHash)
	require.NoError(t, err)
}
