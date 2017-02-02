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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/util"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/core/mocks/validator"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestKVLedgerBlockStorage(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	ledger, _ := ledgermgmt.CreateLedger("TestLedger")
	defer ledger.Close()

	validator := &txValidator{&mocktxvalidator.Support{LedgerVal: ledger}, &validator.MockVsccValidator{}}

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
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

func TestNewTxValidator_DuplicateTransactions(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	ledger, _ := ledgermgmt.CreateLedger("TestLedger")
	defer ledger.Close()

	validator := &txValidator{&mocktxvalidator.Support{LedgerVal: ledger}, &validator.MockVsccValidator{}}

	// Create simeple endorsement transaction
	payload := &common.Payload{
		Header: &common.Header{
			ChainHeader: &common.ChainHeader{
				TxID:    "simple_txID", // Fake txID
				Type:    int32(common.HeaderType_ENDORSER_TRANSACTION),
				ChainID: util2.GetTestChainID(),
			},
		},
		Data: []byte("test"),
	}

	payloadBytes, err := proto.Marshal(payload)

	// Check marshaling didn't fail
	assert.NoError(t, err)

	// Envelope the payload
	envelope := &common.Envelope{
		Payload: payloadBytes,
	}

	envelopeBytes, err := proto.Marshal(envelope)

	// Check marshaling didn't fail
	assert.NoError(t, err)

	block := &common.Block{
		Data: &common.BlockData{
			// Enconde transactions
			Data: [][]byte{envelopeBytes},
		},
	}

	block.Header = &common.BlockHeader{
		Number:   1,
		DataHash: block.Data.Hash(),
	}

	// Initialize metadata
	utils.InitBlockMetadata(block)
	// Commit block to the ledger
	ledger.Commit(block)

	// Validation should invalidate transaction,
	// because it's already committed
	validator.Validate(block)

	txsfltr := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	assert.True(t, txsfltr.IsSet(0))
}
