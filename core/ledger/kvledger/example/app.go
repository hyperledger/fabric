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

package example

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	ptestutils "github.com/hyperledger/fabric/protos/testutils"
)

// App - a sample fund transfer app
type App struct {
	name   string
	ledger ledger.PeerLedger
}

// ConstructAppInstance constructs an instance of an app
func ConstructAppInstance(ledger ledger.PeerLedger) *App {
	return &App{"PaymentApp", ledger}
}

// Init simulates init transaction
func (app *App) Init(initialBalances map[string]int) (*common.Envelope, error) {
	var txSimulator ledger.TxSimulator
	var err error
	if txSimulator, err = app.ledger.NewTxSimulator(); err != nil {
		return nil, err
	}
	defer txSimulator.Done()
	for accountID, bal := range initialBalances {
		txSimulator.SetState(app.name, accountID, toBytes(bal))
	}
	var txSimulationResults []byte
	if txSimulationResults, err = txSimulator.GetTxSimulationResults(); err != nil {
		return nil, err
	}
	tx := constructTransaction(txSimulationResults)
	return tx, nil
}

// TransferFunds simulates a transaction for transferring fund from fromAccount to toAccount
func (app *App) TransferFunds(fromAccount string, toAccount string, transferAmt int) (*common.Envelope, error) {
	// act as endorsing peer shim code to simulate a transaction on behalf of chaincode
	var txSimulator ledger.TxSimulator
	var err error
	if txSimulator, err = app.ledger.NewTxSimulator(); err != nil {
		return nil, err
	}
	defer txSimulator.Done()
	var balFromBytes []byte
	if balFromBytes, err = txSimulator.GetState(app.name, fromAccount); err != nil {
		return nil, err
	}
	balFrom := toInt(balFromBytes)
	if balFrom-transferAmt < 0 {
		return nil, fmt.Errorf("Not enough balance in account [%s]. Balance = [%d], transfer request = [%d]",
			fromAccount, balFrom, transferAmt)
	}

	var balToBytes []byte
	if balToBytes, err = txSimulator.GetState(app.name, toAccount); err != nil {
		return nil, err
	}
	balTo := toInt(balToBytes)
	txSimulator.SetState(app.name, fromAccount, toBytes(balFrom-transferAmt))
	txSimulator.SetState(app.name, toAccount, toBytes(balTo+transferAmt))
	var txSimulationResults []byte
	if txSimulationResults, err = txSimulator.GetTxSimulationResults(); err != nil {
		return nil, err
	}

	// act as endorsing peer to create an Action with the SimulationResults
	// then act as SDK to create a Transaction with the EndorsedAction
	tx := constructTransaction(txSimulationResults)
	return tx, nil
}

// QueryBalances queries the balance funds
func (app *App) QueryBalances(accounts []string) ([]int, error) {
	var queryExecutor ledger.QueryExecutor
	var err error
	if queryExecutor, err = app.ledger.NewQueryExecutor(); err != nil {
		return nil, err
	}
	defer queryExecutor.Done()
	balances := make([]int, len(accounts))
	for i := 0; i < len(accounts); i++ {
		var balBytes []byte
		if balBytes, err = queryExecutor.GetState(app.name, accounts[i]); err != nil {
			return nil, err
		}
		balances[i] = toInt(balBytes)
	}
	return balances, nil
}

func constructTransaction(simulationResults []byte) *common.Envelope {
	ccid := &pb.ChaincodeID{
		Name:    "foo",
		Version: "v1",
	}
	response := &pb.Response{Status: 200}
	txEnv, _, _ := ptestutils.ConstructSingedTxEnvWithDefaultSigner(util.GetTestChainID(), ccid, response, simulationResults, nil, nil)
	return txEnv
}

func toBytes(balance int) []byte {
	return proto.EncodeVarint(uint64(balance))
}

func toInt(balanceBytes []byte) int {
	v, _ := proto.DecodeVarint(balanceBytes)
	return int(v)
}
