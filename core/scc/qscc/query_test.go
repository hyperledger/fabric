/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package qscc

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	ledger2 "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos/common"
	peer2 "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestLedger(chainid string, path string) (*shim.MockStub, error) {
	mockAclProvider.Reset()

	viper.Set("peer.fileSystemPath", path)
	peer.MockInitialize()
	peer.MockCreateChain(chainid)

	lq := &LedgerQuerier{
		aclProvider: mockAclProvider,
	}
	stub := shim.NewMockStub("LedgerQuerier", lq)
	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		return nil, fmt.Errorf("Init failed for test ledger [%s] with message: %s", chainid, string(res.Message))
	}
	return stub, nil
}

//pass the prop so we can conveniently inline it in the call and get it back
func resetProvider(res, chainid string, prop *peer2.SignedProposal, retErr error) *peer2.SignedProposal {
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", res, chainid, prop).Return(retErr)
	return prop
}

func tempDir(t *testing.T, stem string) string {
	path, err := ioutil.TempDir("", "qscc-"+stem)
	require.NoError(t, err)
	return path
}

func TestQueryGetChainInfo(t *testing.T) {
	chainid := "mytestchainid1"
	path := tempDir(t, "test1")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetChainInfo), []byte(chainid)}
	prop := resetProvider(resources.Qscc_GetChainInfo, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.OK), res.Status, "GetChainInfo failed with err: %s", res.Message)

	args = [][]byte{[]byte(GetChainInfo)}
	res = stub.MockInvoke("2", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetChainInfo should have failed because no channel id was provided")

	args = [][]byte{[]byte(GetChainInfo), []byte("fakechainid")}
	res = stub.MockInvoke("3", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetChainInfo should have failed because the channel id does not exist")
}

func TestQueryGetTransactionByID(t *testing.T) {
	chainid := "mytestchainid2"
	path := tempDir(t, "test2")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte("1")}
	prop := resetProvider(resources.Qscc_GetTransactionByID, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetTransactionByID should have failed with invalid txid: 1")

	args = [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte(nil)}
	res = stub.MockInvoke("2", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetTransactionByID should have failed with invalid txid: nil")

	// Test with wrong number of parameters
	args = [][]byte{[]byte(GetTransactionByID), []byte(chainid)}
	res = stub.MockInvoke("3", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetTransactionByID should have failed due to incorrect number of arguments")
}

func TestQueryGetBlockByNumber(t *testing.T) {
	chainid := "mytestchainid3"
	path := tempDir(t, "test3")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	// block number 0 (genesis block) would already be present in the ledger
	args := [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("0")}
	prop := resetProvider(resources.Qscc_GetBlockByNumber, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByNumber should have succeeded for block number: 0")

	// block number 1 should not be present in the ledger
	args = [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("1")}
	res = stub.MockInvoke("2", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByNumber should have failed with invalid number: 1")

	// block number cannot be nil
	args = [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte(nil)}
	res = stub.MockInvoke("3", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByNumber should have failed with nil block number")
}

func TestQueryGetBlockByHash(t *testing.T) {
	chainid := "mytestchainid4"
	path := tempDir(t, "test4")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte("0")}
	prop := resetProvider(resources.Qscc_GetBlockByHash, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByHash should have failed with invalid hash: 0")

	args = [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte(nil)}
	res = stub.MockInvoke("2", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByHash should have failed with nil hash")
}

func TestQueryGetBlockByTxID(t *testing.T) {
	chainid := "mytestchainid5"
	path := tempDir(t, "test5")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetBlockByTxID), []byte(chainid), []byte("")}
	prop := resetProvider(resources.Qscc_GetBlockByTxID, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByTxID should have failed with blank txId.")
}

func TestFailingAccessControl(t *testing.T) {
	chainid := "mytestchainid6"
	path := tempDir(t, "test6")
	defer os.RemoveAll(path)

	_, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}
	e := &LedgerQuerier{
		aclProvider: mockAclProvider,
	}
	stub := shim.NewMockStub("LedgerQuerier", e)

	// GetChainInfo
	args := [][]byte{[]byte(GetChainInfo), []byte(chainid)}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes
	// Set the ACLProvider to have a failure
	resetProvider(resources.Qscc_GetChainInfo, chainid, sProp, errors.New("Failed access control"))
	res := stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetChainInfo must fail: %s", res.Message)
	assert.Contains(t, res.Message, "Failed access control")
	// assert that the expectations were met
	mockAclProvider.AssertExpectations(t)

	// GetBlockByNumber
	args = [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("1")}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes
	// Set the ACLProvider to have a failure
	resetProvider(resources.Qscc_GetBlockByNumber, chainid, sProp, errors.New("Failed access control"))
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByNumber must fail: %s", res.Message)
	assert.Contains(t, res.Message, "Failed access control")
	// assert that the expectations were met
	mockAclProvider.AssertExpectations(t)

	// GetBlockByHash
	args = [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte("1")}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes
	// Set the ACLProvider to have a failure
	resetProvider(resources.Qscc_GetBlockByHash, chainid, sProp, errors.New("Failed access control"))
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByHash must fail: %s", res.Message)
	assert.Contains(t, res.Message, "Failed access control")
	// assert that the expectations were met
	mockAclProvider.AssertExpectations(t)

	// GetBlockByTxID
	args = [][]byte{[]byte(GetBlockByTxID), []byte(chainid), []byte("1")}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes
	// Set the ACLProvider to have a failure
	resetProvider(resources.Qscc_GetBlockByTxID, chainid, sProp, errors.New("Failed access control"))
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByTxID must fail: %s", res.Message)
	assert.Contains(t, res.Message, "Failed access control")
	// assert that the expectations were met
	mockAclProvider.AssertExpectations(t)

	// GetTransactionByID
	args = [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte("1")}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes
	// Set the ACLProvider to have a failure
	resetProvider(resources.Qscc_GetTransactionByID, chainid, sProp, errors.New("Failed access control"))
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "Qscc_GetTransactionByID must fail: %s", res.Message)
	assert.Contains(t, res.Message, "Failed access control")
	// assert that the expectations were met
	mockAclProvider.AssertExpectations(t)
}

func TestQueryNonexistentFunction(t *testing.T) {
	chainid := "mytestchainid7"
	path := tempDir(t, "test7")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte("GetBlocks"), []byte(chainid), []byte("arg1")}
	prop := resetProvider("qscc/GetBlocks", chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlocks should have failed because the function does not exist")
}

// TestQueryGeneratedBlock tests various queries for a newly generated block
// that contains two transactions
func TestQueryGeneratedBlock(t *testing.T) {
	chainid := "mytestchainid8"
	path := tempDir(t, "test8")
	defer os.RemoveAll(path)

	stub, err := setupTestLedger(chainid, path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	block1 := addBlockForTesting(t, chainid)

	// block number 1 should now exist
	args := [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("1")}
	prop := resetProvider(resources.Qscc_GetBlockByNumber, chainid, &peer2.SignedProposal{}, nil)
	res := stub.MockInvokeWithSignedProposal("1", args, prop)
	assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByNumber should have succeeded for block number 1")

	// block number 1
	args = [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte(block1.Header.Hash())}
	prop = resetProvider(resources.Qscc_GetBlockByHash, chainid, &peer2.SignedProposal{}, nil)
	res = stub.MockInvokeWithSignedProposal("2", args, prop)
	assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByHash should have succeeded for block 1 hash")

	// drill into the block to find the transaction ids it contains
	for _, d := range block1.Data.Data {
		ebytes := d
		if ebytes != nil {
			if env, err := utils.GetEnvelopeFromBlock(ebytes); err != nil {
				t.Fatalf("error getting envelope from block: %s", err)
			} else if env != nil {
				payload, err := utils.GetPayload(env)
				if err != nil {
					t.Fatalf("error extracting payload from envelope: %s", err)
				}
				chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
				if err != nil {
					t.Fatalf(err.Error())
				}
				if common.HeaderType(chdr.Type) == common.HeaderType_ENDORSER_TRANSACTION {
					args = [][]byte{[]byte(GetBlockByTxID), []byte(chainid), []byte(chdr.TxId)}
					mockAclProvider.Reset()
					prop = resetProvider(resources.Qscc_GetBlockByTxID, chainid, &peer2.SignedProposal{}, nil)
					res = stub.MockInvokeWithSignedProposal("3", args, prop)
					assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByTxId should have succeeded for txid: %s", chdr.TxId)

					args = [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte(chdr.TxId)}
					prop = resetProvider(resources.Qscc_GetTransactionByID, chainid, &peer2.SignedProposal{}, nil)
					res = stub.MockInvokeWithSignedProposal("4", args, prop)
					assert.Equal(t, int32(shim.OK), res.Status, "GetTransactionById should have succeeded for txid: %s", chdr.TxId)
				}
			}
		}
	}
}

func addBlockForTesting(t *testing.T, chainid string) *common.Block {
	ledger := peer.GetLedger(chainid)
	defer ledger.Close()

	txid1 := util.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid1)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes1, _ := simulator.GetTxSimulationResults()
	pubSimResBytes1, _ := simRes1.GetPubSimulationBytes()

	txid2 := util.GenerateUUID()
	simulator, _ = ledger.NewTxSimulator(txid2)
	simulator.SetState("ns2", "key4", []byte("value4"))
	simulator.SetState("ns2", "key5", []byte("value5"))
	simulator.SetState("ns2", "key6", []byte("value6"))
	simulator.Done()
	simRes2, _ := simulator.GetTxSimulationResults()
	pubSimResBytes2, _ := simRes2.GetPubSimulationBytes()

	bcInfo, err := ledger.GetBlockchainInfo()
	assert.NoError(t, err)
	block1 := testutil.ConstructBlock(t, 1, bcInfo.CurrentBlockHash, [][]byte{pubSimResBytes1, pubSimResBytes2}, false)
	ledger.CommitWithPvtData(&ledger2.BlockAndPvtData{Block: block1}, &ledger2.CommitOptions{})
	return block1
}

var mockAclProvider *mocks.MockACLProvider

func TestMain(m *testing.M) {
	mockAclProvider = &mocks.MockACLProvider{}
	mockAclProvider.Reset()

	os.Exit(m.Run())
}
