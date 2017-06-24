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
	"os"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	policymocks "github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/protos/common"
	peer2 "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func setupTestLedger(chainid string, path string) (*shim.MockStub, error) {
	viper.Set("peer.fileSystemPath", path)
	peer.MockInitialize()
	peer.MockCreateChain(chainid)

	lq := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", lq)
	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		return nil, fmt.Errorf("Init failed for test ledger [%s] with message: %s", chainid, string(res.Message))
	}
	return stub, nil
}

func TestQueryGetChainInfo(t *testing.T) {
	chainid := "mytestchainid1"
	path := "/var/hyperledger/test1/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetChainInfo), []byte(chainid)}
	res := stub.MockInvoke("1", args)
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
	path := "/var/hyperledger/test2/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte("1")}
	res := stub.MockInvoke("1", args)
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
	path := "/var/hyperledger/test3/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	// block number 0 (genesis block) would already be present in the ledger
	args := [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("0")}
	res := stub.MockInvoke("1", args)
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
	path := "/var/hyperledger/test4/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte("0")}
	res := stub.MockInvoke("1", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByHash should have failed with invalid hash: 0")

	args = [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte(nil)}
	res = stub.MockInvoke("2", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByHash should have failed with nil hash")
}

func TestQueryGetBlockByTxID(t *testing.T) {
	chainid := "mytestchainid5"
	path := "/var/hyperledger/test5/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte(GetBlockByTxID), []byte(chainid), []byte("")}
	res := stub.MockInvoke("1", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlockByTxID should have failed with blank txId.")
}

func TestFailingAccessControl(t *testing.T) {
	chainid := "mytestchainid6"
	path := "/var/hyperledger/test6/"
	_, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}
	e := new(LedgerQuerier)
	// Init the policy checker to have a failure
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			chainid: &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: &policymocks.MockIdentityDeserializer{Identity: []byte("Alice"), Msg: []byte("msg1")}}},
		},
	}
	e.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		&policymocks.MockIdentityDeserializer{Identity: []byte("Alice"), Msg: []byte("msg1")},
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	stub := shim.NewMockStub("LedgerQuerier", e)

	args := [][]byte{[]byte(GetChainInfo), []byte(chainid)}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	policyManagerGetter.Managers[chainid].(*policymocks.MockChannelPolicyManager).MockPolicy.(*policymocks.MockPolicy).Deserializer.(*policymocks.MockIdentityDeserializer).Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res := stub.MockInvokeWithSignedProposal("2", args, sProp)
	assert.Equal(t, int32(shim.OK), res.Status, "GetChainInfo failed with err: %s", res.Message)

	sProp, _ = utils.MockSignedEndorserProposalOrPanic(chainid, &peer2.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
	res = stub.MockInvokeWithSignedProposal("3", args, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetChainInfo must fail: %s", res.Message)
	assert.True(t, strings.HasPrefix(res.Message, "Authorization request failed"))
}

func TestQueryNonexistentFunction(t *testing.T) {
	chainid := "mytestchainid7"
	path := "/var/hyperledger/test7/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	args := [][]byte{[]byte("GetBlocks"), []byte(chainid), []byte("arg1")}
	res := stub.MockInvoke("1", args)
	assert.Equal(t, int32(shim.ERROR), res.Status, "GetBlocks should have failed because the function does not exist")
}

// TestQueryGeneratedBlock tests various queries for a newly generated block
// that contains two transactions
func TestQueryGeneratedBlock(t *testing.T) {
	chainid := "mytestchainid8"
	path := "/var/hyperledger/test8/"
	stub, err := setupTestLedger(chainid, path)
	defer os.RemoveAll(path)
	if err != nil {
		t.Fatalf(err.Error())
	}

	block1 := addBlockForTesting(t, chainid)

	// block number 1 should now exist
	args := [][]byte{[]byte(GetBlockByNumber), []byte(chainid), []byte("1")}
	res := stub.MockInvoke("1", args)
	assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByNumber should have succeeded for block number 1")

	// block number 1
	args = [][]byte{[]byte(GetBlockByHash), []byte(chainid), []byte(block1.Header.Hash())}
	res = stub.MockInvoke("2", args)
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
					res = stub.MockInvoke("3", args)
					assert.Equal(t, int32(shim.OK), res.Status, "GetBlockByTxId should have succeeded for txid: %s", chdr.TxId)

					args = [][]byte{[]byte(GetTransactionByID), []byte(chainid), []byte(chdr.TxId)}
					res = stub.MockInvoke("4", args)
					assert.Equal(t, int32(shim.OK), res.Status, "GetTransactionById should have succeeded for txid: %s", chdr.TxId)
				}
			}
		}
	}
}

func addBlockForTesting(t *testing.T, chainid string) *common.Block {
	bg, _ := testutil.NewBlockGenerator(t, chainid, false)
	ledger := peer.GetLedger(chainid)
	defer ledger.Close()

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes1, _ := simulator.GetTxSimulationResults()
	simulator, _ = ledger.NewTxSimulator()
	simulator.SetState("ns2", "key4", []byte("value4"))
	simulator.SetState("ns2", "key5", []byte("value5"))
	simulator.SetState("ns2", "key6", []byte("value6"))
	simulator.Done()
	simRes2, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes1, simRes2})
	ledger.Commit(block1)

	return block1
}
