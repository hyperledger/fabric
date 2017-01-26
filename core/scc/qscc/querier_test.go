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

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/peer"
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

	args := [][]byte{[]byte(GetChainInfo), []byte("mytestchainid2")}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
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

	args := [][]byte{[]byte(GetTransactionByID), []byte("mytestchainid3"), []byte("1")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("qscc getTransactionByID should have failed with invalid txid: 1")
	}
}

func TestQueryWithWrongParameters(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test4/")
	defer os.RemoveAll("/var/hyperledger/test4/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid4")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	// Test with wrong number of parameters
	args := [][]byte{[]byte(GetTransactionByID), []byte("mytestchainid4")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("qscc getTransactionByID should have failed with invalid txid: 1")
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

	args := [][]byte{[]byte(GetBlockByNumber), []byte("mytestchainid5"), []byte("0")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("qscc GetBlockByNumber should have failed with invalid number: 0")
	}
}

func TestQueryGetBlockByHash(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test6/")
	defer os.RemoveAll("/var/hyperledger/test6/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid6")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)

	args := [][]byte{[]byte(GetBlockByHash), []byte("mytestchainid6"), []byte("0")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("qscc GetBlockByHash should have failed with invalid hash: 0")
	}
}

func TestQueryGetQueryResult(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test7/")
	defer os.RemoveAll("/var/hyperledger/test7/")
	peer.MockInitialize()
	peer.MockCreateChain("mytestchainid7")

	e := new(LedgerQuerier)
	stub := shim.NewMockStub("LedgerQuerier", e)
	qstring := "{\"selector\":{\"key\":\"value\"}}"
	args := [][]byte{[]byte(GetQueryResult), []byte("mytestchainid7"), []byte(qstring)}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("qscc GetQueryResult should have failed with invalid query: abc")
	}
}
