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

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/example"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("main")

const (
	ledgerPath = "/tmp/test/ledgernext/kvledger/example"
)

var finalLedger ledger.ValidatedLedger
var marbleApp *example.MarbleApp
var committer *example.Committer
var consenter *example.Consenter

func init() {

	// Initialization will get a handle to the ledger at the specified path
	// Note, if subledgers are supported in the future,
	// the various ledgers could be created/managed at this level
	logger.Debugf("===COUCHDB=== Marble Example main init()")

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../peer")

	os.RemoveAll(ledgerPath)
	ledgerConf := kvledger.NewConf(ledgerPath, 0)
	var err error
	finalLedger, err = kvledger.NewKVLedger(ledgerConf)
	if err != nil {
		panic(fmt.Errorf("Error in NewKVLedger(): %s", err))
	}
	marbleApp = example.ConstructMarbleAppInstance(finalLedger)
	committer = example.ConstructCommitter(finalLedger)
	consenter = example.ConstructConsenter()
}

func main() {
	defer finalLedger.Close()

	// Each of the functions here will emulate endorser, orderer,
	// and committer by calling ledger APIs to similate the proposal,
	// get simulation results, create a transaction, add it to a block,
	// and then commit the block.

	initApp()
	transferMarble()

}

func initApp() {
	logger.Debugf("===COUCHDB=== Marble Example initApp() to create a marble")
	marble := []string{"marble1", "blue", "35", "tom"}
	tx, err := marbleApp.CreateMarble(marble)
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx)
	finalBlock, invalidTx, err := committer.CommitBlock(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock, finalBlock, invalidTx)
}

func transferMarble() {
	logger.Debugf("===COUCHDB=== Marble Example transferMarble()")
	tx1, err := marbleApp.TransferMarble([]string{"marble1", "jerry"})
	handleError(err, true)
	rawBlock := consenter.ConstructBlock(tx1)
	finalBlock, invalidTx, err := committer.CommitBlock(rawBlock)
	handleError(err, true)
	printBlocksInfo(rawBlock, finalBlock, invalidTx)
}

func printBlocksInfo(rawBlock *common.Block, finalBlock *common.Block, invalidTxs []*pb.InvalidTransaction) {
	fmt.Printf("Num txs in rawBlock = [%d], num invalidTxs = [%d]\n",
		len(rawBlock.Data.Data), len(invalidTxs))
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
