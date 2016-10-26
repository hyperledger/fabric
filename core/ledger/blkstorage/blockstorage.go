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

package blkstorage

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos"
)

// BlockStore - an interface for persisting and retrieving blocks
type BlockStore interface {
	AddBlock(block *protos.Block2) error
	GetBlockchainInfo() (*protos.BlockchainInfo, error)
	RetrieveBlocks(startNum uint64, endNum uint64) (ledger.ResultsIterator, error)
	RetrieveBlockByHash(blockHash []byte) (*protos.Block2, error)
	RetrieveBlockByNumber(blockNum uint64) (*protos.Block2, error)
	RetrieveTxByID(txID string) (*protos.Transaction2, error)
	Shutdown()
}
