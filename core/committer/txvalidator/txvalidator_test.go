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

package txvalidator

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

type mockVsccValidator struct {
}

func (v *mockVsccValidator) VSCCValidateTx(payload *common.Payload, envBytes []byte) error {
	return nil
}

func TestKVLedgerBlockStorage(t *testing.T) {
	conf := kvledger.NewConf("/tmp/tests/ledger/", 0)
	defer os.RemoveAll("/tmp/tests/ledger/")

	ledger, _ := kvledger.NewKVLedger(conf)
	defer ledger.Close()

	validator := &txValidator{ledger, &mockVsccValidator{}}

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	block := testutil.ConstructBlock(t, [][]byte{simRes}, true)

	validator.Validate(block)

	txsfltr := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	assert.True(t, !txsfltr.IsSet(uint(0)))
	assert.True(t, !txsfltr.IsSet(uint(1)))
	assert.True(t, !txsfltr.IsSet(uint(2)))
}
