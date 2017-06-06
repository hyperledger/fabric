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

package pvtrwstorage

import "github.com/hyperledger/fabric/common/ledger"

// StoreProvider provides handle to specific 'RWSetStore' that in turn manages
// private read-write sets for a namespace
type StoreProvider interface {
	OpenStore(namespace string) (RWSetStore, error)
	Close()
}

// TxRWSet contains the private read-write set for a transaction
type TxRWSet struct {
	txid  string
	rwset []byte
}

// BlockRWSet contains the set of TxRWSet present in a block
type BlockRWSet struct {
	blockNum  uint64
	txsRWSets []*TxRWSet
}

// RWSetStore manages the permanent storage of private read-write sets for a namespace
type RWSetStore interface {
	Persist(blkRWSet *BlockRWSet) error
	GetRWSetByTxid(txid string) (*TxRWSet, error)
	GetRWSetByBlocknumTxnum(blockNum uint64, txNum uint64) (*TxRWSet, error)
	GetRWSetsByBlockNum(blockNum uint64) (*BlockRWSet, error)
	RetrieveBlocks(startingBlockNum uint64) (ledger.ResultsIterator, error)
	GetLastBlockPersisted() (uint64, error)
	GetMinBlockNum() (uint64, error)
	Purge(maxBlockNumToRetain uint64) error
	Shutdown()
}
