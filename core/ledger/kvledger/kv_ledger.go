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

package kvledger

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/blkstorage"
	"github.com/hyperledger/fabric/core/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger/kvledger/kvledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/couchdbtxmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/lockbasedtxmgmt"
	"github.com/hyperledger/fabric/protos"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("kvledger")

// Conf captures `KVLedger` configurations
type Conf struct {
	blockStorageDir  string
	maxBlockfileSize int
	txMgrDBPath      string
}

// NewConf constructs new `Conf`.
// filesystemPath is the top level directory under which `KVLedger` manages its data
func NewConf(filesystemPath string, maxBlockfileSize int) *Conf {
	if !strings.HasSuffix(filesystemPath, "/") {
		filesystemPath = filesystemPath + "/"
	}
	blocksStorageDir := filesystemPath + "blocks"
	txMgrDBPath := filesystemPath + "txMgmgt/db"
	return &Conf{blocksStorageDir, maxBlockfileSize, txMgrDBPath}
}

// KVLedger provides an implementation of `ledger.ValidatedLedger`.
// This implementation provides a key-value based data model
type KVLedger struct {
	blockStore           blkstorage.BlockStore
	txtmgmt              txmgmt.TxMgr
	pendingBlockToCommit *protos.Block2
}

// NewKVLedger constructs new `KVLedger`
func NewKVLedger(conf *Conf) (*KVLedger, error) {
	blockStore := fsblkstorage.NewFsBlockStore(fsblkstorage.NewConf(conf.blockStorageDir, conf.maxBlockfileSize))

	if kvledgerconfig.IsCouchDBEnabled() == true {
		//By default we can talk to CouchDB with empty id and pw (""), or you can add your own id and password to talk to a secured CouchDB
		logger.Debugf("===COUCHDB=== NewKVLedger() Using CouchDB instead of RocksDB...hardcoding and passing connection config for now")
		txmgmt := couchdbtxmgmt.NewCouchDBTxMgr(&couchdbtxmgmt.Conf{DBPath: conf.txMgrDBPath},
			"127.0.0.1",   //couchDB host
			5984,          //couchDB port
			"marbles_app", //couchDB db name
			"",            //enter couchDB id here
			"")            //enter couchDB pw here
		return &KVLedger{blockStore, txmgmt, nil}, nil
	}

	// Fall back to using RocksDB lockbased transaction manager
	txmgmt := lockbasedtxmgmt.NewLockBasedTxMgr(&lockbasedtxmgmt.Conf{DBPath: conf.txMgrDBPath})
	return &KVLedger{blockStore, txmgmt, nil}, nil

}

// GetTransactionByID retrieves a transaction by id
func (l *KVLedger) GetTransactionByID(txID string) (*protos.Transaction2, error) {
	return l.blockStore.RetrieveTxByID(txID)
}

// GetBlockchainInfo returns basic info about blockchain
func (l *KVLedger) GetBlockchainInfo() (*protos.BlockchainInfo, error) {
	return l.blockStore.GetBlockchainInfo()
}

// GetBlockByNumber returns block at a given height
func (l *KVLedger) GetBlockByNumber(blockNumber uint64) (*protos.Block2, error) {
	return l.blockStore.RetrieveBlockByNumber(blockNumber)

}

// GetBlocksByNumber returns all the blocks between given heights (both inclusive). ResultsIterator contains type BlockHolder
func (l *KVLedger) GetBlocksByNumber(startBlockNumber, endBlockNumber uint64) (ledger.ResultsIterator, error) {
	return l.blockStore.RetrieveBlocks(startBlockNumber, endBlockNumber)

}

// GetBlockByHash returns a block given it's hash
func (l *KVLedger) GetBlockByHash(blockHash []byte) (*protos.Block2, error) {
	return l.blockStore.RetrieveBlockByHash(blockHash)
}

//VerifyChain will verify the integrity of the blockchain. This is accomplished
// by ensuring that the previous block hash stored in each block matches
// the actual hash of the previous block in the chain. The return value is the
// block number of lowest block in the range which can be verified as valid.
func (l *KVLedger) VerifyChain(startBlockNumber, endBlockNumber uint64) (uint64, error) {
	return 0, errors.New("Not yet implemented")
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (l *KVLedger) Prune(policy ledger.PrunePolicy) error {
	return errors.New("Not yet implemented")
}

// NewTxSimulator returns new `ledger.TxSimulator`
func (l *KVLedger) NewTxSimulator() (ledger.TxSimulator, error) {
	return l.txtmgmt.NewTxSimulator()
}

// NewQueryExecutor gives handle to a query executer.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (l *KVLedger) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return l.txtmgmt.NewQueryExecutor()
}

// RemoveInvalidTransactionsAndPrepare validates all the transactions in the given block
// and returns a block that contains only valid transactions and a list of transactions that are invalid
func (l *KVLedger) RemoveInvalidTransactionsAndPrepare(block *protos.Block2) (*protos.Block2, []*protos.InvalidTransaction, error) {
	var validBlock *protos.Block2
	var invalidTxs []*protos.InvalidTransaction
	var err error
	validBlock, invalidTxs, err = l.txtmgmt.ValidateAndPrepare(block)
	if err == nil {
		l.pendingBlockToCommit = validBlock
	}
	return validBlock, invalidTxs, err
}

// Commit commits the valid block (returned in the method RemoveInvalidTransactionsAndPrepare) and related state changes
func (l *KVLedger) Commit() error {
	if l.pendingBlockToCommit == nil {
		panic(fmt.Errorf(`Nothing to commit. RemoveInvalidTransactionsAndPrepare() method should have been called and should not have thrown error`))
	}
	if err := l.blockStore.AddBlock(l.pendingBlockToCommit); err != nil {
		return err
	}
	if err := l.txtmgmt.Commit(); err != nil {
		panic(fmt.Errorf(`Error during commit to txmgr:%s`, err))
	}
	l.pendingBlockToCommit = nil
	return nil
}

// Rollback rollbacks the changes caused by the last invocation to method `RemoveInvalidTransactionsAndPrepare`
func (l *KVLedger) Rollback() {
	l.txtmgmt.Rollback()
	l.pendingBlockToCommit = nil
}

// Close closes `KVLedger`
func (l *KVLedger) Close() {
	l.blockStore.Shutdown()
	l.txtmgmt.Shutdown()
}
