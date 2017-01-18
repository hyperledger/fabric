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

import "github.com/hyperledger/fabric/protos/common"

// Committer is the interface supported by committers
// The only committer is noopssinglechain committer.
// The interface is intentionally sparse with the sole
// aim of "leave-everything-to-the-committer-for-now".
// As we solidify the bootstrap process and as we add
// more support (such as Gossip) this interface will
// change
type Committer interface {

	// Commit block to the ledger
	Commit(block *common.Block) error

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Gets blocks with sequence numbers provided in the slice
	GetBlocks(blockSeqs []uint64) []*common.Block

	// Closes committing service
	Close()
}
