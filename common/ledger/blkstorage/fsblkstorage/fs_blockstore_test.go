/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrongBlockNumber(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()

	provider := env.provider
	store, _ := provider.OpenBlockStore("testLedger")
	defer store.Shutdown()

	blocks := testutil.ConstructTestBlocks(t, 5)
	for i := 0; i < 3; i++ {
		err := store.AddBlock(blocks[i])
		assert.NoError(t, err)
	}
	err := store.AddBlock(blocks[4])
	assert.Error(t, err, "Error shold have been thrown when adding block number 4 while block number 3 is expected")
}

func TestTxIDIndexErrorPropagations(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()

	provider := env.provider
	store, _ := provider.OpenBlockStore("testLedger")
	defer store.Shutdown()
	blocks := testutil.ConstructTestBlocks(t, 3)
	for i := 0; i < 3; i++ {
		err := store.AddBlock(blocks[i])
		assert.NoError(t, err)
	}

	fsStore := store.(*fsBlockStore)
	index := fsStore.fileMgr.db

	txIDBasedFunctions := []func() error{
		func() error {
			_, err := fsStore.RetrieveTxByID("junkTxID")
			return err
		},
		func() error {
			_, err := fsStore.RetrieveBlockByTxID("junkTxID")
			return err
		},
		func() error {
			_, err := fsStore.RetrieveTxValidationCodeByTxID("junkTxID")
			return err
		},
	}

	index.Put(
		constructTxIDKey("junkTxID", 5, 4),
		[]byte("junkValue"),
		false,
	)
	expectedErrMsg := fmt.Sprintf("unexpected error while unmarshaling bytes [%#v] into TxIDIndexValProto:", []byte("junkValue"))
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
