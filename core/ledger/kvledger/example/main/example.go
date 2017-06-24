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
	"fmt"
	"os"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/example"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("main")

const (
	ledgerID = "Default"
)

var peerLedger ledger.PeerLedger
var app *example.App
var committer *example.Committer
var consenter *example.Consenter

var accounts = []string{"account1", "account2", "account3", "account4"}

func init() {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig()

	// Initialization will get a handle to the ledger at the specified path
	// Note, if subledgers are supported in the future,
	// the various ledgers could be created/managed at this level
	cleanup()
	ledgermgmt.Initialize()
	var err error

	gb, _ := configtxtest.MakeGenesisBlock(ledgerID)
	peerLedger, err = ledgermgmt.CreateLedger(gb)
	if err != nil {
		panic(fmt.Errorf("Error in NewKVLedger(): %s", err))
	}
	app = example.ConstructAppInstance(peerLedger)
	committer = example.ConstructCommitter(peerLedger)
	consenter = example.ConstructConsenter()
}

func main() {
	defer ledgermgmt.Close()

	// Each of the functions here will emulate endorser, orderer,
	// and committer by calling ledger APIs to similate the proposal,
	// get simulation results, create a transaction, add it to a block,
	// and then commit the block.

	// Initialize account balances by setting each account to 100
	initApp()

	printBalances()

	// Transfer money between accounts. Exercises happy path.
	transferFunds()

	printBalances()

	// Attempt to transfer more money than account balance
	// Exercises simulation failure
	tryInvalidTransfer()

	// Attempt two transactions, the first one will have sufficient funds,
	// the second one should fail since the account balance was updated
	// (by the first tran) since simulation time. This exercises the MVCC check.
	tryDoubleSpend()

	printBalances()
}

func initApp() {
	logger.Debug("Entering initApp()")
	tx, err := app.Init(map[string]int{
		accounts[0]: 100,
		accounts[1]: 100,
		accounts[2]: 100,
		accounts[3]: 100})
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx)
	err = committer.Commit(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock)
	logger.Debug("Exiting initApp()")
}

func transferFunds() {
	logger.Debug("Entering transferFunds()")
	tx1, err := app.TransferFunds("account1", "account2", 50)
	handleError(err, true)
	tx2, err := app.TransferFunds("account3", "account4", 50)
	handleError(err, true)

	// act as ordering service (consenter) to create a Raw Block from the Transaction
	rawBlock := consenter.ConstructBlock(tx1, tx2)

	// act as committing peer to commit the Raw Block
	err = committer.Commit(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock)
	logger.Debug("Exiting transferFunds")
}

func tryInvalidTransfer() {
	logger.Debug("Entering tryInvalidTransfer()")
	_, err := app.TransferFunds("account1", "account2", 60)
	handleError(err, false)
	logger.Debug("Exiting tryInvalidTransfer()")
}

func tryDoubleSpend() {
	logger.Debug("Entering tryDoubleSpend()")
	tx1, err := app.TransferFunds("account1", "account2", 50)
	handleError(err, true)
	tx2, err := app.TransferFunds("account1", "account4", 50)
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx1, tx2)
	err = committer.Commit(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock)
	logger.Debug("Exiting tryDoubleSpend()")
}

func printBlocksInfo(block *common.Block) {
	logger.Debug("Entering printBlocksInfo()")
	// Read invalid transactions filter
	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	numOfInvalid := 0
	// Count how many transaction indeed invalid
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			numOfInvalid++
		}
	}
	fmt.Printf("Num txs in rawBlock = [%d], num invalidTxs = [%d]\n",
		len(block.Data.Data), numOfInvalid)
	logger.Debug("Exiting printBlocksInfo()")
}

func printBalances() {
	logger.Debug("Entering printBalances()")
	balances, err := app.QueryBalances(accounts)
	handleError(err, true)
	for i := 0; i < len(accounts); i++ {
		fmt.Printf("[%s] = [%d]\n", accounts[i], balances[i])
	}
	logger.Debug("Exiting printBalances()")
}

func handleError(err error, quit bool) {
	if err != nil {
		if quit {
			panic(fmt.Errorf("Error: %s\n", err))
		} else {
			fmt.Printf("Error: %s\n", err)
		}
	}
}

func cleanup() {
	ledgerRootPath := ledgerconfig.GetRootPath()
	os.RemoveAll(ledgerRootPath)
}
