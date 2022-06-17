/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/stretchr/testify/require"
)

func TestWrongBlockNumber(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()

	provider := env.provider
	store, _ := provider.Open("testLedger")
	defer store.Shutdown()

	blocks := testutil.ConstructTestBlocks(t, 5)
	for i := 0; i < 3; i++ {
		err := store.AddBlock(blocks[i])
		require.NoError(t, err)
	}
	err := store.AddBlock(blocks[4])
	require.Error(t, err, "Error shold have been thrown when adding block number 4 while block number 3 is expected")
}

func TestTxIDIndexErrorPropagations(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()

	provider := env.provider
	store, _ := provider.Open("testLedger")
	defer store.Shutdown()
	blocks := testutil.ConstructTestBlocks(t, 3)
	for i := 0; i < 3; i++ {
		err := store.AddBlock(blocks[i])
		require.NoError(t, err)
	}

	index := store.fileMgr.db

	txIDBasedFunctions := []func() error{
		func() error {
			_, err := store.RetrieveTxByID("junkTxID")
			return err
		},
		func() error {
			_, err := store.RetrieveBlockByTxID("junkTxID")
			return err
		},
		func() error {
			_, _, err := store.RetrieveTxValidationCodeByTxID("junkTxID")
			return err
		},
	}

	index.Put(
		constructTxIDKey("junkTxID", 5, 4),
		[]byte("junkValue"),
		false,
	)
	expectedErrMsg := fmt.Sprintf("unexpected error while unmarshalling bytes [%#v] into TxIDIndexValProto:", []byte("junkValue"))
	for _, f := range txIDBasedFunctions {
		err := f()
		require.Error(t, err)
		require.Contains(t, err.Error(), expectedErrMsg)
	}

	env.provider.leveldbProvider.Close()
	expectedErrMsg = "error while trying to retrieve transaction info by TXID [junkTxID]:"
	for _, f := range txIDBasedFunctions {
		err := f()
		require.Error(t, err)
		require.Contains(t, err.Error(), expectedErrMsg)
	}
}
