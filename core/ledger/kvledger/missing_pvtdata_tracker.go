/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"math"

	"github.com/hyperledger/fabric/core/ledger"
)

type missingPvtdataTracker struct {
	kvLedger             *kvLedger
	nextStartingBlockNum uint64
}

// GetMissingPvtDataInfoForMostRecentBlocks returns the missing private data information for the
// most recent `maxBlock` blocks which miss at least a private data of a eligible collection.
func (t *missingPvtdataTracker) GetMissingPvtDataInfoForMostRecentBlocks(maxBlock int) (ledger.MissingPvtDataInfo, error) {
	if t.nextStartingBlockNum == 0 {
		return nil, nil
	}
	// the missing pvtData info in the pvtdataStore could belong to a block which is yet
	// to be processed and committed to the blockStore and stateDB (such a scenario is possible
	// after a peer rollback). In such cases, we cannot return missing pvtData info. Otherwise,
	// we would end up in an inconsistent state database.
	if t.kvLedger.isPvtstoreAheadOfBlkstore.Load().(bool) {
		return nil, nil
	}
	// it is safe to not acquire a read lock on l.blockAPIsRWLock. Without a lock, the value of
	// lastCommittedBlock can change due to a new block commit. As a result, we may not
	// be able to fetch the missing data info of truly the most recent blocks. This
	// decision was made to ensure that the regular block commit rate is not affected.
	missingPvtdataInfo, err := t.kvLedger.pvtdataStore.GetMissingPvtDataInfoForMostRecentBlocks(t.nextStartingBlockNum, maxBlock)
	if err != nil {
		return nil, err
	}

	var smallestBlkNum uint64 = math.MaxUint64
	for blkNum := range missingPvtdataInfo {
		if blkNum < smallestBlkNum {
			smallestBlkNum = blkNum
		}
	}
	t.nextStartingBlockNum = smallestBlkNum - 1
	return missingPvtdataInfo, nil
}
