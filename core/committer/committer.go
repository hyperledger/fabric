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

package committer

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// Committer is the interface supported by committers
// The only committer is noopssinglechain committer.
// The interface is intentionally sparse with the sole
// aim of "leave-everything-to-the-committer-for-now".
// As we solidify the bootstrap process and as we add
// more support (such as Gossip) this interface will
// change
type Committer interface {

	// CommitWithPvtData block and private data into the ledger
	CommitWithPvtData(blockAndPvtData *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error

	// GetPvtDataAndBlockByNum retrieves block with private data with given
	// sequence number
	GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error)

	// GetPvtDataByNum returns a slice of the private data from the ledger
	// for given block and based on the filter which indicates a map of
	// collections and namespaces of private data to retrieve
	GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// DoesPvtDataInfoExistInLedger returns true if the ledger has pvtdata info
	// about a given block number.
	DoesPvtDataInfoExistInLedger(blockNum uint64) (bool, error)

	// Gets blocks with sequence numbers provided in the slice
	GetBlocks(blockSeqs []uint64) []*common.Block

	// GetConfigHistoryRetriever returns the ConfigHistoryRetriever
	GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error)

	// CommitPvtDataOfOldBlocks commits the private data corresponding to already committed block
	// If hashes for some of the private data supplied in this function does not match
	// the corresponding hash present in the block, the unmatched private data is not
	// committed and instead the mismatch inforation is returned back
	CommitPvtDataOfOldBlocks(blockPvtData []*ledger.BlockPvtData) ([]*ledger.PvtdataHashMismatch, error)

	// GetMissingPvtDataTracker return the MissingPvtDataTracker
	GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error)

	// Closes committing service
	Close()
}
