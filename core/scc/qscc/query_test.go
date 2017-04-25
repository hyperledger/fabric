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
	"testing"

	"github.com/spf13/viper"

	"strings"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	peer2 "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test1/")
	defer os.RemoveAll("/var/hyperledger/test1/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid1")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func TestQueryGetChainInfo(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test2/")
	defer os.RemoveAll("/var/hyperledger/test2/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid2")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	args := [][]byte{[]byte(GetChainInfo), []byte("mytestchainid2")}
	if res := stub.MockInvoke("2", args); res.Status != shim.OK {
		t.Fatalf("qscc GetChainInfo failed with err: %s", res.Message)
	}
}

func TestQueryGetTransactionByID(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test3/")
	defer os.RemoveAll("/var/hyperledger/test3/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid3")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	args := [][]byte{[]byte(GetTransactionByID), []byte("mytestchainid3"), []byte("1")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatal("qscc getTransactionByID should have failed with invalid txid: 2")
	}
}

func TestQueryWithWrongParameters(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test4/")
	defer os.RemoveAll("/var/hyperledger/test4/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid4")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Test with wrong number of parameters
	args := [][]byte{[]byte(GetTransactionByID), []byte("mytestchainid4")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatal("qscc getTransactionByID should have failed with invalid txid: 2")
	}
}

func TestQueryGetBlockByNumber(t *testing.T) {
	//t.Skip()
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test5/")
	defer os.RemoveAll("/var/hyperledger/test5/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid5")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
	// block number 0 (genesis block) would already be present in the ledger
	args := [][]byte{[]byte(GetBlockByNumber), []byte("mytestchainid5"), []byte("1")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatal("qscc GetBlockByNumber should have failed with invalid number: 1")
	}
}

func TestQueryGetBlockByHash(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test6/")
	defer os.RemoveAll("/var/hyperledger/test6/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid6")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	args := [][]byte{[]byte(GetBlockByHash), []byte("mytestchainid6"), []byte("0")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatal("qscc GetBlockByHash should have failed with invalid hash: 0")
	}
}

func TestQueryGetBlockByTxID(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test8/")
	defer os.RemoveAll("/var/hyperledger/test8/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid8")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	txID := ""

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	args := [][]byte{[]byte(GetBlockByTxID), []byte("mytestchainid8"), []byte(txID)}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("qscc GetBlockByTxID should have failed with invalid txID: %s", txID)
	}
}

func TestFailingAccessControl(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test9/")
	defer os.RemoveAll("/var/hyperledger/test9/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid9")

	e := new(LedgerQuerier)
	// Init the policy checker to have a failure
	policyManagerGetter := &policy.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"mytestchainid9": &policy.MockChannelPolicyManager{MockPolicy: &policy.MockPolicy{Deserializer: &policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}

	e.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		&policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")},
		&policy.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	stub := shim.NewMockStub("LedgerQuerier", e)

	args := [][]byte{[]byte(GetChainInfo), []byte("mytestchainid9")}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("mytestchainid9", &peer2.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	policyManagerGetter.Managers["mytestchainid9"].(*policy.MockChannelPolicyManager).MockPolicy.(*policy.MockPolicy).Deserializer.(*policy.MockIdentityDeserializer).Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("qscc GetChainInfo failed with err: %s", res.Message)
	}

	sProp, _ = utils.MockSignedEndorserProposalOrPanic("mytestchainid9", &peer2.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
	res := stub.MockInvokeWithSignedProposal("3", args, sProp)
	if res.Status == shim.OK {
		t.Fatalf("qscc GetChainInfo must fail: %s", res.Message)
	}
	assert.True(t, strings.HasPrefix(res.Message, "Authorization request failed"))
}
