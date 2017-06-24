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

const (
	ledgerID = "Default"
)

var logger = logging.MustGetLogger("main")

var peerLedger ledger.PeerLedger
var marbleApp *example.MarbleApp
var committer *example.Committer
var consenter *example.Consenter

func init() {

	// Initialization will get a handle to the ledger at the specified path
	// Note, if subledgers are supported in the future,
	// the various ledgers could be created/managed at this level
	logger.Debugf("Marble Example main init()")

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig()

	cleanup()
	ledgermgmt.Initialize()
	var err error
	gb, _ := configtxtest.MakeGenesisBlock(ledgerID)
	peerLedger, err = ledgermgmt.CreateLedger(gb)

	if err != nil {
		panic(fmt.Errorf("Error in NewKVLedger(): %s", err))
	}
	marbleApp = example.ConstructMarbleAppInstance(peerLedger)
	committer = example.ConstructCommitter(peerLedger)
	consenter = example.ConstructConsenter()
}

func main() {
	defer ledgermgmt.Close()
	// Each of the functions here will emulate endorser, orderer,
	// and committer by calling ledger APIs to similate the proposal,
	// get simulation results, create a transaction, add it to a block,
	// and then commit the block.

	initApp()
	transferMarble()

}

func initApp() {
	logger.Debugf("Marble Example initApp() to create a marble")
	marble := []string{"marble1", "blue", "35", "tom"}
	tx, err := marbleApp.CreateMarble(marble)
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx)
	err = committer.Commit(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock)
}

func transferMarble() {
	logger.Debugf("Marble Example transferMarble()")
	tx1, err := marbleApp.TransferMarble([]string{"marble1", "jerry"})
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx1)
	err = committer.Commit(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock)
}

func printBlocksInfo(block *common.Block) {
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
