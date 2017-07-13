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
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/util"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/core/mocks/validator"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestBlockValidation(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	gbHash := gb.Header.Hash()
	ledger, _ := ledgermgmt.CreateLedger(gb)
	defer ledger.Close()

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()

	_, err := testutil.ConstructBytesProposalResponsePayload("v1", simRes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	mockVsccValidator := &validator.MockVsccValidator{}
	tValidator := &txValidator{&mocktxvalidator.Support{LedgerVal: ledger}, mockVsccValidator}

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil})

	block := testutil.ConstructBlock(t, 1, gbHash, [][]byte{simRes}, true)

	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))

	/*

		a better way of testing this without all of the mocking was
		implemented in validator_test.go

		newMockVsccValidator := &validator.MockVsccValidator{
			CIns:     upgradeChaincodeIns,
			RespPayl: prespPaylBytes,
		}
		newTxValidator := &txValidator{&mocktxvalidator.Support{LedgerVal: ledger}, newMockVsccValidator}

		// generate new block
		newBlock := testutil.ConstructBlock(t, 2, block.Header.Hash(), [][]byte{simRes}, true) // contains one tx with chaincode version v1

		newTxValidator.Validate(newBlock)

		// tx should be invalided because of chaincode upgrade
		txsfltr = util.TxValidationFlags(newBlock.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_EXPIRED_CHAINCODE))
	*/
}

func TestNewTxValidator_DuplicateTransactions(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	ledger, _ := ledgermgmt.CreateLedger(gb)

	defer ledger.Close()

	tValidator := &txValidator{&mocktxvalidator.Support{LedgerVal: ledger}, &validator.MockVsccValidator{}}

	// Create simple endorsement transaction
	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
				TxId:      "simple_txID", // Fake txID
				Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
				ChannelId: util2.GetTestChainID(),
			}),
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
		Number:   0,
		DataHash: block.Data.Hash(),
	}

	// Initialize metadata
	utils.InitBlockMetadata(block)
	// Commit block to the ledger
	ledger.Commit(block)

	// Validation should invalidate transaction,
	// because it's already committed
	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.True(t, txsfltr.IsInvalid(0))
}

func createCCUpgradeEnvelope(chainID, chaincodeName, chaincodeVersion string, signer msp.SigningIdentity) (*common.Envelope, error) {
	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	spec := &peer.ChaincodeSpec{
		Type: peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
		ChaincodeId: &peer.ChaincodeID{
			Path:    "github.com/codePath",
			Name:    chaincodeName,
			Version: chaincodeVersion,
		},
	}

	cds := &peer.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: []byte{}}
	prop, _, err := utils.CreateUpgradeProposalFromCDS(chainID, cds, creator, []byte{}, []byte{}, []byte{})
	if err != nil {
		return nil, err
	}

	proposalResponse := &peer.ProposalResponse{
		Response: &peer.Response{
			Status: 200, // endorsed successfully
		},
		Endorsement: &peer.Endorsement{},
	}

	return utils.CreateSignedTx(prop, signer, proposalResponse)
}

func TestGetTxCCInstance(t *testing.T) {
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		t.Fatalf("Could not initialize msp, err: %s", err)
	}
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("Could not initialize signer, err: %s", err)
	}

	chainID := util2.GetTestChainID()
	upgradeCCName := "mycc"
	upgradeCCVersion := "v1"

	env, err := createCCUpgradeEnvelope(chainID, upgradeCCName, upgradeCCVersion, signer)
	assert.NoError(t, err)

	// get the payload from the envelope
	payload, err := utils.GetPayload(env)
	assert.NoError(t, err)

	expectInvokeCCIns := &sysccprovider.ChaincodeInstance{
		ChainID:          chainID,
		ChaincodeName:    "lscc",
		ChaincodeVersion: "",
	}
	expectUpgradeCCIns := &sysccprovider.ChaincodeInstance{
		ChainID:          chainID,
		ChaincodeName:    upgradeCCName,
		ChaincodeVersion: upgradeCCVersion,
	}

	tValidator := &txValidator{}
	invokeCCIns, upgradeCCIns, err := tValidator.getTxCCInstance(payload)
	if err != nil {
		t.Fatalf("Get chaincode from tx error: %s", err)
	}
	assert.EqualValues(t, expectInvokeCCIns, invokeCCIns)
	assert.EqualValues(t, expectUpgradeCCIns, upgradeCCIns)
}

func TestInvalidTXsForUpgradeCC(t *testing.T) {
	txsChaincodeNames := map[int]*sysccprovider.ChaincodeInstance{
		0: &sysccprovider.ChaincodeInstance{"chain0", "cc0", "v0"}, // invoke cc0/chain0:v0, should not be affected by upgrade tx in other chain
		1: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v0"}, // invoke cc0/chain1:v0, should be invalided by cc1/chain1 upgrade tx
		2: &sysccprovider.ChaincodeInstance{"chain1", "lscc", ""},  // upgrade cc0/chain1 to v1, should be invalided by latter cc0/chain1 upgtade tx
		3: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v0"}, // invoke cc0/chain1:v0, should be invalided by cc1/chain1 upgrade tx
		4: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v1"}, // invoke cc0/chain1:v1, should be invalided by cc1/chain1 upgrade tx
		5: &sysccprovider.ChaincodeInstance{"chain1", "cc1", "v0"}, // invoke cc1/chain1:v0, should not be affected by other chaincode upgrade tx
		6: &sysccprovider.ChaincodeInstance{"chain1", "lscc", ""},  // upgrade cc0/chain1 to v2, should be invalided by latter cc0/chain1 upgtade tx
		7: &sysccprovider.ChaincodeInstance{"chain1", "lscc", ""},  // upgrade cc0/chain1 to v3
	}
	upgradedChaincodes := map[int]*sysccprovider.ChaincodeInstance{
		2: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v1"},
		6: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v2"},
		7: &sysccprovider.ChaincodeInstance{"chain1", "cc0", "v3"},
	}

	txsfltr := ledgerUtil.NewTxValidationFlags(8)
	txsfltr.SetFlag(0, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(1, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(2, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(3, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(4, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(5, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(6, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(7, peer.TxValidationCode_VALID)

	expectTxsFltr := ledgerUtil.NewTxValidationFlags(8)
	expectTxsFltr.SetFlag(0, peer.TxValidationCode_VALID)
	expectTxsFltr.SetFlag(1, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(2, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(3, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(4, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(5, peer.TxValidationCode_VALID)
	expectTxsFltr.SetFlag(6, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(7, peer.TxValidationCode_VALID)

	tValidator := &txValidator{}
	finalfltr := tValidator.invalidTXsForUpgradeCC(txsChaincodeNames, upgradedChaincodes, txsfltr)

	assert.EqualValues(t, expectTxsFltr, finalfltr)
}
