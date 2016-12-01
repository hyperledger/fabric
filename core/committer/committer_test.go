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

package committer

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/stretchr/testify/assert"

	pb "github.com/hyperledger/fabric/protos/peer"
)

func TestKVLedgerBlockStorage(t *testing.T) {
	conf := kvledger.NewConf("/tmp/tests/ledger/", 0)
	defer os.RemoveAll("/tmp/tests/ledger/")

	ledger, _ := kvledger.NewKVLedger(conf)
	defer ledger.Close()

	committer := NewLedgerCommitter(ledger)

	height, err := committer.LedgerHeight()
	assert.Equal(t, uint64(0), height)
	assert.NoError(t, err)

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	block1 := testutil.ConstructBlock(t, [][]byte{simRes}, true)

	err = committer.CommitBlock(block1)
	assert.NoError(t, err)

	height, err = committer.LedgerHeight()
	assert.Equal(t, uint64(1), height)
	assert.NoError(t, err)

	blocks := committer.GetBlocks([]uint64{1})
	assert.Equal(t, 1, len(blocks))
	assert.NoError(t, err)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 1, CurrentBlockHash: block1Hash, PreviousBlockHash: []byte{}})
}
