/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
)

// Provider provides handle to specific 'Store' that in turn manages
// private write sets for a ledger
type Provider interface {
	OpenStore(id string) (Store, error)
	Close()
}

// Store manages the permanent storage of private write sets for a ledger
// The expected use is such that - first the private data will be given to
// this store (via `Prepare` funtion) and then the block is appended to the block storage.
// Finally, the functions `Commit` invoked on this store. The store implementation
// is expected to survive a server crash between the call to `Prepare` and `Commit`.
type Store interface {
	// Init initializes the store. This function is expected to be invoked before using the store
	Init(btlPolicy pvtdatapolicy.BTLPolicy)
	// InitLastCommittedBlockHeight sets the last commited block height into the pvt data store
	// This function is used in a special case where the peer is started up with the blockchain
	// from an earlier version of a peer when the pvt data feature (and hence this store) was not
	// available. This function is expected to be called only this situation and hence is
	// expected to throw an error if the store is not empty. On a successful return from this
	// fucntion the state of the store is expected to be same as of calling the prepare/commit
	// function for block `0` through `blockNum` with no pvt data
	InitLastCommittedBlock(blockNum uint64) error
	// GetPvtDataByBlockNum returns only the pvt data  corresponding to the given block number
	// The pvt data is filtered by the list of 'ns/collections' supplied in the filter
	// A nil filter does not filter any results
	GetPvtDataByBlockNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error)
	// GetMissingPvtDataInfoForMostRecentBlocks returns the missing private data information for the
	// most recent `maxBlock` blocks which miss at least a private data of a eligible collection.
	GetMissingPvtDataInfoForMostRecentBlocks(maxBlock int) (ledger.MissingPvtDataInfo, error)
	// Prepare prepares the Store for commiting the pvt data and storing both eligible and ineligible
	// missing private data --- `eligible` denotes that the missing private data belongs to a collection
	// for which this peer is a member; `ineligible` denotes that the missing private data belong to a
	// collection for which this peer is not a member.
	// This call does not commit the pvt data and store missing private data. Subsequently, the caller
	// is expected to call `Commit` function. Return from this should ensure
	// that enough preparation is done such that `Commit` function invoked afterwards can commit the
	// data and the store is capable of surviving a crash between this function call and the next
	// invoke to the `Commit`
	Prepare(blockNum uint64, pvtData []*ledger.TxPvtData, missingPvtData ledger.TxMissingPvtDataMap) error
	// Commit commits the pvt data passed in the previous invoke to the `Prepare` function
	Commit() error
	// ProcessCollsEligibilityEnabled notifies the store when the peer becomes eligible to recieve data for an
	// existing collection. Parameter 'committingBlk' refers to the block number that contains the corresponding
	// collection upgrade transaction and the parameter 'nsCollMap' contains the collections for which the peer
	// is now eligible to recieve pvt data
	ProcessCollsEligibilityEnabled(committingBlk uint64, nsCollMap map[string][]string) error
	// CommitPvtDataOfOldBlocks commits the pvtData (i.e., previously missing data) of old blocks.
	// The parameter `blocksPvtData` refers a list of old block's pvtdata which are missing in the pvtstore.
	// This call stores an additional entry called `lastUpdatedOldBlocksList` which keeps the exact list
	// of updated blocks. This list would be used during recovery process. Once the stateDB is updated with
	// these pvtData, the `lastUpdatedOldBlocksList` must be removed. During the peer startup,
	// if the `lastUpdatedOldBlocksList` exists, stateDB needs to be updated with the appropriate pvtData.
	CommitPvtDataOfOldBlocks(blocksPvtData map[uint64][]*ledger.TxPvtData) error
	// GetLastUpdatedOldBlocksPvtData returns the pvtdata of blocks listed in `lastUpdatedOldBlocksList`
	GetLastUpdatedOldBlocksPvtData() (map[uint64][]*ledger.TxPvtData, error)
	// ResetLastUpdatedOldBlocksList removes the `lastUpdatedOldBlocksList` entry from the store
	ResetLastUpdatedOldBlocksList() error
	// IsEmpty returns true if the store does not have any block committed yet
	IsEmpty() (bool, error)
	// LastCommittedBlockHeight returns the height of the last committed block
	LastCommittedBlockHeight() (uint64, error)
	// HasPendingBatch returns if the store has a pending batch
	HasPendingBatch() (bool, error)
	// Shutdown stops the store
	Shutdown()
}

// ErrIllegalCall is to be thrown by a store impl if the store does not expect a call to Prepare/Commit/InitLastCommittedBlock
type ErrIllegalCall struct {
	msg string
}

func (err *ErrIllegalCall) Error() string {
	return err.msg
}

// ErrIllegalArgs is to be thrown by a store impl if the args passed are not allowed
type ErrIllegalArgs struct {
	msg string
}

func (err *ErrIllegalArgs) Error() string {
	return err.msg
}

// ErrOutOfRange is to be thrown for the request for the data that is not yet committed
type ErrOutOfRange struct {
	msg string
}

func (err *ErrOutOfRange) Error() string {
	return err.msg
}
