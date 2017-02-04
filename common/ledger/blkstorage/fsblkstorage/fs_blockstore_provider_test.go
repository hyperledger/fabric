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

package fsblkstorage

import (
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
)

func TestMultipleBlockStores(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath, 0))
	defer env.Cleanup()

	provider := env.provider
	store1, _ := provider.OpenBlockStore("ledger1")
	defer store1.Shutdown()

	store2, _ := provider.OpenBlockStore("ledger2")
	defer store2.Shutdown()

	blocks1 := testutil.ConstructTestBlocks(t, 5)
	for _, b := range blocks1 {
		store1.AddBlock(b)
	}

	blocks2 := testutil.ConstructTestBlocks(t, 10)
	for _, b := range blocks2 {
		store2.AddBlock(b)
	}
	checkBlocks(t, blocks1, store1)
	checkBlocks(t, blocks2, store2)
}

func checkBlocks(t *testing.T, expectedBlocks []*common.Block, store blkstorage.BlockStore) {
	bcInfo, _ := store.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo.Height, uint64(len(expectedBlocks)))
	testutil.AssertEquals(t, bcInfo.CurrentBlockHash, expectedBlocks[len(expectedBlocks)-1].GetHeader().Hash())

	itr, _ := store.RetrieveBlocks(1)
	for i := 0; i < len(expectedBlocks); i++ {
		blockHolder, _ := itr.Next()
		block := blockHolder.(ledger.BlockHolder).GetBlock()
		testutil.AssertEquals(t, block, expectedBlocks[i])
	}
}

func TestBlockStoreProvider(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath, 0))
	defer env.Cleanup()

	provider := env.provider
	stores := []blkstorage.BlockStore{}
	numStores := 10
	for i := 0; i < numStores; i++ {
		store, _ := provider.OpenBlockStore(constructLedgerid(i))
		defer store.Shutdown()
		stores = append(stores, store)
	}

	storeNames, _ := provider.List()
	testutil.AssertEquals(t, len(storeNames), numStores)

	for i := 0; i < numStores; i++ {
		exists, err := provider.Exists(constructLedgerid(i))
		testutil.AssertNoError(t, err, "")
		testutil.AssertEquals(t, exists, true)
	}

	exists, err := provider.Exists(constructLedgerid(numStores + 1))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, exists, false)

}

func constructLedgerid(id int) string {
	return fmt.Sprintf("ledger_%d", id)
}
