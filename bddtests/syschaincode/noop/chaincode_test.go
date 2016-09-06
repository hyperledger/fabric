/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package noop

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos"
)

var something = "c29tZXRoaW5n"

func TestMocking(t *testing.T) {
	var mockledger, ledger ledgerHandler
	mockledger = mockLedger{}
	var noop = SystemChaincode{mockledger}
	ledger = noop.getLedger()
	if mockledger != ledger {
		t.Errorf("Mocking functionality of Noop system chaincode does not work.")
	}
}

func TestInvokeUnsupported(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Invoke(nil, "unsupported_operation", []string{"arg1", "arg2"})
	if res != nil || err == nil {
		t.Errorf("Invoke has to return nil and error when called with unsupported operation!")
	}
}

func TestInvokeExecuteNotEnoughArgs(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Invoke(nil, "", []string{})
	if res != nil || err == nil {
		t.Errorf("Invoke.execute has to indicate error if called with less than one arguments!")
	}
}

func TestInvokeExecuteOneArgReturnsNothing(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Invoke(nil, "transaction", []string{})
	if res != nil || err != nil {
		t.Errorf("Invoke.execute has to return nil with no error.")
	}
}

func TestInvokeExecuteMoreArgsReturnsError(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Invoke(nil, "transaction", []string{"arg1"})
	if res != nil || err == nil {
		t.Errorf("Invoke.execute has to return error when called with more than one arguments.")
	}
}

func TestQueryUnsupported(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Query(nil, "unsupported_operation", []string{"arg1", "arg2"})
	if res != nil || err == nil {
		t.Errorf("Invoke has to return nil and error when called with unsupported operation!")
	}
}

func TestQueryGetTranNotEnoughArgs(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Query(nil, "getTran", []string{})
	if res != nil || err == nil {
		t.Errorf("Invoke has to return nil and error when called with unsupported operation!")
	}
}

func TestQueryGetTranNonExisting(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Query(nil, "getTran", []string{"noSuchTX"})
	if res != nil || err == nil {
		t.Errorf("Invoke has to return nil when called with a non-existent transaction.")
	}
}

func TestQueryGetTranNonExistingWithManyArgs(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Query(nil, "getTran", []string{"noSuchTX", "arg2"})
	if res != nil || err == nil {
		t.Errorf("Invoke has to return nil when called with a non-existent transaction.")
	}
}

func TestQueryGetTranExisting(t *testing.T) {
	var noop = SystemChaincode{mockLedger{}}
	var res, err = noop.Query(nil, "getTran", []string{"someTx"})
	if res == nil || err != nil {
		t.Errorf("Invoke has to return a transaction when called with an existing one.")
	}
}

type mockLedger struct {
}

func (ml mockLedger) GetTransactionByID(txID string) (*protos.Transaction, error) {
	if txID == "noSuchTX" {
		return nil, fmt.Errorf("Some error")
	}
	newCCIS := &protos.ChaincodeInvocationSpec{ChaincodeSpec: &protos.ChaincodeSpec{CtorMsg: &protos.ChaincodeInput{Args: util.ToChaincodeArgs("execute", something)}}}
	pl, _ := proto.Marshal(newCCIS)
	return &protos.Transaction{Payload: pl}, nil
}
