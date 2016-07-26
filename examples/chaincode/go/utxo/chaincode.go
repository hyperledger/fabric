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

package main

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/examples/chaincode/go/utxo/util"
)

// The UTXO example chaincode contains a single invocation function named execute. This function accepts BASE64
// encoded transactions from the Bitcoin network. This chaincode will parse the transactions and pass the transaction
// components to the Bitcoin libconsensus C library for script verification. A table of UTXOs is maintained to ensure
// each transaction is valid.
// Documentation can be found at
// https://github.com/hyperledger/fabric/blob/master/examples/chaincode/go/utxo/README.md

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init does nothing in the UTXO chaincode
func (t *SimpleChaincode) Init(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	return nil, nil
}

// Invoke callback representing the invocation of a chaincode
func (t *SimpleChaincode) Invoke(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	switch function {

	case "execute":

		if len(args) < 1 {
			return nil, errors.New("execute operation must include single argument, the base64 encoded form of a bitcoin transaction")
		}
		txDataBase64 := args[0]
		txData, err := base64.StdEncoding.DecodeString(txDataBase64)
		if err != nil {
			return nil, fmt.Errorf("Error decoding TX as base64:  %s", err)
		}

		utxo := util.MakeUTXO(MakeChaincodeStore(stub))
		execResult, err := utxo.Execute(txData)
		if err != nil {
			return nil, fmt.Errorf("Error executing TX:  %s", err)
		}

		fmt.Printf("\nExecResult: Coinbase: %t, SumInputs %d, SumOutputs %d\n\n", execResult.IsCoinbase, execResult.SumPriorOutputs, execResult.SumCurrentOutputs)

		if execResult.IsCoinbase == false {
			if execResult.SumCurrentOutputs > execResult.SumPriorOutputs {
				return nil, fmt.Errorf("sumOfCurrentOutputs > sumOfPriorOutputs: sumOfCurrentOutputs = %d, sumOfPriorOutputs = %d", execResult.SumCurrentOutputs, execResult.SumPriorOutputs)
			}
		}

		return nil, nil

	default:
		return nil, errors.New("Unsupported operation")
	}

}

// Query callback representing the query of a chaincode
func (t *SimpleChaincode) Query(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	switch function {

	case "getTran":

		if len(args) < 1 {
			return nil, errors.New("queryBTC operation must include single argument, the TX hash hex")
		}

		utxo := util.MakeUTXO(MakeChaincodeStore(stub))
		tx, err := utxo.Query(args[0])
		if err != nil {
			return nil, fmt.Errorf("Error querying for transaction:  %s", err)
		}
		if tx == nil {
			var data []byte
			return data, nil
		}
		return tx, nil

	default:
		return nil, errors.New("Unsupported operation")
	}

}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting chaincode: %s", err)
	}
}
